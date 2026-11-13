package controller

import (
	"regexp"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hookSecurityStepUpMail(t *testing.T, hook func(*model.AdtySite, string, string, string) error) {
	t.Helper()
	restore := service.SetSecurityStepUpEmailSenderForTest(hook)
	t.Cleanup(restore)
}

func TestVerifyMethodsAdvertisePasswordAndEmailCodeWhenFactorsEnabled(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "step-up@example.com").Error)
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.PasskeyCredential{UserID: user.Id, CredentialID: "step-up-key", PublicKey: "public-key"}).Error)

	response := securityEnrollmentRequest("GET", "/api/verify/methods?scope=account.password.change", "", "", identity, GetVerificationMethods)
	body := decodeSecurityEnrollmentResponse(t, response)
	require.True(t, body.Success, response.Body.String())
	var requirements service.VerificationRequirements
	require.NoError(t, common.Unmarshal(body.Data, &requirements))
	require.GreaterOrEqual(t, len(requirements.Methods), 2)
	assert.Equal(t, service.VerificationMethodPassword, requirements.Methods[0].Method)
	assert.True(t, requirements.Methods[0].Available)
	assert.Equal(t, service.VerificationMethodEmailCode, requirements.Methods[1].Method)
	assert.True(t, requirements.Methods[1].Available)
}

func TestVerifyEmailCodeIssuesSingleUseProof(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "step-up@example.com").Error)
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)

	var sent string
	hookSecurityStepUpMail(t, func(_ *model.AdtySite, _, _, content string) error {
		sent = content
		return nil
	})

	issued := securityEnrollmentRequest("POST", "/api/verify/email-code", `{"scope":"account.password.change"}`, "", identity, SendSecurityEmailCode)
	body := decodeSecurityEnrollmentResponse(t, issued)
	require.True(t, body.Success, issued.Body.String())
	match := regexp.MustCompile(`<strong>([^<]+)</strong>`).FindStringSubmatch(sent)
	require.Len(t, match, 2)
	code := match[1]

	verified := securityEnrollmentRequest("POST", "/api/verify", `{"method":"email_code","scope":"account.password.change","code":"`+code+`"}`, "", identity, UniversalVerify)
	body = decodeSecurityEnrollmentResponse(t, verified)
	require.True(t, body.Success, verified.Body.String())
	var proof service.SecurityProof
	require.NoError(t, common.Unmarshal(body.Data, &proof))
	assert.Equal(t, service.VerificationMethodEmailCode, proof.Method)
	_, err := service.ConsumeOperationProof(proof.ProofToken, identity, service.VerificationOperation{Scope: service.VerificationScopePasswordChange})
	require.NoError(t, err)

	replay := securityEnrollmentRequest("POST", "/api/verify", `{"method":"email_code","scope":"account.password.change","code":"`+code+`"}`, "", identity, UniversalVerify)
	assert.Contains(t, replay.Body.String(), `"success":false`)
}

func TestVerifyPasswordStillWorksWhenTwoFAEnabled(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "step-up@example.com").Error)
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)

	verified := securityEnrollmentRequest("POST", "/api/verify", `{"method":"password","scope":"account.password.change","password":"enrollment-password"}`, "", identity, UniversalVerify)
	body := decodeSecurityEnrollmentResponse(t, verified)
	require.True(t, body.Success, verified.Body.String())
	var proof service.SecurityProof
	require.NoError(t, common.Unmarshal(body.Data, &proof))
	assert.Equal(t, service.VerificationMethodPassword, proof.Method)
	_, err := service.ConsumeOperationProof(proof.ProofToken, identity, service.VerificationOperation{Scope: service.VerificationScopePasswordChange})
	require.NoError(t, err)
}

func TestVerifyEmailCodeIsScopedToRequestSite(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "step-up@example.com").Error)
	site := &model.AdtySite{OwnerUserId: user.Id, Subdomain: "step-up-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, model.DB.Create(site).Error)

	var sent string
	hookSecurityStepUpMail(t, func(_ *model.AdtySite, _, _, content string) error {
		sent = content
		return nil
	})

	issued := securityEnrollmentRequest("POST", "/api/verify/email-code", `{"scope":"account.password.change"}`, "", identity, func(c *gin.Context) {
		c.Set(service.AdtySiteContextKey, site)
		SendSecurityEmailCode(c)
	})
	require.True(t, decodeSecurityEnrollmentResponse(t, issued).Success, issued.Body.String())
	match := regexp.MustCompile(`<strong>([^<]+)</strong>`).FindStringSubmatch(sent)
	require.Len(t, match, 2)
	code := match[1]

	// Main site (site ID 0) must not accept a code issued for the request site.
	wrongSite := securityEnrollmentRequest("POST", "/api/verify", `{"method":"email_code","scope":"account.password.change","code":"`+code+`"}`, "", identity, UniversalVerify)
	assert.Contains(t, wrongSite.Body.String(), `"success":false`)

	sameSite := securityEnrollmentRequest("POST", "/api/verify", `{"method":"email_code","scope":"account.password.change","code":"`+code+`"}`, "", identity, func(c *gin.Context) {
		c.Set(service.AdtySiteContextKey, site)
		UniversalVerify(c)
	})
	body := decodeSecurityEnrollmentResponse(t, sameSite)
	require.True(t, body.Success, sameSite.Body.String())
}

func TestRootVerifyMethodsStayExempt(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"role": common.RoleRootUser, "email": "root@example.com"}).Error)
	response := securityEnrollmentRequest("GET", "/api/verify/methods?scope=account.password.change", "", "", identity, GetVerificationMethods)
	body := decodeSecurityEnrollmentResponse(t, response)
	require.True(t, body.Success, response.Body.String())
	var requirements service.VerificationRequirements
	require.NoError(t, common.Unmarshal(body.Data, &requirements))
	assert.Equal(t, []service.VerificationMethodOption{{Method: service.VerificationMethodRootExempt, Available: true}}, requirements.Methods)

	issued := securityEnrollmentRequest("POST", "/api/verify/email-code", `{"scope":"account.password.change"}`, "", identity, SendSecurityEmailCode)
	assert.Contains(t, issued.Body.String(), `"success":false`)
}
