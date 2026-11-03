package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 站点公开配置（任务 8.3 的后端，在 8.6 先建）。

func TestAdtySitePublicEndpoint(t *testing.T) {
	cfg := config.GlobalConfig.Get("adty_site_setting")
	require.NotNil(t, cfg, "adty_site_setting 要登记到 config")
	old := *adty_setting.GetSiteSetting()
	oldMinTopUp := operation_setting.MinTopUp
	t.Cleanup(func() {
		*adty_setting.GetSiteSetting() = old
		operation_setting.MinTopUp = oldMinTopUp
	})
	// 首页数字条的「$N 起充」读这个，后台「充值设置」里的最低充值金额
	operation_setting.MinTopUp = 5
	// 和后台保存走同一条路：PUT /api/option/ → handleConfigUpdate → UpdateConfigFromMap
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"api_base_url":          "https://api.example.com",
		"support_email":         "support@example.com",
		"telegram_url":          "https://t.me/example",
		"support_hours":         "10:00-22:00 UTC+8",
		"tawk_property_id":      "prop",
		"tawk_widget_id":        "widget",
		"refund_policy_enabled": "true",
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/adty/site", nil)
	GetAdtySite(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	assert.True(t, envelope.Success)
	assert.Equal(t, map[string]any{
		"api_base_url":          "https://api.example.com",
		"support_email":         "support@example.com",
		"telegram_url":          "https://t.me/example",
		"support_hours":         "10:00-22:00 UTC+8",
		"tawk_property_id":      "prop",
		"tawk_widget_id":        "widget",
		"refund_policy_enabled": true,
		"min_topup":             float64(5),
	}, envelope.Data)
}
