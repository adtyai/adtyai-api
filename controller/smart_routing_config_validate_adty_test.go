package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/stretchr/testify/require"
)

// 审查 R-P2-1 / R-P2-2：负权重、未知字段、不存在的方案都返回 400，配置不变。
func TestRoutingPutConfigValidation(t *testing.T) {
	before := setupRoutingConfigTest(t)

	rec := adtyCall(AdtyRoutingPreviewScheme, http.MethodPost, "/api/adty/routing/weight-schemes/preview",
		`{"weights":{"conn":-50,"price":150}}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	code, out := routingCall(t, AdtyRoutingPutConfig, `{"weights":{"conn":-50,"price":150},"preview_token":"x"}`)
	require.Equal(t, http.StatusBadRequest, code, out)
	require.Equal(t, "invalid_weights", out["code"], out)

	code, out = routingCall(t, AdtyRoutingPutConfig, `{"TotallyUnknown":1}`)
	require.Equal(t, http.StatusBadRequest, code, out)
	require.Equal(t, "unknown_field", out["code"], out)

	code, out = routingCall(t, AdtyRoutingPutConfig, `{"active_weight_scheme":"ghost","preview_token":"x"}`)
	require.Equal(t, http.StatusBadRequest, code, out)
	require.Equal(t, "scheme_not_found", out["code"], out)

	rec = adtyCall(AdtyRoutingPreviewScheme, http.MethodPost, "/api/adty/routing/weight-schemes/preview",
		`{"scheme_id":99999}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	require.Equal(t, before, smartrouting.GetConfig())
}
