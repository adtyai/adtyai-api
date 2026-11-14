package controller

import (
	"fmt"
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
	"gorm.io/gorm"
)

func enablePasswordRegisterWithEmailCode(t *testing.T) {
	t.Helper()
	oldVerify, oldRegister, oldPassword := common.EmailVerificationEnabled, common.RegisterEnabled, common.PasswordRegisterEnabled
	common.EmailVerificationEnabled, common.RegisterEnabled, common.PasswordRegisterEnabled = true, true, true
	t.Cleanup(func() {
		common.EmailVerificationEnabled, common.RegisterEnabled, common.PasswordRegisterEnabled = oldVerify, oldRegister, oldPassword
	})
}

func postRegisterWithEmailCode(email, username, code string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(
		fmt.Sprintf(`{"username":%q,"password":"password-123","email":%q,"verification_code":%q}`, username, email, code)))
	Register(c)
	return recorder
}

func TestRegisterConsumesEmailVerificationCode(t *testing.T) {
	emailEnumDB(t)
	enablePasswordRegisterWithEmailCode(t)
	key := model.SiteEmailCodeKey("fresh@example.com", 0)
	require.NoError(t, common.RegisterVerificationCodeWithKey(key, "123456", common.EmailVerificationPurpose))

	ok := postRegisterWithEmailCode("fresh@example.com", "fresh-user", "123456")
	require.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
	assert.False(t, common.VerifyCodeWithKey(key, "123456", common.EmailVerificationPurpose), "a successful register must consume the code")

	// If the address later becomes eligible again, the already-consumed code
	// must still be unusable inside its original validity window.
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "fresh-user").Update("email", "").Error)
	reuse := postRegisterWithEmailCode("fresh@example.com", "fresh-user-2", "123456")
	assert.Contains(t, reuse.Body.String(), `"success":false`)
	assert.Contains(t, reuse.Body.String(), "Verification code is incorrect or has expired")
	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "fresh-user-2").Count(&count).Error)
	assert.Zero(t, count)
}

func TestRegisterWriteFailureDoesNotRestoreConsumedCode(t *testing.T) {
	emailEnumDB(t)
	enablePasswordRegisterWithEmailCode(t)
	key := model.SiteEmailCodeKey("retry@example.com", 0)
	require.NoError(t, common.RegisterVerificationCodeWithKey(key, "654321", common.EmailVerificationPurpose))

	callback := "test:register-consume-no-restore"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.User); ok {
			_ = tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Create().Remove(callback) })

	failed := postRegisterWithEmailCode("retry@example.com", "retry-user", "654321")
	require.NoError(t, model.DB.Callback().Create().Remove(callback))
	assert.Contains(t, failed.Body.String(), `"success":false`)
	assert.False(t, common.VerifyCodeWithKey(key, "654321", common.EmailVerificationPurpose), "a consumed code must not be restored when the user-row write fails")

	retry := postRegisterWithEmailCode("retry@example.com", "retry-user", "654321")
	assert.Contains(t, retry.Body.String(), `"success":false`)
	assert.Contains(t, retry.Body.String(), "Verification code is incorrect or has expired")
	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "retry-user").Count(&count).Error)
	assert.Zero(t, count)

	require.NoError(t, common.RegisterVerificationCodeWithKey(key, "111222", common.EmailVerificationPurpose))
	ok := postRegisterWithEmailCode("retry@example.com", "retry-user", "111222")
	require.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
}

func TestRegisterLocksAfterMaxFailedCodeAttempts(t *testing.T) {
	emailEnumDB(t)
	enablePasswordRegisterWithEmailCode(t)
	key := model.SiteEmailCodeKey("lock@example.com", 0)
	require.NoError(t, common.RegisterVerificationCodeWithKey(key, "135790", common.EmailVerificationPurpose))

	for attempt := 1; attempt <= common.MaxFailAttempts; attempt++ {
		wrong := postRegisterWithEmailCode("lock@example.com", "lock-user", "000000")
		assert.Contains(t, wrong.Body.String(), `"success":false`)
		if attempt < common.MaxFailAttempts {
			assert.Contains(t, wrong.Body.String(), "Verification code is incorrect or has expired")
			assert.True(t, common.VerifyCodeWithKey(key, "135790", common.EmailVerificationPurpose), "a counted miss must not consume the code")
		} else {
			assert.Contains(t, wrong.Body.String(), service.ErrVerificationCodeLocked.Error())
		}
	}

	locked := postRegisterWithEmailCode("lock@example.com", "lock-user", "135790")
	assert.Contains(t, locked.Body.String(), `"success":false`)
	assert.Contains(t, locked.Body.String(), service.ErrVerificationCodeLocked.Error())
	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "lock-user").Count(&count).Error)
	assert.Zero(t, count)

	require.NoError(t, common.RegisterVerificationCodeWithKey(key, "246810", common.EmailVerificationPurpose))
	ok := postRegisterWithEmailCode("lock@example.com", "lock-user", "246810")
	require.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
}
