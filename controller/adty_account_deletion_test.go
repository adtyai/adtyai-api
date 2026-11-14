package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func accountDeletionBody(t *testing.T, user *model.User) string {
	t.Helper()
	email := user.Email
	if email == "" {
		email = fmt.Sprintf("delete-%d@example.com", user.Id)
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("email", email).Error)
		user.Email = email
	}
	code := "246810"
	require.NoError(t, common.RegisterVerificationCodeWithKey(accountDeletionCodeKey(user.Id, email), code, common.AccountDeletionPurpose))
	return `{"verification_code":"` + code + `"}`
}

func TestAccountDeletionRequiresASingleUseEmailCode(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "owner@example.com").Error)
	operation := service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}
	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)

	missing := securityEnrollmentRequest("DELETE", "/api/user/self", `{}`, proof, identity, DeleteSelf)
	assert.Equal(t, http.StatusOK, missing.Code)
	assert.Contains(t, missing.Body.String(), `"success":false`)
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.False(t, stored.DeletedAt.Valid, "a missing email code must not delete the account")

	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	require.NoError(t, common.RegisterVerificationCodeWithKey(accountDeletionCodeKey(user.Id, "owner@example.com"), "135790", common.AccountDeletionPurpose))
	wrong := securityEnrollmentRequest("DELETE", "/api/user/self", `{"verification_code":"000000"}`, proof, identity, DeleteSelf)
	assert.Contains(t, wrong.Body.String(), `"success":false`)
	stored, err = model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.False(t, stored.DeletedAt.Valid, "a wrong email code must not delete the account")

	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	ok := securityEnrollmentRequest("DELETE", "/api/user/self", `{"verification_code":"135790"}`, proof, identity, DeleteSelf)
	assert.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
}

func TestAccountDeletionCodeSendFailureLeavesNoCode(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "owner@example.com").Error)
	previous := sendAccountDeletionEmail
	sendAccountDeletionEmail = func(*model.AdtySite, string, string, string) error {
		return assert.AnError
	}
	t.Cleanup(func() { sendAccountDeletionEmail = previous })

	failed := securityEnrollmentRequest("POST", "/api/user/self/deletion-code", `{}`, "", identity, SendAccountDeletionCode)
	assert.Contains(t, failed.Body.String(), `"success":false`)
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.False(t, stored.DeletedAt.Valid)

	sendAccountDeletionEmail = func(*model.AdtySite, string, string, string) error { return nil }
	sent := securityEnrollmentRequest("POST", "/api/user/self/deletion-code", `{}`, "", identity, SendAccountDeletionCode)
	assert.Contains(t, sent.Body.String(), `"success":true`, sent.Body.String())
	assert.Contains(t, sent.Body.String(), `"data"`)
}

func TestAccountDeletionCodeLocksAfterMaxAttempts(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "owner@example.com").Error)
	operation := service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}
	require.NoError(t, common.RegisterVerificationCodeWithKey(accountDeletionCodeKey(user.Id, "owner@example.com"), "135790", common.AccountDeletionPurpose))

	for attempt := 1; attempt <= common.MaxFailAttempts; attempt++ {
		proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
		wrong := securityEnrollmentRequest("DELETE", "/api/user/self", `{"verification_code":"000000"}`, proof, identity, DeleteSelf)
		assert.Contains(t, wrong.Body.String(), `"success":false`)
		if attempt < common.MaxFailAttempts {
			assert.Contains(t, wrong.Body.String(), "Email confirmation code is invalid")
		} else {
			assert.Contains(t, wrong.Body.String(), service.ErrVerificationCodeLocked.Error())
		}
	}

	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	locked := securityEnrollmentRequest("DELETE", "/api/user/self", `{"verification_code":"135790"}`, proof, identity, DeleteSelf)
	assert.Contains(t, locked.Body.String(), `"success":false`)
	assert.Contains(t, locked.Body.String(), service.ErrVerificationCodeLocked.Error())
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.False(t, stored.DeletedAt.Valid, "a locked email code must not delete the account")

	require.NoError(t, common.RegisterVerificationCodeWithKey(accountDeletionCodeKey(user.Id, "owner@example.com"), "246810", common.AccountDeletionPurpose))
	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	ok := securityEnrollmentRequest("DELETE", "/api/user/self", `{"verification_code":"246810"}`, proof, identity, DeleteSelf)
	assert.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
}
