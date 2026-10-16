package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupRoutingConfigTest(t *testing.T) smartrouting.Config {
	t.Helper()
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.RoutingWeightSnapshot{}, &model.RoutingWeightScheme{}))
	prev := smartrouting.GetConfig()
	t.Cleanup(func() { smartrouting.SetConfig(prev) })
	cfg := smartrouting.DefaultConfig()
	cfg.Enabled = true
	cfg.ActiveWeightScheme = "balanced_default"
	cfg.BalancePatrolEnabled = true
	cfg.TokenSkewSuspect = 0.15
	cfg.TokenSkewFail = 0.4
	cfg.HalfOpenPreferNonStream = true
	cfg.PriceScoreSkipNonToken = true
	smartrouting.SetConfig(cfg)
	return smartrouting.GetConfig()
}

func routingCall(t *testing.T, h gin.HandlerFunc, body string) (int, map[string]any) {
	t.Helper()
	rec := adtyCall(h, http.MethodPut, "/api/adty/routing/config", body, 1, common.RoleRootUser, nil)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return rec.Code, out
}

// 审查 P1-3 A：启停开关只发 {"enabled":false}，无需 preview_token，且其他字段不变。
func TestRoutingPutConfigToggleIsPartialAndNeedsNoToken(t *testing.T) {
	before := setupRoutingConfigTest(t)
	code, out := routingCall(t, AdtyRoutingPutConfig, `{"enabled":false}`)
	require.Equal(t, http.StatusOK, code, out)
	after := smartrouting.GetConfig()
	require.False(t, after.Enabled)
	after.Enabled = true
	require.Equal(t, before, after, "only enabled may change")
}

// 审查 P1-3 B/C：preview {weights} → PUT {weights, preview_token} 成功，enabled 及其他参数保持不变。
func TestRoutingApplyWeightsKeepsOtherFields(t *testing.T) {
	before := setupRoutingConfigTest(t)
	rec := adtyCall(AdtyRoutingPreviewScheme, http.MethodPost, "/api/adty/routing/weight-schemes/preview",
		`{"weights":{"conn":40,"price":20,"speed":20,"stab":10,"auth":10}}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var pv struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pv))
	require.NotEmpty(t, pv.Data.PreviewToken)

	code, out := routingCall(t, AdtyRoutingPutConfig,
		`{"weights":{"conn":40,"price":20,"speed":20,"stab":10,"auth":10},"preview_token":"`+pv.Data.PreviewToken+`"}`)
	require.Equal(t, http.StatusOK, code, out)
	after := smartrouting.GetConfig()
	require.Equal(t, smartrouting.WeightParts{Conn: 40, Price: 20, Speed: 20, Stab: 10, Auth: 10}, after.Weights)
	after.Weights = before.Weights
	require.Equal(t, before, after, "apply weights must not touch other fields")
}

// 权重变化仍然必须带 preview_token；空 patch / 非对象 400。
func TestRoutingPutConfigWeightChangeStillGated(t *testing.T) {
	setupRoutingConfigTest(t)
	code, out := routingCall(t, AdtyRoutingPutConfig, `{"weights":{"conn":90,"price":10,"speed":0,"stab":0,"auth":0}}`)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "preview_required", out["code"])
	code, out = routingCall(t, AdtyRoutingPutConfig, `{}`)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "empty_patch", out["code"])
	code, _ = routingCall(t, AdtyRoutingPutConfig, `[1]`)
	require.Equal(t, http.StatusBadRequest, code)
}

// 审查 P2-3：坏快照回滚返回 400 bad_snapshot，不再报 rolled_back。
func TestRoutingRollbackBadSnapshot(t *testing.T) {
	setupRoutingConfigTest(t)
	snap := model.RoutingWeightSnapshot{Kind: "scheme", Snapshot: `{"name":"x","weights":"not-a-config"}`}
	require.NoError(t, model.DB.Create(&snap).Error)
	body, _ := json.Marshal(map[string]any{"snapshot_id": snap.ID})
	rec := adtyCall(AdtyRoutingSchemeRollback, http.MethodPost, "/x", string(body), 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "bad_snapshot")
}
