package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTOTPCodeCannotOpenASecondLoginSession(t *testing.T) {
	user, _ := setupSecurityEnrollmentTest(t)
	const secret = "JBSWY3DPEHPK3PXP"
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: secret, IsEnabled: true}).Error)
	first, err := service.StartLoginVerification(user, "password", nil)
	require.NoError(t, err)
	second, err := service.StartLoginVerification(user, "password", nil)
	require.NoError(t, err)
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)

	kept := postBackupLogin(t, first.FlowToken, code, "")
	assert.True(t, kept.Success, kept.Message)
	replay := postBackupLogin(t, second.FlowToken, code, "")
	assert.False(t, replay.Success, replay.Message)
	assert.EqualValues(t, 2, activeLoginSessions(t, user.Id))
	assert.Positive(t, storedTOTPStep(t, user.Id))
}

func TestTOTPCodeStaysUnusedWhenLoginSessionIsNotRetained(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"

	t.Run("session limit", func(t *testing.T) {
		user, token, code := totpLoginChallenge(t, secret)
		previousLimit := common.UserSessionActiveLimit
		common.UserSessionActiveLimit = 1
		t.Cleanup(func() { common.UserSessionActiveLimit = previousLimit })

		failed := postBackupLogin(t, token, code, "")
		assert.False(t, failed.Success, failed.Message)
		assert.Zero(t, storedTOTPStep(t, user.Id))
		assert.EqualValues(t, 1, activeLoginSessions(t, user.Id))

		common.UserSessionActiveLimit = previousLimit
		kept := postBackupLogin(t, token, code, "")
		assert.True(t, kept.Success, kept.Message)
		assert.Positive(t, storedTOTPStep(t, user.Id))
		assert.EqualValues(t, 2, activeLoginSessions(t, user.Id))
	})

	t.Run("site mismatch", func(t *testing.T) {
		user, token, code := totpLoginChallenge(t, secret)
		saved := *adty_setting.GetResellerSetting()
		t.Cleanup(func() {
			*adty_setting.GetResellerSetting() = saved
			service.InvalidateAdtySiteCache()
		})
		cfg := adty_setting.GetResellerSetting()
		*cfg = adty_setting.DefaultResellerSetting()
		cfg.Enabled = true
		cfg.SiteLoginOnly = true
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("site_id", 7).Error)

		failed := postBackupLogin(t, token, code, "shop.foreign.example")
		assert.False(t, failed.Success, failed.Message)
		assert.Zero(t, storedTOTPStep(t, user.Id))
		assert.EqualValues(t, 1, activeLoginSessions(t, user.Id))

		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("site_id", 0).Error)
		next, err := service.StartLoginVerification(user, "password", nil)
		require.NoError(t, err)
		kept := postBackupLogin(t, next.FlowToken, code, "shop.foreign.example")
		assert.True(t, kept.Success, kept.Message)
		assert.Positive(t, storedTOTPStep(t, user.Id))
		assert.EqualValues(t, 2, activeLoginSessions(t, user.Id))
	})
}

func totpLoginChallenge(t *testing.T, secret string) (*model.User, string, string) {
	t.Helper()
	user, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: secret, IsEnabled: true}).Error)
	challenge, err := service.StartLoginVerification(user, "password", nil)
	require.NoError(t, err)
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	return user, challenge.FlowToken, code
}

func storedTOTPStep(t *testing.T, userID int) int64 {
	t.Helper()
	var step int64
	require.NoError(t, model.DB.Model(&model.TwoFA{}).Where("user_id = ?", userID).Select("last_totp_step").Scan(&step).Error)
	return step
}
