package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTopUpVerificationUsers(t *testing.T) (admin, root *model.User, adminID, rootID AuthIdentity) {
	t.Helper()
	user, _ := setupSecurityStepUpTest(t)
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"role": common.RoleAdminUser, "username": "topup-admin"}).Error)
	require.NoError(t, model.PublishUserAuthCache(user.Id))
	adminBundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "topup-admin-test")
	require.NoError(t, err)
	adminID, err = ParseAccessToken(adminBundle.AccessToken)
	require.NoError(t, err)

	password, err := common.Password2Hash("step-up-password")
	require.NoError(t, err)
	rootUser := &model.User{
		Username: "topup-root", Password: password, Email: "topup-root@example.com",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(rootUser).Error)
	require.NoError(t, model.PublishUserAuthCache(rootUser.Id))
	rootBundle, err := CreateLoginSession(rootUser.Id, "password", "127.0.0.1", "topup-root-test")
	require.NoError(t, err)
	rootID, err = ParseAccessToken(rootBundle.AccessToken)
	require.NoError(t, err)
	return user, rootUser, adminID, rootID
}

func TestAdtyTopUpVerificationScopesUseImportantOperationPolicy(t *testing.T) {
	admin, _, adminID, rootID := setupTopUpVerificationUsers(t)
	enableStepUpFactors(t, admin.Id, true, false, true)

	for _, scope := range []string{VerificationScopeAdtyTopUpRefund, VerificationScopeAdtyTopUpManualComplete} {
		t.Run(scope+"/admin", func(t *testing.T) {
			requirements, err := GetVerificationRequirements(adminID, scope)
			require.NoError(t, err)
			assert.Equal(t, []string{VerificationMethodPassword, VerificationMethodEmailCode, VerificationMethodTwoFA, VerificationMethodPasskey}, methodNames(requirements))
			assert.True(t, requirements.Methods[0].Available)
			assert.True(t, requirements.Methods[1].Available)
			_, err = RequireVerificationMethod(adminID, scope, VerificationMethodPassword)
			require.NoError(t, err)
			_, err = RequireVerificationMethod(adminID, scope, VerificationMethodEmailCode)
			require.NoError(t, err)
		})
		t.Run(scope+"/root", func(t *testing.T) {
			requirements, err := GetVerificationRequirements(rootID, scope)
			require.NoError(t, err)
			require.Len(t, requirements.Methods, 1)
			assert.Equal(t, VerificationMethodRootExempt, requirements.Methods[0].Method)
			_, err = RequireVerificationMethod(rootID, scope, VerificationMethodPassword)
			assert.ErrorIs(t, err, ErrProofMethod)
			_, err = RequireVerificationMethod(rootID, scope, VerificationMethodRootExempt)
			require.NoError(t, err)
		})
	}
}

func TestAdtyTopUpVerificationContextBinding(t *testing.T) {
	_, err := BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "100", nil))
	require.NoError(t, err)
	quota := int64(50_000_000)
	_, err = BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "", &quota))
	require.NoError(t, err)
	_, err = BindVerificationOperation(AdtyTopUpManualCompleteOperation("EPU-1", "0xabc", "checked"))
	require.NoError(t, err)

	_, err = BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "100", &quota))
	assert.ErrorIs(t, err, ErrVerificationContextInvalid, "usd and quota together are invalid")
	_, err = BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "", nil))
	assert.ErrorIs(t, err, ErrVerificationContextInvalid)
	_, err = BindVerificationOperation(AdtyTopUpRefundOperation("", "duplicate", "100", nil))
	assert.ErrorIs(t, err, ErrVerificationContextInvalid)
	_, err = BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "not-a-number", nil))
	assert.ErrorIs(t, err, ErrVerificationContextInvalid)
	_, err = BindVerificationOperation(AdtyTopUpManualCompleteOperation("EPU-1", "", "checked"))
	assert.ErrorIs(t, err, ErrVerificationContextInvalid)

	sameUSD, err := BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "100.00", nil))
	require.NoError(t, err)
	canonical, err := BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "100", nil))
	require.NoError(t, err)
	assert.Equal(t, sameUSD.ContextHash, canonical.ContextHash, "equivalent USD amounts share a digest")

	otherAmount, err := BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "duplicate", "50", nil))
	require.NoError(t, err)
	assert.NotEqual(t, canonical.ContextHash, otherAmount.ContextHash)
	otherOrder, err := BindVerificationOperation(AdtyTopUpRefundOperation("EPU-2", "duplicate", "100", nil))
	require.NoError(t, err)
	assert.NotEqual(t, canonical.ContextHash, otherOrder.ContextHash)
	otherReason, err := BindVerificationOperation(AdtyTopUpRefundOperation("EPU-1", "other", "100", nil))
	require.NoError(t, err)
	assert.NotEqual(t, canonical.ContextHash, otherReason.ContextHash)
}

func TestAdtyTopUpVerificationProofIsBoundAndSingleUse(t *testing.T) {
	_, _, adminID, rootID := setupTopUpVerificationUsers(t)
	operation := AdtyTopUpRefundOperation("EPU-bound", "duplicate", "100", nil)
	binding, err := BindVerificationOperation(operation)
	require.NoError(t, err)

	proof, err := CompleteSecurityVerification(adminID, binding, VerificationMethodPassword)
	require.NoError(t, err)
	_, err = ConsumeOperationProof(proof.ProofToken, adminID, AdtyTopUpRefundOperation("EPU-other", "duplicate", "100", nil))
	assert.ErrorIs(t, err, ErrProofContext)
	_, err = ConsumeOperationProof(proof.ProofToken, adminID, AdtyTopUpRefundOperation("EPU-bound", "duplicate", "50", nil))
	assert.ErrorIs(t, err, ErrProofContext)
	_, err = ConsumeOperationProof(proof.ProofToken, adminID, AdtyTopUpManualCompleteOperation("EPU-bound", "0xabc", "checked"))
	assert.ErrorIs(t, err, ErrProofScope)
	_, err = ConsumeOperationProof(proof.ProofToken, adminID, operation)
	require.NoError(t, err)
	_, err = ConsumeOperationProof(proof.ProofToken, adminID, operation)
	assert.ErrorIs(t, err, ErrProofConsumed)

	rootProof, err := CompleteSecurityVerification(rootID, binding, VerificationMethodRootExempt)
	require.NoError(t, err)
	assert.Equal(t, VerificationMethodRootExempt, rootProof.Method)
	_, err = ConsumeOperationProof(rootProof.ProofToken, rootID, operation)
	require.NoError(t, err)
}

func TestAdtyTopUpVerificationRejectsNonAdmin(t *testing.T) {
	_, identity := setupSecurityStepUpTest(t)
	_, err := GetVerificationRequirements(identity, VerificationScopeAdtyTopUpRefund)
	assert.ErrorIs(t, err, ErrVerificationForbidden)
	_, err = GetVerificationRequirements(identity, VerificationScopeAdtyTopUpManualComplete)
	assert.ErrorIs(t, err, ErrVerificationForbidden)
}
