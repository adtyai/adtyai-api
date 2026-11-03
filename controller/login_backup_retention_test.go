package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupCodeStaysUnusedUntilLoginSessionIsRetained(t *testing.T) {
	const code = "ABCD-1234"

	t.Run("session limit", func(t *testing.T) {
		user, token := backupLoginChallenge(t, code)
		previousLimit := common.UserSessionActiveLimit
		common.UserSessionActiveLimit = 1
		t.Cleanup(func() { common.UserSessionActiveLimit = previousLimit })

		failed := postBackupLogin(t, token, code, "")
		assert.False(t, failed.Success, failed.Message)
		assert.Equal(t, 1, unusedBackupCodes(t, user.Id))
		assert.EqualValues(t, 1, activeLoginSessions(t, user.Id))

		common.UserSessionActiveLimit = previousLimit
		kept := postBackupLogin(t, token, code, "")
		assert.True(t, kept.Success, kept.Message)
		assert.Equal(t, 0, unusedBackupCodes(t, user.Id))
		assert.EqualValues(t, 2, activeLoginSessions(t, user.Id))
	})

	t.Run("site mismatch", func(t *testing.T) {
		user, token := backupLoginChallenge(t, code)
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
		assert.Equal(t, 1, unusedBackupCodes(t, user.Id))
		assert.EqualValues(t, 1, activeLoginSessions(t, user.Id))

		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("site_id", 0).Error)
		user.SiteId = 0
		next, err := service.StartLoginVerification(user, "password", nil)
		require.NoError(t, err)
		kept := postBackupLogin(t, next.FlowToken, code, "shop.foreign.example")
		assert.True(t, kept.Success, kept.Message)
		assert.Equal(t, 0, unusedBackupCodes(t, user.Id))
		assert.EqualValues(t, 2, activeLoginSessions(t, user.Id))
	})
}

func backupLoginChallenge(t *testing.T, code string) (*model.User, string) {
	t.Helper()
	user, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
	hash, err := common.HashBackupCode(code)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.TwoFABackupCode{UserId: user.Id, CodeHash: hash}).Error)
	challenge, err := service.StartLoginVerification(user, "password", nil)
	require.NoError(t, err)
	require.NotNil(t, challenge)
	return user, challenge.FlowToken
}

func postBackupLogin(t *testing.T, token, code, host string) securityEnrollmentResponse {
	t.Helper()
	body, err := common.Marshal(map[string]string{"flow_token": token, "code": code})
	require.NoError(t, err)
	router := gin.New()
	router.POST("/api/user/login/verify", VerifyLogin)
	response := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/user/login/verify", strings.NewReader(string(body)))
	if host != "" {
		request.Host = host
	}
	router.ServeHTTP(response, request)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	if !result.Success {
		assert.Empty(t, response.Header().Values("Set-Cookie"))
	}
	return result
}

func unusedBackupCodes(t *testing.T, userID int) int {
	t.Helper()
	count, err := model.GetUnusedBackupCodeCount(userID)
	require.NoError(t, err)
	return count
}

func activeLoginSessions(t *testing.T, userID int) int64 {
	t.Helper()
	count, err := model.CountActiveUserSessions(userID, common.GetTimestamp())
	require.NoError(t, err)
	return count
}
