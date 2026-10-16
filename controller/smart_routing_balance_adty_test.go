package controller

import (
	"net/http"
	"net/http/httptest"
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
