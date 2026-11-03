package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 多语言条款（任务 8.6）：本语言 → en → 上游 legal.*。

// withLegalTexts 换掉上游和我们的条款设置，测试结束还原。设置走后台保存同一条路。
func withLegalTexts(t *testing.T, upstream map[string]string, adty map[string]string) {
	t.Helper()
	oldUpstream := *system_setting.GetLegalSettings()
	oldAdty := *adty_setting.GetLegalSetting()
	t.Cleanup(func() {
		*system_setting.GetLegalSettings() = oldUpstream
		*adty_setting.GetLegalSetting() = oldAdty
	})
	*system_setting.GetLegalSettings() = system_setting.LegalSettings{}
	*adty_setting.GetLegalSetting() = adty_setting.LegalSetting{}
	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("legal"), upstream))
	cfg := config.GlobalConfig.Get("adty_legal_setting")
	require.NotNil(t, cfg, "adty_legal_setting 要登记到 config")
	require.NoError(t, config.UpdateConfigFromMap(cfg, adty))
}

func getAdtyLegal(t *testing.T, doc, lang string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/adty/legal/:doc", GetAdtyLegal)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/adty/legal/"+doc+"?lang="+lang, nil))
	if recorder.Code != http.StatusOK {
		return recorder.Code, ""
	}
	var envelope publicContentResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	assert.True(t, envelope.Success)
	assert.NotEmpty(t, recorder.Header().Get("ETag"), "和上游条款接口一样带 ETag")
	return recorder.Code, envelope.Data
}

func TestAdtyLegalFallbackOrder(t *testing.T) {
	withLegalTexts(t,
		map[string]string{"user_agreement": "upstream agreement", "privacy_policy": "upstream privacy"},
		map[string]string{
			"user_agreement": `{"zh-CN":"中文协议","en":"English agreement"}`,
			"privacy_policy": `{"ja":"  "}`,
			"refund_policy":  `{"en":"English refund"}`,
		})

	cases := []struct {
		name, doc, lang, want string
	}{
		{"本语言有就用本语言", "user-agreement", "zh-CN", "中文协议"},
		{"本语言没有用 en", "user-agreement", "fr", "English agreement"},
		{"不带语言用 en", "user-agreement", "", "English agreement"},
		{"只有空白算没有，回退到上游", "privacy-policy", "ja", "upstream privacy"},
		{"都没有用上游", "privacy-policy", "zh-TW", "upstream privacy"},
		{"退款政策回退到 en", "refund-policy", "vi", "English refund"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := getAdtyLegal(t, tc.doc, tc.lang)
			require.Equal(t, http.StatusOK, code)
			assert.Equal(t, tc.want, body)
		})
	}
}

func TestAdtyRefundPolicyWithoutTextIsEmpty(t *testing.T) {
	withLegalTexts(t, map[string]string{"user_agreement": "upstream agreement"}, map[string]string{})
	code, body := getAdtyLegal(t, "refund-policy", "en")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "", body, "退款政策没有上游文本，不能回退成用户协议")
}

func TestAdtyLegalUnknownDocRejected(t *testing.T) {
	withLegalTexts(t, map[string]string{}, map[string]string{})
	code, _ := getAdtyLegal(t, "cookie-policy", "en")
	assert.Equal(t, http.StatusNotFound, code)
}

// 上游的两个开关只看上游 legal.*：只填了上游文本时仍为真，只填了我们的文本时不变。
func TestUpstreamLegalSwitchWithUpstreamTextOnly(t *testing.T) {
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousMap })
	status := func() map[string]any {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
		GetStatus(ctx)
		var payload struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload.Data
	}

	withLegalTexts(t, map[string]string{"user_agreement": "upstream agreement", "privacy_policy": "upstream privacy"}, map[string]string{})
	data := status()
	assert.Equal(t, true, data["user_agreement_enabled"])
	assert.Equal(t, true, data["privacy_policy_enabled"])

	withLegalTexts(t, map[string]string{}, map[string]string{"user_agreement": `{"en":"English agreement"}`})
	data = status()
	assert.Equal(t, false, data["user_agreement_enabled"])
	assert.Equal(t, false, data["privacy_policy_enabled"])
}
