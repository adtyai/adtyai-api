package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRegenerateBackupCodesReturnsPlaintextWhenSessionAdvanceFails(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	const secret = "JBSWY3DPEHPK3PXP"
	const oldCode = "ABCD-1234"
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: secret, IsEnabled: true}).Error)
	hash, err := common.HashBackupCode(oldCode)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.TwoFABackupCode{UserId: user.Id, CodeHash: hash}).Error)
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	proof, err := service.VerifySecurityInput(identity, service.VerificationInput{
		Method: service.VerificationMethodTwoFA,
		Scope:  service.VerificationScopeTwoFABackupCodes,
		Code:   code,
	})
	require.NoError(t, err)

	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("regen_session_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_sessions" {
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove("regen_session_fail") })

	response := securityEnrollmentRequest("POST", "/api/user/2fa/backup_codes", `{}`, proof.ProofToken, identity, RegenerateBackupCodes)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	var data struct {
		BackupCodes []string `json:"backup_codes"`
	}
	require.NoError(t, common.Unmarshal(result.Data, &data))
	require.NotEmpty(t, data.BackupCodes)

	stillOld, err := model.MatchUnusedBackupCode(user.Id, oldCode)
	require.NoError(t, err)
	assert.False(t, stillOld, "the previous backup code was replaced")
	for _, generated := range data.BackupCodes {
		matched, err := model.MatchUnusedBackupCode(user.Id, generated)
		require.NoError(t, err)
		assert.True(t, matched, generated)
	}
}
