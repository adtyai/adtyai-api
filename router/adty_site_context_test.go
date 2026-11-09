package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 按 Host 识别分站、分站注册和登录、/api/status 覆盖（任务 11.2）。

const siteLoginFailure = "Username or password is incorrect, or user has been banned"

type siteFixture struct {
	db      *gorm.DB
	engine  *gin.Engine
	site    *model.AdtySite
	closed  *model.AdtySite
	pending *model.AdtySite
}

func newSiteFixture(t *testing.T) *siteFixture {
	t.Helper()
	require.NoError(t, i18n.Init())
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.AuthFlow{}, &model.TwoFA{},
		&model.TwoFABackupCode{}, &model.PasskeyCredential{}, &model.AuditLog{}, &model.Log{}, &model.Option{}, &model.TopUp{},
		&model.UserAccessToken{}, &model.Channel{}, &model.Ability{}))
	require.NoError(t, model.MigrateAdtyTables())
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(0.0), new(0.0)
	oldAddress, oldGenerate := system_setting.ServerAddress, constant.GenerateDefaultToken
	system_setting.ServerAddress, constant.GenerateDefaultToken = "https://main.example.com", false
	t.Cleanup(func() { system_setting.ServerAddress, constant.GenerateDefaultToken = oldAddress, oldGenerate })

	owner := model.User{Username: "site-owner", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&owner).Error)
	site, err := service.OpenAdtySite(owner.Id, "shop", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("brand", `{"name":"Shop AI","logo_url":"/uploads/site-assets/shop.png"}`).Error)

	other := model.User{Username: "closed-owner", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&other).Error)
	closed, err := service.OpenAdtySite(other.Id, "gone", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	_, err = service.ScheduleAdtySiteClose(closed.Id, 1, service.AdtySiteClosePlan{TransferTo: site.Id, Immediate: true})
	require.NoError(t, err)

	applicant := model.User{Username: "pending-owner", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&applicant).Error)
	adty_setting.GetLegalSetting().SiteAgreementVersion = 1
	pending, err := service.ApplyAdtySite(applicant.Id, "later", "", "1", "")
	require.NoError(t, err)
	service.InvalidateAdtySiteCache()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.AdtySiteContext())
	SetApiRouter(engine)
	SetRelayRouter(engine)
	engine.NoRoute(func(c *gin.Context) { c.String(http.StatusOK, "index") })
	site, _ = service.GetAdtySite(site.Id)
	closed, _ = service.GetAdtySite(closed.Id)
	return &siteFixture{db: db, engine: engine, site: site, closed: closed, pending: pending}
}

func (f *siteFixture) do(t *testing.T, method, host, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Host = host
	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, req)
	return recorder
}

func (f *siteFixture) user(t *testing.T, name string, siteId int) model.User {
	t.Helper()
	hash, err := common.HashAccountPassword("correct-horse-1")
	require.NoError(t, err)
	user := model.User{Username: name, Password: hash, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, SiteId: siteId}
	require.NoError(t, f.db.Create(&user).Error)
	return user
}

func siteOf(t *testing.T, engine *gin.Engine, host string) *model.AdtySite {
	t.Helper()
	var captured *model.AdtySite
	probe := gin.New()
	probe.Use(middleware.AdtySiteContext())
	probe.GET("/", func(c *gin.Context) { captured = service.AdtySiteForRequest(c) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = host
	probe.ServeHTTP(httptest.NewRecorder(), req)
	return captured
}

func TestSiteContextBySubdomain(t *testing.T) {
	f := newSiteFixture(t)
	for _, tc := range []struct {
		host string
		want int
	}{
		{"shop.sites.example.com", f.site.Id},
		{"SHOP.Sites.Example.com:443", f.site.Id},
		{"shop.sites.example.com.", f.site.Id},
		{"main.example.com", 0},
		{"later.sites.example.com", 0}, // pending: not open yet
		{"nobody.sites.example.com", 0},
		{"shop.other.example.com", 0},
	} {
		got := siteOf(t, f.engine, tc.host)
		if tc.want == 0 {
			assert.Nil(t, got, tc.host)
			continue
		}
		require.NotNil(t, got, tc.host)
		assert.Equal(t, tc.want, got.Id, tc.host)
	}
	// 暂停的站照常识别。
	_, err := service.SetAdtySiteStatus(f.site.Id, 1, model.AdtySiteStatusPaused, "")
	require.NoError(t, err)
	require.NotNil(t, siteOf(t, f.engine, "shop.sites.example.com"))
	// 分站功能关了就不识别。
	adty_setting.GetResellerSetting().Enabled = false
	assert.Nil(t, siteOf(t, f.engine, "shop.sites.example.com"))
}

func TestSiteContextByActiveCustomDomainOnly(t *testing.T) {
	f := newSiteFixture(t)
	domains := map[string]int{}
	saved := service.AdtyLoadCustomDomains
	service.AdtyLoadCustomDomains = func() (map[string]int, error) { return domains, nil }
	t.Cleanup(func() {
		service.AdtyLoadCustomDomains = saved
		service.InvalidateAdtySiteCache()
	})
	service.InvalidateAdtySiteCache()
	assert.Nil(t, siteOf(t, f.engine, "api.customer-brand.com"), "a pending custom domain is not in the active list")

	domains["api.customer-brand.com"] = f.site.Id
	service.InvalidateAdtySiteCache()
	got := siteOf(t, f.engine, "API.customer-brand.com")
	require.NotNil(t, got)
	assert.Equal(t, f.site.Id, got.Id)
	assert.Equal(t, "https://shop.sites.example.com", service.AdtySiteBaseURL(got), "links stay on the platform subdomain")
}

func TestCustomDomainAndUnknownHostRejectVerificationAndReset(t *testing.T) {
	f := newSiteFixture(t)
	saved := service.AdtyLoadCustomDomains
	service.AdtyLoadCustomDomains = func() (map[string]int, error) {
		return map[string]int{"api.customer-brand.com": f.site.Id}, nil
	}
	t.Cleanup(func() {
		service.AdtyLoadCustomDomains = saved
		service.InvalidateAdtySiteCache()
	})
	service.InvalidateAdtySiteCache()
	checks := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/verification?email=a@example.com"},
		{http.MethodGet, "/api/reset_password?email=a@example.com"},
		{http.MethodPost, "/api/user/reset"},
	}
	for _, check := range checks {
		assert.Equal(t, http.StatusForbidden, f.do(t, check.method, "api.customer-brand.com", check.path, `{"email":"a@example.com","token":"x","password":"correct-horse-1"}`).Code, check.path)
		assert.Equal(t, http.StatusNotFound, f.do(t, check.method, "stranger.example.net", check.path, `{}`).Code, check.path)
		assert.Equal(t, http.StatusForbidden, f.do(t, check.method, "gone.sites.example.com", check.path, `{}`).Code, check.path)
		assert.NotEqual(t, http.StatusForbidden, f.do(t, check.method, "shop.sites.example.com", check.path, `{}`).Code, check.path)
		assert.NotEqual(t, http.StatusNotFound, f.do(t, check.method, "main.example.com", check.path, `{}`).Code, check.path)
	}
}

func TestCustomDomainRedirectsConsoleAndRejectsAccountAPI(t *testing.T) {
	f := newSiteFixture(t)
	saved := service.AdtyLoadCustomDomains
	service.AdtyLoadCustomDomains = func() (map[string]int, error) {
		return map[string]int{"api.customer-brand.com": f.site.Id}, nil
	}
	t.Cleanup(func() {
		service.AdtyLoadCustomDomains = saved
		service.InvalidateAdtySiteCache()
	})
	service.InvalidateAdtySiteCache()

	page := f.do(t, http.MethodGet, "api.customer-brand.com", "/sign-in", "")
	assert.Equal(t, http.StatusFound, page.Code)
	assert.Equal(t, "https://shop.sites.example.com/sign-in", page.Header().Get("Location"))
	wallet := f.do(t, http.MethodGet, "api.customer-brand.com", "/wallet", "")
	assert.Equal(t, http.StatusFound, wallet.Code)
	assert.Contains(t, wallet.Header().Get("Location"), "https://shop.sites.example.com/wallet")

	assert.Equal(t, http.StatusForbidden, f.do(t, http.MethodGet, "api.customer-brand.com", "/api/user/self", "").Code)
	assert.Equal(t, http.StatusForbidden, f.do(t, http.MethodPost, "api.customer-brand.com", "/api/adty/epay/pay", `{}`).Code)
	assert.Equal(t, http.StatusForbidden, f.do(t, http.MethodPut, "api.customer-brand.com", "/api/adty/marketing/self", `{}`).Code)
	assert.Equal(t, http.StatusOK, f.do(t, http.MethodGet, "api.customer-brand.com", "/", "").Code)
	assert.NotEqual(t, http.StatusForbidden, f.do(t, http.MethodGet, "shop.sites.example.com", "/api/user/self", "").Code)
}

func TestCustomDomainRejectsOAuth(t *testing.T) {
	f := newSiteFixture(t)
	saved := service.AdtyLoadCustomDomains
	service.AdtyLoadCustomDomains = func() (map[string]int, error) {
		return map[string]int{"api.customer-brand.com": f.site.Id}, nil
	}
	t.Cleanup(func() {
		service.AdtyLoadCustomDomains = saved
		service.InvalidateAdtySiteCache()
	})
	service.InvalidateAdtySiteCache()

	body := `{"provider":"github","intent":"login"}`
	var before int64
	require.NoError(t, f.db.Model(&model.AuthFlow{}).Count(&before).Error)
	custom := f.do(t, http.MethodPost, "api.customer-brand.com", "/api/oauth/state", body)
	assert.Equal(t, http.StatusForbidden, custom.Code, custom.Body.String())
	assert.Equal(t, http.StatusForbidden, f.do(t, http.MethodGet, "api.customer-brand.com", "/api/oauth/github", "").Code)
	var after int64
	require.NoError(t, f.db.Model(&model.AuthFlow{}).Count(&after).Error)
	assert.Equal(t, before, after, "a custom domain must not create an OAuth flow")
	assert.Equal(t, http.StatusNotFound, f.do(t, http.MethodPost, "stranger.example.net", "/api/oauth/state", body).Code)
	assert.Equal(t, http.StatusForbidden, f.do(t, http.MethodPost, "gone.sites.example.com", "/api/oauth/state", body).Code)

	platform := f.do(t, http.MethodPost, "shop.sites.example.com", "/api/oauth/state", body)
	assert.NotEqual(t, http.StatusForbidden, platform.Code, platform.Body.String())
	assert.NotEqual(t, http.StatusNotFound, platform.Code, platform.Body.String())
}

func TestUnknownHostModes(t *testing.T) {
	f := newSiteFixture(t)
	cfg := adty_setting.GetResellerSetting()
	assert.Equal(t, http.StatusNotFound, f.do(t, http.MethodGet, "stranger.example.net", "/pricing", "").Code,
		"an unrecognized host cannot be routed to the main site")

	cfg.UnknownHostMode = adty_setting.ResellerUnknownHostReject
	assert.Equal(t, http.StatusNotFound, f.do(t, http.MethodGet, "stranger.example.net", "/pricing", "").Code)
	assert.Equal(t, http.StatusNotFound, f.do(t, http.MethodGet, "stranger.example.net", "/api/status", "").Code,
		"unknown-host APIs cannot silently run in the main-site scope")
	assert.Equal(t, "index", f.do(t, http.MethodGet, "main.example.com", "/pricing", "").Body.String(), "the main host is never rejected")
	assert.Equal(t, "index", f.do(t, http.MethodGet, "shop.sites.example.com", "/pricing", "").Body.String())
}

func TestAPIKeyMustMatchRequestSite(t *testing.T) {
	f := newSiteFixture(t)
	mainUser := f.user(t, "main-api-key-user", 0)
	siteUser := f.user(t, "site-api-key-user", f.site.Id)
	mainKey := "mainSiteKey"
	siteKey := "siteSiteKey"
	for _, token := range []*model.Token{
		{UserId: mainUser.Id, Key: mainKey, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
		{UserId: siteUser.Id, Key: siteKey, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
	} {
		require.NoError(t, f.db.Create(token).Error)
	}
	request := func(host, key string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Host = host
		req.Header.Set("Authorization", "Bearer "+key)
		response := httptest.NewRecorder()
		f.engine.ServeHTTP(response, req)
		return response
	}

	assert.Equal(t, http.StatusOK, request("shop.sites.example.com", siteKey).Code)
	assert.Equal(t, http.StatusForbidden, request("main.example.com", siteKey).Code)
	assert.Equal(t, http.StatusOK, request("main.example.com", mainKey).Code)
	assert.Equal(t, http.StatusForbidden, request("shop.sites.example.com", mainKey).Code)
	assert.Equal(t, http.StatusNotFound, request("stranger.example.net", mainKey).Code)
}

func TestAPIKeyReadOnlyAndCrossSiteHostIsolation(t *testing.T) {
	f := newSiteFixture(t)
	otherOwner := f.user(t, "other-api-owner", 0)
	other, err := service.OpenAdtySite(otherOwner.Id, "other-api-site", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	customer := f.user(t, "read-only-key-user", f.site.Id)
	key := "readonlySiteKey"
	require.NoError(t, f.db.Create(&model.Token{UserId: customer.Id, Key: key, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	for _, path := range []string{"/api/usage/token/", "/api/log/token", "/v1/models"} {
		for _, tc := range []struct {
			host   string
			status int
		}{
			{service.AdtySiteDisplayHost(f.site), http.StatusOK},
			{service.AdtySiteDisplayHost(other), http.StatusForbidden},
			{"main.example.com", http.StatusForbidden},
			{"stranger.example.net", http.StatusNotFound},
		} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Host = tc.host
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("X-Forwarded-Host", service.AdtySiteDisplayHost(f.site))
			response := httptest.NewRecorder()
			f.engine.ServeHTTP(response, req)
			assert.Equal(t, tc.status, response.Code, path+" on "+tc.host+": "+response.Body.String())
		}
	}
}

func TestUnknownHostRejectedWhenResellerDisabled(t *testing.T) {
	f := newSiteFixture(t)
	adty_setting.GetResellerSetting().Enabled = false
	mainUser := f.user(t, "disabled-main-key", 0)
	require.NoError(t, f.db.Create(&model.Token{UserId: mainUser.Id, Key: "disabledResellerKey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	for _, path := range []string{"/pricing", "/api/status", "/v1/models"} {
		for _, host := range []string{"stranger.example.net", "shop.sites.example.com"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Host = host
			req.Header.Set("Authorization", "Bearer disabledResellerKey")
			response := httptest.NewRecorder()
			f.engine.ServeHTTP(response, req)
			assert.Equal(t, http.StatusNotFound, response.Code, path+" on "+host)
		}
	}
}

func TestClosedSubdomainRedirects(t *testing.T) {
	f := newSiteFixture(t)
	resp := f.do(t, http.MethodGet, "gone.sites.example.com", "/wallet?x=1", "")
	assert.Equal(t, http.StatusFound, resp.Code)
	assert.Equal(t, "https://shop.sites.example.com/wallet?x=1", resp.Header().Get("Location"), "customers moved to the shop site")
	assert.Equal(t, http.StatusForbidden, f.do(t, http.MethodGet, "gone.sites.example.com", "/api/status", "").Code, "closed sites cannot run API requests as the main site")

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.closed.Id).Update("close_transfer_to", 0).Error)
	service.InvalidateAdtySiteCache()
	resp = f.do(t, http.MethodGet, "gone.sites.example.com", "/", "")
	assert.Equal(t, "https://main.example.com/", resp.Header().Get("Location"), "customers moved back to the main site")
}

func TestRegisterOnSiteSetsSiteId(t *testing.T) {
	f := newSiteFixture(t)
	register := func(host, name string) *httptest.ResponseRecorder {
		return f.do(t, http.MethodPost, host, "/api/user/register", fmt.Sprintf(`{"username":%q,"password":"correct-horse-1"}`, name))
	}
	require.Contains(t, register("shop.sites.example.com", "on-site").Body.String(), `"success":true`)
	require.Contains(t, register("main.example.com", "on-main").Body.String(), `"success":true`)
	var onSite, onMain model.User
	require.NoError(t, f.db.Where("username = ?", "on-site").First(&onSite).Error)
	require.NoError(t, f.db.Where("username = ?", "on-main").First(&onMain).Error)
	assert.Equal(t, f.site.Id, onSite.SiteId)
	assert.Zero(t, onMain.SiteId)

	var customer model.AdtySiteCustomer
	require.NoError(t, f.db.First(&customer, onSite.Id).Error)
	assert.Equal(t, f.site.Id, customer.SiteId)
	var affiliates int64
	require.NoError(t, f.db.Model(&model.ReferralAffiliate{}).Where("user_id = ?", onSite.Id).Count(&affiliates).Error)
	assert.Zero(t, affiliates, "site customers do not join the P6 program")
	require.NoError(t, f.db.Model(&model.ReferralAffiliate{}).Where("user_id = ?", onMain.Id).Count(&affiliates).Error)
	assert.EqualValues(t, 1, affiliates)

	_, err := service.ScheduleAdtySiteClose(f.site.Id, 1, service.AdtySiteClosePlan{})
	require.NoError(t, err)
	service.InvalidateAdtySiteCache()
	assert.Contains(t, register("shop.sites.example.com", "too-late").Body.String(), `"success":false`, "a closing site takes no new customers")
}

func TestMainLegalConsentIsRequiredAndRecorded(t *testing.T) {
	f := newSiteFixture(t)
	legal := system_setting.GetLegalSettings()
	oldAgreement, oldPrivacy := legal.UserAgreement, legal.PrivacyPolicy
	t.Cleanup(func() {
		legal.UserAgreement, legal.PrivacyPolicy = oldAgreement, oldPrivacy
	})
	legal.UserAgreement = "current terms"
	legal.PrivacyPolicy = "current privacy"

	missing := f.do(t, http.MethodPost, "main.example.com", "/api/user/register", `{"username":"no-legal","password":"correct-horse-1"}`)
	assert.Contains(t, missing.Body.String(), "Accept the current terms first")
	var missingUser int64
	require.NoError(t, f.db.Model(&model.User{}).Where("username = ?", "no-legal").Count(&missingUser).Error)
	assert.Zero(t, missingUser)

	accepted := f.do(t, http.MethodPost, "main.example.com", "/api/user/register", `{"username":"yes-legal","password":"correct-horse-1","agreement_accepted":true}`)
	require.Contains(t, accepted.Body.String(), `"success":true`, accepted.Body.String())
	var user model.User
	require.NoError(t, f.db.Where("username = ?", "yes-legal").First(&user).Error)
	var logs []model.Log
	require.NoError(t, f.db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeSystem).Find(&logs).Error)
	found := false
	for _, row := range logs {
		if strings.Contains(row.Content, "Accepted the current user agreement and privacy policy") {
			found = true
		}
	}
	assert.True(t, found, "accepting the published terms must leave a system log")
}

func TestSiteTermsRequiredAtRegister(t *testing.T) {
	f := newSiteFixture(t)
	legal := adty_setting.GetLegalSetting()
	old := legal.SiteCustomerTermsVersion
	legal.SiteCustomerTermsVersion = old + 4
	t.Cleanup(func() { legal.SiteCustomerTermsVersion = old })
	current := service.AdtyCurrentSiteCustomerTermsVersion()

	missing := f.do(t, http.MethodPost, "shop.sites.example.com", "/api/user/register", `{"username":"no-terms","password":"correct-horse-1"}`)
	assert.Contains(t, missing.Body.String(), `"success":false`)
	assert.Contains(t, missing.Body.String(), "Accept the current site customer terms first")
	var missingUser int64
	require.NoError(t, f.db.Model(&model.User{}).Where("username = ?", "no-terms").Count(&missingUser).Error)
	assert.Zero(t, missingUser)

	wrong := f.do(t, http.MethodPost, "shop.sites.example.com", "/api/user/register?site_terms_version=0", `{"username":"old-terms","password":"correct-horse-1"}`)
	assert.Contains(t, wrong.Body.String(), `"success":false`)

	main := f.do(t, http.MethodPost, "main.example.com", "/api/user/register", `{"username":"main-no-terms","password":"correct-horse-1"}`)
	assert.Contains(t, main.Body.String(), `"success":true`)

	accepted := f.do(t, http.MethodPost, "shop.sites.example.com", "/api/user/register?site_terms_version="+current, `{"username":"with-terms","password":"correct-horse-1"}`)
	require.Contains(t, accepted.Body.String(), `"success":true`, accepted.Body.String())
	var user model.User
	require.NoError(t, f.db.Where("username = ?", "with-terms").First(&user).Error)
	var row model.AdtySiteCustomer
	require.NoError(t, f.db.Where("user_id = ?", user.Id).First(&row).Error)
	assert.Equal(t, current, row.TermsVersion)
	assert.NotZero(t, row.TermsAt)
	assert.Equal(t, f.site.Id, row.SiteId)
}

func TestSiteCustomerLoginOnlyOnOwnSite(t *testing.T) {
	f := newSiteFixture(t)
	customer := f.user(t, "shop-customer", f.site.Id)
	mainUser := f.user(t, "main-customer", 0)
	login := func(host, username, password string) (bool, string) {
		t.Helper()
		resp := f.do(t, http.MethodPost, host, "/api/user/login", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password))
		var envelope apiEnvelope
		require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope), resp.Body.String())
		return envelope.Success, envelope.Message
	}
	ok, _ := login("shop.sites.example.com", customer.Username, "correct-horse-1")
	assert.True(t, ok)
	ok, _ = login("main.example.com", mainUser.Username, "correct-horse-1")
	assert.True(t, ok)

	_, wrongPassword := login("main.example.com", mainUser.Username, "wrong-password")
	_, unknownUser := login("main.example.com", "nobody-at-all", "correct-horse-1")
	ok, wrongSite := login("main.example.com", customer.Username, "correct-horse-1")
	assert.False(t, ok)
	assert.Equal(t, siteLoginFailure, wrongPassword)
	assert.Equal(t, wrongPassword, wrongSite, "same message as a wrong password: no account enumeration")
	assert.Equal(t, wrongPassword, unknownUser)
	ok, onOtherSite := login("shop.sites.example.com", mainUser.Username, "correct-horse-1")
	assert.False(t, ok, "main-site users cannot sign in on a reseller site")
	assert.Equal(t, siteLoginFailure, onOtherSite)

	// 二次验证：挑战不能在别的站上发出来，也不能拿到别的站去完成。
	factor := &model.TwoFA{UserId: customer.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}
	require.NoError(t, f.db.Create(factor).Error)
	ok, message := login("main.example.com", customer.Username, "correct-horse-1")
	assert.False(t, ok)
	assert.Equal(t, siteLoginFailure, message, "no 2FA challenge is issued on the wrong site")
	resp := f.do(t, http.MethodPost, "shop.sites.example.com", "/api/user/login", fmt.Sprintf(`{"username":%q,"password":"correct-horse-1"}`, customer.Username))
	var challenge struct {
		Success bool `json:"success"`
		Data    struct {
			FlowToken string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &challenge))
	require.True(t, challenge.Success, resp.Body.String())
	require.NotEmpty(t, challenge.Data.FlowToken)
	code, err := totp.GenerateCode(factor.Secret, time.Now())
	require.NoError(t, err)
	verify := f.do(t, http.MethodPost, "main.example.com", "/api/user/login/verify", fmt.Sprintf(`{"flow_token":%q,"code":%q}`, challenge.Data.FlowToken, code))
	assert.Contains(t, verify.Body.String(), siteLoginFailure)
	assert.NotContains(t, verify.Body.String(), "access_token")
	var active int64
	require.NoError(t, f.db.Model(&model.UserSession{}).Where("user_id = ? AND status = ?", customer.Id, model.UserSessionStatusActive).Count(&active).Error)
	assert.EqualValues(t, 1, active, "only the first password login on the shop site holds a session")

	adty_setting.GetResellerSetting().SiteLoginOnly = false
	ok, _ = login("main.example.com", mainUser.Username, "correct-horse-1")
	assert.True(t, ok)
	require.NoError(t, f.db.Where("user_id = ?", customer.Id).Delete(&model.TwoFA{}).Error)
	ok, _ = login("main.example.com", customer.Username, "correct-horse-1")
	assert.True(t, ok, "the switch turns the restriction off")
}

func TestAPIKeySiteBindingIgnoresForwardedHostAndAllowsOwnCustomDomain(t *testing.T) {
	f := newSiteFixture(t)
	customer := f.user(t, "token-customer", f.site.Id)
	require.NoError(t, f.db.Create(&model.Token{UserId: customer.Id, Key: "sitetokenkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	saved := service.AdtyLoadCustomDomains
	service.AdtyLoadCustomDomains = func() (map[string]int, error) {
		return map[string]int{"api.customer-brand.com": f.site.Id}, nil
	}
	t.Cleanup(func() { service.AdtyLoadCustomDomains = saved; service.InvalidateAdtySiteCache() })
	service.InvalidateAdtySiteCache()
	for _, tc := range []struct {
		host   string
		status int
	}{
		{"main.example.com", http.StatusForbidden},
		{"shop.sites.example.com", http.StatusOK},
		{"api.customer-brand.com", http.StatusOK},
		{"stranger.example.net", http.StatusNotFound},
		{"gone.sites.example.com", http.StatusForbidden},
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Host = tc.host
		req.Header.Set("X-Forwarded-Host", "shop.sites.example.com")
		req.Header.Set("Authorization", "Bearer sk-sitetokenkey")
		recorder := httptest.NewRecorder()
		f.engine.ServeHTTP(recorder, req)
		assert.Equal(t, tc.status, recorder.Code, tc.host+": "+recorder.Body.String())
	}
}

func TestStatusReturnsSiteBrand(t *testing.T) {
	f := newSiteFixture(t)
	oldName, oldFooter, oldQR, oldTurnstile := common.SystemName, common.Footer, common.WeChatAccountQRCodeImageURL, common.TurnstileSiteKey
	common.SystemName = "ADTY AI"
	common.Footer = "<p>main-footer</p>"
	common.WeChatAccountQRCodeImageURL = "https://cdn.example/qr.png"
	common.TurnstileSiteKey = "global-turnstile"
	cfg := adty_setting.GetResellerSetting()
	oldWidgets, oldSecrets := cfg.TurnstileWidgets, cfg.TurnstileWidgetSecret
	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "widget-1", SiteKey: "site-widget-key"}}
	cfg.TurnstileWidgetSecret = map[string]string{"widget-1": "site-widget-secret"}
	t.Cleanup(func() {
		common.SystemName, common.Footer, common.WeChatAccountQRCodeImageURL, common.TurnstileSiteKey = oldName, oldFooter, oldQR, oldTurnstile
		cfg.TurnstileWidgets, cfg.TurnstileWidgetSecret = oldWidgets, oldSecrets
	})
	status := func(host string) map[string]any {
		t.Helper()
		var envelope apiEnvelope
		require.NoError(t, common.Unmarshal(f.do(t, http.MethodGet, host, "/api/status", "").Body.Bytes(), &envelope))
		return envelope.Data
	}
	main := status("main.example.com")
	assert.Equal(t, "ADTY AI", main["system_name"])
	assert.Equal(t, "https://main.example.com", main["server_address"])
	assert.Nil(t, main["adty_site"])
	assert.Equal(t, "<p>main-footer</p>", main["footer_html"])
	assert.Equal(t, "global-turnstile", main["turnstile_site_key"])
	assert.Equal(t, "main.example.com", main["passkey_rp_id"])

	site := status("shop.sites.example.com")
	assert.Equal(t, "Shop AI", site["system_name"])
	assert.Equal(t, "/uploads/site-assets/shop.png", site["logo"])
	assert.Equal(t, "https://shop.sites.example.com", site["server_address"])
	assert.Equal(t, false, site["github_oauth"])
	assert.Equal(t, false, site["oidc_enabled"])
	assert.Equal(t, false, site["passkey_login"])
	assert.Nil(t, site["custom_oauth_providers"])
	assert.Equal(t, "", site["footer_html"])
	assert.Equal(t, "", site["wechat_qrcode"])
	assert.Equal(t, "", site["docs_link"])
	assert.Equal(t, "", site["passkey_rp_id"])
	assert.Empty(t, site["chats"])
	assert.Empty(t, site["api_info"])
	assert.Empty(t, site["announcements"])
	assert.Empty(t, site["faq"])
	assert.Empty(t, site["passkey_rp_ids"])
	assert.Equal(t, "site-widget-key", site["turnstile_site_key"])
}

func TestStatusDoesNotQuerySitesWhileHoldingOptionLock(t *testing.T) {
	f := newSiteFixture(t)
	cfg := adty_setting.GetResellerSetting()
	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "widget-1", SiteKey: "site-widget-key"}}
	service.InvalidateAdtySiteCache()
	var underLock, outside int
	require.NoError(t, f.db.Callback().Query().After("gorm:query").Register("status_site_query_lock", func(tx *gorm.DB) {
		table := tx.Statement.Table
		if table != "adty_sites" && table != "adty_site_domains" {
			return
		}
		if common.OptionMapRWMutex.TryLock() {
			common.OptionMapRWMutex.Unlock()
			outside++
			return
		}
		underLock++
	}))
	t.Cleanup(func() {
		_ = f.db.Callback().Query().Remove("status_site_query_lock")
	})
	response := f.do(t, http.MethodGet, "shop.sites.example.com", "/api/status", "")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Greater(t, outside, 0, "a cold site cache still loads the site table before the status handler")
	assert.Zero(t, underLock, "status must not query site tables while holding the option map lock")
}

func TestSiteNoticeAndHomePageContentAreEmpty(t *testing.T) {
	f := newSiteFixture(t)
	common.OptionMapRWMutex.Lock()
	oldNotice, oldHome := common.OptionMap["Notice"], common.OptionMap["HomePageContent"]
	common.OptionMap["Notice"] = "main-notice"
	common.OptionMap["HomePageContent"] = "<p>main-home</p>"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap["Notice"], common.OptionMap["HomePageContent"] = oldNotice, oldHome
		common.OptionMapRWMutex.Unlock()
	})
	body := func(host, path string) string {
		t.Helper()
		var envelope struct {
			Success bool   `json:"success"`
			Data    string `json:"data"`
		}
		response := f.do(t, http.MethodGet, host, path, "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
		require.True(t, envelope.Success, response.Body.String())
		return envelope.Data
	}
	assert.Equal(t, "main-notice", body("main.example.com", "/api/notice"))
	assert.Equal(t, "<p>main-home</p>", body("main.example.com", "/api/home_page_content"))
	assert.Empty(t, body("shop.sites.example.com", "/api/notice"))
	assert.Empty(t, body("shop.sites.example.com", "/api/home_page_content"))
}
