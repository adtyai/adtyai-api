package service

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSecurityStepUpTest(t *testing.T) (*model.User, AuthIdentity) {
	t.Helper()
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.TwoFA{}, &model.PasskeyCredential{}, &model.AuthFlow{}, &model.AdtySite{}))
	previousSecret, previousRedis, previousPassword := common.SessionSecret, common.RedisEnabled, common.PasswordLoginEnabled
	previousSettings := *system_setting.GetPasskeySettings()
	previousSend := sendSecurityStepUpEmail
	common.SessionSecret = "security-stepup-test-secret"
	common.RedisEnabled = false
	common.PasswordLoginEnabled = true
	*system_setting.GetPasskeySettings() = system_setting.PasskeySettings{Enabled: true, RPID: "example.com", Origins: "https://example.com", RPDisplayName: "new-api"}
	t.Cleanup(func() {
		common.SessionSecret, common.RedisEnabled, common.PasswordLoginEnabled = previousSecret, previousRedis, previousPassword
		*system_setting.GetPasskeySettings() = previousSettings
		sendSecurityStepUpEmail = previousSend
	})
	password, err := common.Password2Hash("step-up-password")
	require.NoError(t, err)
	user := &model.User{
		Username: "step-up-user", Password: password, Email: "step-up@example.com",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, model.PublishUserAuthCache(user.Id))
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "step-up-test")
	require.NoError(t, err)
	identity, err := ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)
	return user, identity
}

func enableStepUpFactors(t *testing.T, userID int, twoFA, locked, passkey bool) {
	t.Helper()
	if twoFA {
		factor := &model.TwoFA{UserId: userID, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}
		if locked {
			until := time.Now().Add(time.Minute)
			factor.LockedUntil = &until
		}
		require.NoError(t, model.DB.Create(factor).Error)
	}
	if passkey {
		require.NoError(t, model.DB.Create(&model.PasskeyCredential{UserID: userID, CredentialID: "step-up-key", PublicKey: "public-key"}).Error)
	}
}

func captureStepUpEmail(t *testing.T) *struct{ subject, to, content string } {
	t.Helper()
	sent := &struct{ subject, to, content string }{}
	sendSecurityStepUpEmail = func(_ *model.AdtySite, subject, to, content string) error {
		sent.subject, sent.to, sent.content = subject, to, content
		return nil
	}
	return sent
}

func extractStepUpCode(t *testing.T, content string) string {
	t.Helper()
	match := regexp.MustCompile(`<strong>([^<]+)</strong>`).FindStringSubmatch(content)
	require.Len(t, match, 2, "email body %q", content)
	return match[1]
}

func methodNames(requirements *VerificationRequirements) []string {
	names := make([]string, 0, len(requirements.Methods))
	for _, option := range requirements.Methods {
		names = append(names, option.Method)
	}
	return names
}

func TestImportantOperationStepUpOffersPasswordAndEmailCode(t *testing.T) {
	user, identity := setupSecurityStepUpTest(t)
	enableStepUpFactors(t, user.Id, true, false, true)

	for _, scope := range []string{VerificationScopePasswordChange, VerificationScopeAdtySiteOwnerManage} {
		t.Run(scope, func(t *testing.T) {
			requirements, err := GetVerificationRequirements(identity, scope)
			require.NoError(t, err)
			assert.Equal(t, []string{VerificationMethodPassword, VerificationMethodEmailCode, VerificationMethodTwoFA, VerificationMethodPasskey}, methodNames(requirements))
			assert.True(t, requirements.Methods[0].Available)
			assert.True(t, requirements.Methods[1].Available)
			_, err = RequireVerificationMethod(identity, scope, VerificationMethodPassword)
			require.NoError(t, err)
			_, err = RequireVerificationMethod(identity, scope, VerificationMethodEmailCode)
			require.NoError(t, err)
		})
	}
}

func TestImportantOperationStepUpCompletesEitherPasswordOrEmailCode(t *testing.T) {
	user, identity := setupSecurityStepUpTest(t)
	enableStepUpFactors(t, user.Id, true, false, true)
	operation := VerificationOperation{Scope: VerificationScopePasswordChange}

	passwordProof, err := VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodPassword, Scope: operation.Scope, Password: "step-up-password",
	})
	require.NoError(t, err)
	require.NotEmpty(t, passwordProof.ProofToken)
	assert.Equal(t, VerificationMethodPassword, passwordProof.Method)
	_, err = ConsumeOperationProof(passwordProof.ProofToken, identity, operation)
	require.NoError(t, err)
	_, err = ConsumeOperationProof(passwordProof.ProofToken, identity, operation)
	assert.ErrorIs(t, err, ErrProofConsumed)

	sent := captureStepUpEmail(t)
	require.NoError(t, IssueSecurityEmailCode(identity, operation.Scope, nil, "en"))
	require.Equal(t, "step-up@example.com", sent.to)
	assert.NotContains(t, sent.subject, "adty.email.security_stepup_subject")
	code := extractStepUpCode(t, sent.content)

	// A registration-purpose code for the same address must not complete step-up.
	require.NoError(t, common.RegisterVerificationCodeWithKey(model.SiteEmailCodeKey("step-up@example.com", 0), code, common.EmailVerificationPurpose))
	emailProof, err := VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodEmailCode, Scope: operation.Scope, Code: code, SiteID: 0,
	})
	require.NoError(t, err)
	require.NotEmpty(t, emailProof.ProofToken)
	assert.Equal(t, VerificationMethodEmailCode, emailProof.Method)
	_, err = ConsumeOperationProof(emailProof.ProofToken, identity, operation)
	require.NoError(t, err)
	_, err = ConsumeOperationProof(emailProof.ProofToken, identity, operation)
	assert.ErrorIs(t, err, ErrProofConsumed)
	// The registration code is still present; step-up consumed its own purpose-bound key.
	assert.True(t, common.VerifyCodeWithKey(model.SiteEmailCodeKey("step-up@example.com", 0), code, common.EmailVerificationPurpose))
}

func TestLockedTwoFADoesNotRemovePasswordOrEmailCode(t *testing.T) {
	user, identity := setupSecurityStepUpTest(t)
	enableStepUpFactors(t, user.Id, true, true, false)

	requirements, err := GetVerificationRequirements(identity, VerificationScopeAccountBind)
	require.NoError(t, err)
	assert.Equal(t, []string{VerificationMethodPassword, VerificationMethodEmailCode, VerificationMethodTwoFA}, methodNames(requirements))
	assert.True(t, requirements.Methods[0].Available)
	assert.True(t, requirements.Methods[1].Available)
	assert.False(t, requirements.Methods[2].Available)
	_, err = RequireVerificationMethod(identity, VerificationScopeAccountBind, VerificationMethodTwoFA)
	assert.ErrorIs(t, err, ErrVerificationUnavailable)
	_, err = RequireVerificationMethod(identity, VerificationScopeAccountBind, VerificationMethodPassword)
	require.NoError(t, err)
	_, err = RequireVerificationMethod(identity, VerificationScopeAccountBind, VerificationMethodEmailCode)
	require.NoError(t, err)
}

func TestPasswordlessAccountUsesEmailCodeWithoutPassword(t *testing.T) {
	user, identity := setupSecurityStepUpTest(t)
	require.NoError(t, model.DB.Model(user).Update("password", "").Error)

	requirements, err := GetVerificationRequirements(identity, VerificationScopePasswordSet)
	require.NoError(t, err)
	assert.Equal(t, []string{VerificationMethodEmailCode, VerificationMethodOAuth}, methodNames(requirements))
	_, err = RequireVerificationMethod(identity, VerificationScopePasswordSet, VerificationMethodPassword)
	assert.ErrorIs(t, err, ErrProofMethod)

	sent := captureStepUpEmail(t)
	require.NoError(t, IssueSecurityEmailCode(identity, VerificationScopePasswordSet, nil, "en"))
	code := extractStepUpCode(t, sent.content)
	proof, err := VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodEmailCode, Scope: VerificationScopePasswordSet, Code: code,
	})
	require.NoError(t, err)
	_, err = ConsumeOperationProof(proof.ProofToken, identity, VerificationOperation{Scope: VerificationScopePasswordSet})
	require.NoError(t, err)
}

func TestRootImportantOperationStaysExemptFromPasswordAndEmailCode(t *testing.T) {
	user, identity := setupSecurityStepUpTest(t)
	require.NoError(t, model.DB.Model(user).Update("role", common.RoleRootUser).Error)

	requirements, err := GetVerificationRequirements(identity, VerificationScopePasswordChange)
	require.NoError(t, err)
	assert.Equal(t, []VerificationMethodOption{{Method: VerificationMethodRootExempt, Available: true}}, requirements.Methods)
	_, err = RequireVerificationMethod(identity, VerificationScopePasswordChange, VerificationMethodPassword)
	assert.ErrorIs(t, err, ErrProofMethod)
	_, err = RequireVerificationMethod(identity, VerificationScopePasswordChange, VerificationMethodEmailCode)
	assert.ErrorIs(t, err, ErrProofMethod)
	assert.ErrorIs(t, IssueSecurityEmailCode(identity, VerificationScopePasswordChange, nil, "en"), ErrProofMethod)

	proof, err := VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodRootExempt, Scope: VerificationScopePasswordChange,
	})
	require.NoError(t, err)
	_, err = ConsumeOperationProof(proof.ProofToken, identity, VerificationOperation{Scope: VerificationScopePasswordChange})
	require.NoError(t, err)
}

func TestSensitiveProofMethodsMatchesLiveNonRootPolicy(t *testing.T) {
	assert.False(t, SensitiveActionNeedsProof(common.RoleRootUser))
	assert.Empty(t, SensitiveProofMethods(common.RoleRootUser))
	assert.True(t, SensitiveActionNeedsProof(common.RoleCommonUser))
	assert.Equal(t, []string{VerificationMethodPassword, VerificationMethodEmailCode}, SensitiveProofMethods(common.RoleCommonUser))
	assert.Equal(t, []string{VerificationMethodPassword, VerificationMethodEmailCode}, SensitiveProofMethods(common.RoleAdminUser))

	_, identity := setupSecurityStepUpTest(t)
	requirements, err := GetVerificationRequirements(identity, VerificationScopePasswordChange)
	require.NoError(t, err)
	assert.Equal(t, SensitiveProofMethods(common.RoleCommonUser), methodNames(requirements)[:2])
}

func TestSecurityEmailCodeIsSiteScopedSingleUse(t *testing.T) {
	user, identity := setupSecurityStepUpTest(t)
	site := &model.AdtySite{OwnerUserId: user.Id, Subdomain: "step-up-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, model.DB.Create(site).Error)

	sent := captureStepUpEmail(t)
	require.NoError(t, IssueSecurityEmailCode(identity, VerificationScopePasswordChange, site, "en"))
	code := extractStepUpCode(t, sent.content)

	_, err := VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodEmailCode, Scope: VerificationScopePasswordChange, Code: code, SiteID: 0,
	})
	assert.ErrorIs(t, err, ErrVerificationFailed)

	wrong := VerificationInput{Method: VerificationMethodEmailCode, Scope: VerificationScopePasswordChange, Code: "000000", SiteID: site.Id}
	_, err = VerifySecurityInput(identity, wrong)
	assert.ErrorIs(t, err, ErrVerificationFailed)

	proof, err := VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodEmailCode, Scope: VerificationScopePasswordChange, Code: code, SiteID: site.Id,
	})
	require.NoError(t, err)
	_, err = VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodEmailCode, Scope: VerificationScopePasswordChange, Code: code, SiteID: site.Id,
	})
	assert.ErrorIs(t, err, ErrVerificationFailed, "a consumed code must not be reusable")
	_, err = ConsumeOperationProof(proof.ProofToken, identity, VerificationOperation{Scope: VerificationScopePasswordChange})
	require.NoError(t, err)
}

func TestSecurityEmailCodeSendFailureLeavesNoUsableCode(t *testing.T) {
	_, identity := setupSecurityStepUpTest(t)
	sendSecurityStepUpEmail = func(*model.AdtySite, string, string, string) error {
		return assert.AnError
	}
	assert.ErrorIs(t, IssueSecurityEmailCode(identity, VerificationScopePasswordChange, nil, "en"), ErrSecurityEmailCodeDelivery)
	_, err := VerifySecurityInput(identity, VerificationInput{
		Method: VerificationMethodEmailCode, Scope: VerificationScopePasswordChange, Code: "123456",
	})
	assert.ErrorIs(t, err, ErrVerificationFailed)
}

func TestLoginScopeIsUnchangedByImportantOperationPolicy(t *testing.T) {
	_, identity := setupSecurityStepUpTest(t)
	_, err := GetVerificationRequirements(identity, VerificationScopeLogin)
	assert.ErrorIs(t, err, ErrProofScope)
}

func TestSecurityStepUpCodeKeyUsesSiteEmailCodeKey(t *testing.T) {
	assert.True(t, strings.HasPrefix(SecurityStepUpCodeKey(" User@Example.com ", 8, 42, VerificationScopePasswordChange), model.SiteEmailCodeKey("User@Example.com", 8)))
	assert.NotEqual(t, SecurityStepUpCodeKey("user@example.com", 0, 1, VerificationScopePasswordChange), SecurityStepUpCodeKey("user@example.com", 8, 1, VerificationScopePasswordChange))
}
