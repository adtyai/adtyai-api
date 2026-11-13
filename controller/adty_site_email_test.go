package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSiteEmailControllerDB(t *testing.T) (*model.User, *model.User, *model.AdtySite) {
	t.Helper()
	require.NoError(t, i18n.Init())
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.UserAccessToken{}, &model.AdtySite{}, &model.AuditLog{}))
	require.NoError(t, model.EnsureSiteEmailUniqueIndex(db))
	hash, err := common.HashAccountPassword("MainPass123")
	require.NoError(t, err)
	siteHash, err := common.HashAccountPassword("SitePass123")
	require.NoError(t, err)
	mainUser := &model.User{
		Username: "site-email-main", Password: hash, Email: "shared-ctrl@example.com",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, db.Create(mainUser).Error)
	site := &model.AdtySite{OwnerUserId: mainUser.Id, Subdomain: "mailsite", RootDomain: "sites.example.test", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	siteUser := &model.User{
		Username: "site-email-sub", Password: siteHash, Email: "shared-ctrl@example.com",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, SiteId: site.Id,
	}
	require.NoError(t, db.Create(siteUser).Error)
	return mainUser, siteUser, site
}

func callEmailHandlerOnSite(handler gin.HandlerFunc, path string, site *model.AdtySite) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if site != nil {
		c.Set(service.AdtySiteContextKey, site)
	}
	handler(c)
	return recorder
}

func postPasswordResetOnSite(email, token, password string, site *model.AdtySite) string {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body, _ := common.Marshal(map[string]string{"email": email, "token": token, "password": password})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/reset", strings.NewReader(string(body)))
	if site != nil {
		c.Set(service.AdtySiteContextKey, site)
	}
	ResetPassword(c)
	return recorder.Body.String()
}

func TestVerificationAndPasswordResetCodesStayOnOneSite(t *testing.T) {
	mainUser, siteUser, site := setupSiteEmailControllerDB(t)
	sent := captureAdtyMail(t)
	previousSite := service.AdtySendSiteEmailAsync
	service.AdtySendSiteEmailAsync = func(_ *model.AdtySite, subject, to, content string) {
		service.AdtySendEmailAsync(subject, to, content)
	}
	t.Cleanup(func() { service.AdtySendSiteEmailAsync = previousSite })

	freshMain := callEmailHandlerOnSite(SendEmailVerification, "/api/verification?email="+url.QueryEscape("new-ctrl@example.com"), nil)
	takenMain := callEmailHandlerOnSite(SendEmailVerification, "/api/verification?email="+url.QueryEscape(mainUser.Email), nil)
	assert.Equal(t, freshMain.Code, takenMain.Code)
	assert.JSONEq(t, freshMain.Body.String(), takenMain.Body.String())
	mails := sent()
	require.GreaterOrEqual(t, len(mails), 2)
	assert.Contains(t, mails[len(mails)-1].content, "sign in or reset your password")

	freshSite := callEmailHandlerOnSite(SendEmailVerification, "/api/verification?email="+url.QueryEscape("fresh-on-site@example.com"), site)
	// The shared address is already registered on this sub-site, so this request
	// must also send a reminder rather than a usable registration code.
	takenSite := callEmailHandlerOnSite(SendEmailVerification, "/api/verification?email="+url.QueryEscape(siteUser.Email), site)
	assert.Equal(t, freshSite.Code, takenSite.Code)
	mails = sent()
	var siteReminder sentMail
	for _, mail := range mails {
		if mail.to == siteUser.Email && strings.Contains(mail.content, "sign in or reset your password") {
			siteReminder = mail
		}
	}
	assert.Contains(t, siteReminder.content, "sign in or reset your password")
	assert.False(t, common.VerifyCodeWithKey(model.SiteEmailCodeKey(siteUser.Email, site.Id), "000000", common.EmailVerificationPurpose))

	// A code issued for registration on the main site must not register on the sub-site.
	require.NoError(t, common.RegisterVerificationCodeWithKey(model.SiteEmailCodeKey("open@example.com", 0), "111111", common.EmailVerificationPurpose))
	assert.False(t, common.VerifyCodeWithKey(model.SiteEmailCodeKey("open@example.com", site.Id), "111111", common.EmailVerificationPurpose))
	assert.True(t, common.VerifyCodeWithKey(model.SiteEmailCodeKey("open@example.com", 0), "111111", common.EmailVerificationPurpose))

	resetMain := callEmailHandlerOnSite(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape(mainUser.Email), nil)
	resetSite := callEmailHandlerOnSite(SendPasswordResetEmail, "/api/reset_password?email="+url.QueryEscape(siteUser.Email), site)
	assert.Equal(t, http.StatusOK, resetMain.Code)
	assert.Equal(t, resetMain.Code, resetSite.Code)
	mails = sent()
	var mainToken, siteToken string
	for _, mail := range mails {
		if !strings.Contains(mail.content, "token=") {
			continue
		}
		_, after, found := strings.Cut(mail.content, "token=")
		if !found {
			continue
		}
		token := after[:strings.IndexAny(after, `"&<' `)]
		if strings.Contains(mail.content, "/user/reset") && mainToken == "" && (site == nil || !strings.Contains(mail.content, site.Subdomain)) {
			mainToken = token
		}
		if site != nil && strings.Contains(mail.content, site.Subdomain) {
			siteToken = token
		}
	}
	// Branding may use ServerAddress when reseller cache is empty, so fall back
	// to reading tokens from the last two reset mails if host matching failed.
	if mainToken == "" || siteToken == "" {
		var resetMails []sentMail
		for _, mail := range mails {
			if strings.Contains(mail.content, "token=") {
				resetMails = append(resetMails, mail)
			}
		}
		require.GreaterOrEqual(t, len(resetMails), 2)
		_, after, _ := strings.Cut(resetMails[len(resetMails)-2].content, "token=")
		if mainToken == "" {
			mainToken = after[:strings.IndexAny(after, `"&<' `)]
		}
		_, after, _ = strings.Cut(resetMails[len(resetMails)-1].content, "token=")
		if siteToken == "" {
			siteToken = after[:strings.IndexAny(after, `"&<' `)]
		}
	}
	require.NotEmpty(t, mainToken)
	require.NotEmpty(t, siteToken)
	assert.NotEqual(t, mainToken, siteToken)

	// Using the main-site token on the sub-site must not change either password.
	cross := postPasswordResetOnSite(mainUser.Email, mainToken, "CrossedPass123", site)
	assert.Contains(t, cross, "Password reset link is invalid or has expired")
	require.NoError(t, model.DB.First(mainUser, mainUser.Id).Error)
	require.NoError(t, model.DB.First(siteUser, siteUser.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("MainPass123", mainUser.Password))
	assert.True(t, common.ValidatePasswordAndHash("SitePass123", siteUser.Password))

	okMain := postPasswordResetOnSite(mainUser.Email, mainToken, "MainReset123", nil)
	assert.Contains(t, okMain, `"success":true`)
	require.NoError(t, model.DB.First(mainUser, mainUser.Id).Error)
	require.NoError(t, model.DB.First(siteUser, siteUser.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("MainReset123", mainUser.Password))
	assert.True(t, common.ValidatePasswordAndHash("SitePass123", siteUser.Password))

	okSite := postPasswordResetOnSite(siteUser.Email, siteToken, "SiteReset123", site)
	assert.Contains(t, okSite, `"success":true`)
	require.NoError(t, model.DB.First(mainUser, mainUser.Id).Error)
	require.NoError(t, model.DB.First(siteUser, siteUser.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("MainReset123", mainUser.Password))
	assert.True(t, common.ValidatePasswordAndHash("SiteReset123", siteUser.Password))

	replay := postPasswordResetOnSite(siteUser.Email, siteToken, "ReplayedPass123", site)
	assert.Contains(t, replay, "Password reset link is invalid or has expired")
	require.NoError(t, model.DB.First(siteUser, siteUser.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("SiteReset123", siteUser.Password))
}
