package controller

import (
	"encoding/json"
	"github.com/QuantumNous/new-api/i18n"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// 审查 R-P1-2：上游没有余额接口（返回空对象）的健康渠道，刷新余额后
// 标记 unsupported，且不能被禁用。
func TestRoutingBalanceRefreshNeverDisables(t *testing.T) {
	setupRoutingConfigTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	base := srv.URL
	ch := model.Channel{Id: 51, Name: "m51", Type: constant.ChannelTypeOpenAI, Key: "k", Status: common.ChannelStatusEnabled, Models: "m", Group: "default", BaseURL: &base}
	require.NoError(t, model.DB.Create(&ch).Error)

	r := routingRefreshOneBalance(&ch)
	require.Equal(t, "unsupported", r.Status, r.Error)

	require.NoError(t, routingRefreshAllBalances())
	var got model.Channel
	require.NoError(t, model.DB.First(&got, ch.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, got.Status)
}

func balanceTestChannel(t *testing.T, id int, body string) model.Channel {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	base := srv.URL
	ch := model.Channel{Id: id, Name: "b" + strconv.Itoa(id), Type: constant.ChannelTypeOpenAI, Key: "k", Status: common.ChannelStatusEnabled, Models: "m", Group: "default", BaseURL: &base}
	require.NoError(t, model.DB.Create(&ch).Error)
	return ch
}

// 审查 R3-P2-2：上游返回非标准 JSON（无 hard_limit_usd）→ failed，而不是 ok + 余额 0。
func TestRoutingBalanceRefreshMalformedIsFailed(t *testing.T) {
	setupRoutingConfigTest(t)
	ch := balanceTestChannel(t, 52, `{"foo":"bar","data":{"x":1}}`)
	r := routingRefreshOneBalance(&ch)
	require.Equal(t, "failed", r.Status)
	require.NotEmpty(t, r.Error)
	rec := adtyCall(AdtyRoutingBalanceRefresh, http.MethodPost, "/x", `{"channel_id":52}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
}

// 审查 R3-P2-3：官方「更新全部余额」保留余额<=0 禁用行为，并在响应中返回被禁用渠道。
func TestUpdateAllChannelsBalanceReportsDisabled(t *testing.T) {
	require.NoError(t, i18n.Init())
	setupRoutingConfigTest(t)
	balanceTestChannel(t, 53, `{"object":"billing_subscription","hard_limit_usd":0,"has_payment_method":true,"total_usage":0}`)
	rec := adtyCall(UpdateAllChannelsBalance, http.MethodGet, "/api/channel/update_balance", "", 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Data struct {
			DisabledChannels []struct {
				Id   int    `json:"id"`
				Name string `json:"name"`
			} `json:"disabled_channels"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	require.Len(t, out.Data.DisabledChannels, 1, rec.Body.String())
	require.Equal(t, 53, out.Data.DisabledChannels[0].Id)
	require.Equal(t, "b53", out.Data.DisabledChannels[0].Name)
	var got model.Channel
	require.NoError(t, model.DB.First(&got, 53).Error)
	require.NotEqual(t, common.ChannelStatusEnabled, got.Status)
}
