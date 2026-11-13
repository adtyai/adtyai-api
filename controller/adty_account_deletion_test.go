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
