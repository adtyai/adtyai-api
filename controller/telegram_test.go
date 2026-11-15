package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 约定 I：Telegram 只能作通知通道，不能登录、不能充当二次验证。ISS-R4-0121
// 之前这条链是「填上 client_id/client_secret 再 PUT 一个开关」就能用的完整
// OAuth 登录链，凭据还明文存在 options 表里。下面几条把退役的每一面都钉住。
func TestTelegramOAuthLoginIsRetired(t *testing.T) {
	_, identity := setupSecurityEnrollmentTest(t)

	state := securityEnrollmentRequest("POST", "/api/oauth/state", `{"provider":"telegram","intent":"login"}`, "", service.AuthIdentity{}, GenerateOAuthCode)
	assert.Contains(t, state.Body.String(), `"success":false`)
	assert.Contains(t, state.Body.String(), "TELEGRAM_OAUTH_NOT_CONFIGURED")
	assert.NotContains(t, state.Body.String(), "flow_token")

	bind := securityEnrollmentRequest("POST", "/api/oauth/state", `{"provider":"telegram","intent":"bind"}`, "", identity, GenerateOAuthCode)
	assert.Contains(t, bind.Body.String(), `"success":false`)
	assert.Contains(t, bind.Body.String(), "TELEGRAM_OAUTH_NOT_CONFIGURED")
	assert.NotContains(t, bind.Body.String(), "flow_token")

	status := securityEnrollmentRequest("GET", "/api/status", "", "", identity, GetStatus)
	assert.NotContains(t, status.Body.String(), `"telegram_oauth":true`)
	assert.NotContains(t, status.Body.String(), `"telegram_oauth_configured":true`)
}

func TestTelegramOAuthOptionsCannotBeWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, _ = setupSecurityEnrollmentTest(t)
	for _, testCase := range []struct{ key, value string }{
		{"TelegramOAuthEnabled", "true"},
		{"telegram.client_id", "12345"},
		{"telegram.client_secret", "telegram-client-secret"},
	} {
		t.Run(testCase.key, func(t *testing.T) {
			body, err := common.Marshal(OptionUpdateRequest{Key: testCase.key, Value: testCase.value})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))

			UpdateOption(context)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"success":false`)
			var stored model.Option
			assert.Error(t, model.DB.Where(&model.Option{Key: testCase.key}).First(&stored).Error)
		})
	}
}

func TestTelegramOAuthLegacyCredentialsArePurged(t *testing.T) {
	_, _ = setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Create(&model.Option{Key: "telegram.client_secret", Value: "plaintext-secret"}).Error)
	require.NoError(t, model.DB.Create(&model.Option{Key: "telegram.client_id", Value: "12345"}).Error)

	model.InitOptionMap()

	var count int64
	require.NoError(t, model.DB.Model(&model.Option{}).Where("key LIKE ?", "telegram.%").Count(&count).Error)
	assert.Zero(t, count)
}

func TestTelegramLegacyAuthEndpointsStayGone(t *testing.T) {
	_, identity := setupSecurityEnrollmentTest(t)
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/api/oauth/telegram/login"},
		{"POST", "/api/oauth/telegram/bind/start"},
		{"GET", "/api/oauth/telegram/bind/old-flow"},
	} {
		response := securityEnrollmentRequest(endpoint.method, endpoint.path, "", "", identity, TelegramLegacyAuth)
		assert.Equal(t, http.StatusGone, response.Code)
		assert.Contains(t, response.Body.String(), "TELEGRAM_LEGACY_AUTH_REMOVED")
	}
}
