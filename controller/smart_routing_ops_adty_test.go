package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/stretchr/testify/require"
)

// 审查 N-P1-5：成本回填真实写 channel_cost_ledger，且幂等。
func TestRoutingCostBackfillWritesLedger(t *testing.T) {
	setupRoutingConfigTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelCostLedger{}, &model.ChannelCostPrice{}, &model.Log{}))
	cfg := smartrouting.GetConfig()
	cfg.HistoryBackfillEnabled = true
	smartrouting.SetConfig(cfg)
	now := time.Now().Unix()
	for i := 0; i < 3; i++ {
		require.NoError(t, model.LOG_DB.Create(&model.Log{Type: model.LogTypeConsume, CreatedAt: now - 60, ChannelId: 7, ModelName: "m", PromptTokens: 100, CompletionTokens: 50, RequestId: "rq" + strconv.Itoa(i)}).Error)
	}
	body := `{"from":` + strconv.FormatInt(now-3600, 10) + `,"to":` + strconv.FormatInt(now+1, 10) + `}`
	for round, want := range []float64{3, 0} {
		rec := adtyCall(AdtyRoutingCostBackfill, http.MethodPost, "/x", body, 1, common.RoleRootUser, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var out struct {
			Data map[string]float64 `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		require.Equal(t, want, out.Data["inserted"], "round %d", round)
	}
	var n int64
	require.NoError(t, model.DB.Model(&model.ChannelCostLedger{}).Where("channel_id = ?", 7).Count(&n).Error)
	require.EqualValues(t, 3, n)
}

// 审查 N-P1-5：余额刷新不再空返回 accepted；指定渠道不存在 404，非 root 403。
func TestRoutingBalanceRefreshValidates(t *testing.T) {
	setupRoutingConfigTest(t)
	rec := adtyCall(AdtyRoutingBalanceRefresh, http.MethodPost, "/x", `{"channel_id":424242}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = adtyCall(AdtyRoutingBalanceRefresh, http.MethodPost, "/x", `{}`, 2, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}
