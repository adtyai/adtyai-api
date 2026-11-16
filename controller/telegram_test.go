package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 约定 I：Telegram 只能作通知通道，不能登录、不能充当二次验证。第三方登录框架
// 已整体删除；这里只钉住旧开关和凭据既不能写入，也会在启动时被清掉。
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
