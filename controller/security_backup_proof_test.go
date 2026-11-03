package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBackupCodeSurvivesSecurityProofFailure(t *testing.T) {
	const code = "ABCD-1234"

	t.Run("proof insert rolls back", func(t *testing.T) {
		user, identity := setupSecurityEnrollmentTest(t)
		require.NoError(t, enableBackupCode(t, user.Id, code))
		failInsert := true
		require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("security_proof_insert_fail", func(tx *gorm.DB) {
			if failInsert && tx.Statement.Table == "auth_flows" {
				tx.AddError(assert.AnError)
			}
		}))
		t.Cleanup(func() { _ = model.DB.Callback().Create().Remove("security_proof_insert_fail") })

		_, err := service.VerifySecurityInput(identity, backupProofInput(code))
		assert.Error(t, err)
		assert.Equal(t, 1, unusedBackupCodes(t, user.Id))

		failInsert = false
		proof, err := service.VerifySecurityInput(identity, backupProofInput(code))
		require.NoError(t, err)
		assert.NotEmpty(t, proof.ProofToken)
		assert.Equal(t, 0, unusedBackupCodes(t, user.Id))
	})

	t.Run("revoked session", func(t *testing.T) {
		user, identity := setupSecurityEnrollmentTest(t)
		require.NoError(t, enableBackupCode(t, user.Id, code))
		revoked, err := model.RevokeUserSession(identity.UserID, identity.SessionID, "test")
		require.NoError(t, err)
		require.True(t, revoked)

		_, err = service.VerifySecurityInput(identity, backupProofInput(code))
		assert.Error(t, err)
		assert.Equal(t, 1, unusedBackupCodes(t, user.Id))
	})
}

func enableBackupCode(t *testing.T, userID int, code string) error {
	t.Helper()
	if err := model.DB.Create(&model.TwoFA{UserId: userID, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error; err != nil {
		return err
	}
	hash, err := common.HashBackupCode(code)
	if err != nil {
		return err
	}
	return model.DB.Create(&model.TwoFABackupCode{UserId: userID, CodeHash: hash}).Error
}

func backupProofInput(code string) service.VerificationInput {
	return service.VerificationInput{
		Method: service.VerificationMethodTwoFA,
		Scope:  service.VerificationScopePasswordChange,
		Code:   code,
	}
}
