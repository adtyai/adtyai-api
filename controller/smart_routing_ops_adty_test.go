package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 审查 N-P1-5：余额刷新不再空返回 accepted；指定渠道不存在 404，非 root 403。
func TestRoutingBalanceRefreshValidates(t *testing.T) {
	setupRoutingConfigTest(t)
	rec := adtyCall(AdtyRoutingBalanceRefresh, http.MethodPost, "/x", `{"channel_id":424242}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = adtyCall(AdtyRoutingBalanceRefresh, http.MethodPost, "/x", `{}`, 2, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// 审查 N-P2-4：方案权重校验、重名、禁删生效方案、导出可再导入（改名）。
func TestRoutingSchemeValidation(t *testing.T) {
	setupRoutingConfigTest(t)
	call := func(h func(*gin.Context), body string, params gin.Params) (int, string) {
		rec := adtyCall(h, http.MethodPost, "/x", body, 1, common.RoleRootUser, params)
		return rec.Code, rec.Body.String()
	}
	code, body := call(AdtyRoutingCreateScheme, `{"name":"neg","weights":{"conn":-1,"price":1,"speed":0,"stab":0,"auth":0}}`, nil)
	require.Equal(t, http.StatusBadRequest, code, body)
	code, body = call(AdtyRoutingCreateScheme, `{"name":"zero","weights":{"conn":0,"price":0,"speed":0,"stab":0,"auth":0}}`, nil)
	require.Equal(t, http.StatusBadRequest, code, body)
	code, body = call(AdtyRoutingImportScheme, `{"name":"junk","weights":"not-json"}`, nil)
	require.Equal(t, http.StatusBadRequest, code, body)
	code, body = call(AdtyRoutingCreateScheme, `{"name":"ok1","weights":{"conn":1,"price":1,"speed":1,"stab":1,"auth":1}}`, nil)
	require.Equal(t, http.StatusOK, code, body)
	code, body = call(AdtyRoutingCreateScheme, `{"name":"ok1","weights":{"conn":1,"price":1,"speed":1,"stab":1,"auth":1}}`, nil)
	require.Equal(t, http.StatusConflict, code, body)

	var row model.RoutingWeightScheme
	require.NoError(t, model.DB.Where("name = ?", "ok1").First(&row).Error)
	exp := adtyCall(AdtyRoutingExportScheme, http.MethodGet, "/x", "", 1, common.RoleRootUser, gin.Params{{Key: "id", Value: strconv.FormatInt(row.ID, 10)}})
	var ex struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(exp.Body.Bytes(), &ex))
	ex.Data["name"] = "ok1-copy"
	b, _ := json.Marshal(ex.Data)
	code, body = call(AdtyRoutingImportScheme, string(b), nil)
	require.Equal(t, http.StatusOK, code, body)

	cfg := smartrouting.GetConfig()
	cfg.ActiveWeightScheme = "ok1"
	smartrouting.SetConfig(cfg)
	rec := adtyCall(AdtyRoutingDeleteScheme, http.MethodDelete, "/x", "", 1, common.RoleRootUser, gin.Params{{Key: "id", Value: strconv.FormatInt(row.ID, 10)}})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

// 全量审查 tip 2400c2d P2-1：preview 与 PUT config 同门，仅 root；admin(role=10) 403。
func TestRoutingPreviewRequiresRoot(t *testing.T) {
	setupRoutingConfigTest(t)
	body := `{"weights":{"conn":40,"price":20,"speed":20,"stab":10,"auth":10},"kind":"put_config"}`
	rec := adtyCall(AdtyRoutingPreviewScheme, http.MethodPost, "/api/adty/routing/weight-schemes/preview",
		body, 2, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "root only")
	rec = adtyCall(AdtyRoutingPreviewScheme, http.MethodPost, "/api/adty/routing/weight-schemes/preview",
		body, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// 审查 N-P2-5：标签/价格/鉴权标记写操作仅 root，不存在的渠道/记录不再报成功。
func TestRoutingTagsPricesValidate(t *testing.T) {
	setupRoutingConfigTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelRouteTag{}, &model.ChannelCostPrice{}, &model.ChannelAuthFlag{}))
	rec := adtyCall(AdtyRoutingCreateTag, http.MethodPost, "/x", `{"channel_id":1,"tag":"fake_model"}`, 2, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusForbidden, rec.Code)
	rec = adtyCall(AdtyRoutingCreateTag, http.MethodPost, "/x", `{"channel_id":999,"tag":"fake_model"}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	rec = adtyCall(AdtyRoutingPutPrices, http.MethodPut, "/x", `[{"channel_id":999,"model":"x"}]`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = adtyCall(AdtyRoutingDeleteTag, http.MethodDelete, "/x?id=9999", "", 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = adtyCall(AdtyRoutingMarkAuthFlag, http.MethodPut, "/x", `{"false_positive":true}`, 1, common.RoleRootUser, gin.Params{{Key: "id", Value: "9999"}})
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}
