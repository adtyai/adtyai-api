package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOAuthSiteFixture(t *testing.T) (*oidcOAuthFixture, *model.AdtySite) {
	t.Helper()
	fixture := setupOIDCOauthTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AdtySiteDomain{}, &model.AdtySiteCustomer{}, &model.AdtyUserFingerprint{}))
	settings := adty_setting.GetResellerSetting()
	previous, previousLegal := *settings, *adty_setting.GetLegalSetting()
	previousRegister := common.RegisterEnabled
	*settings = adty_setting.DefaultResellerSetting()
	settings.Enabled, settings.RootDomains = true, []string{"sites.example"}
	common.RegisterEnabled = true
	t.Cleanup(func() {
		*settings, *adty_setting.GetLegalSetting() = previous, previousLegal
		common.RegisterEnabled = previousRegister
		service.InvalidateAdtySiteCache()
	})
	site := &model.AdtySite{OwnerUserId: fixture.user.Id, Subdomain: "shop", RootDomain: "sites.example", Status: model.AdtySiteStatusActive}
	require.NoError(t, model.DB.Create(site).Error)
	require.NoError(t, model.DB.Model(fixture.user).Update("site_id", site.Id).Error)
	service.InvalidateAdtySiteCache()
	return fixture, site
}

func startOAuthSiteFlow(t *testing.T, fixture *oidcOAuthFixture, site *model.AdtySite, terms string) (string, string) {
	t.Helper()
	body, err := common.Marshal(map[string]string{"provider": "oidc", "intent": "login", "site_terms_version": terms})
	require.NoError(t, err)
	response := securityEnrollmentRequest(http.MethodPost, "/api/oauth/state", string(body), "", service.AuthIdentity{}, func(c *gin.Context) {
		c.Request.Host = "shop.sites.example"
		c.Set(service.AdtySiteContextKey, site)
		withOAuthTestBrowser(GenerateOAuthCode)(c)
	})
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			State       string `json:"flow_token"`
			URL         string `json:"authorization_url"`
			RedirectURI string `json:"redirect_uri"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, response.Body.String())
	assert.Equal(t, "https://app.example/oauth/oidc", envelope.Data.RedirectURI)
	parsed, err := url.Parse(envelope.Data.URL)
	require.NoError(t, err)
	code := "code-" + envelope.Data.State
	fixture.mutex.Lock()
	fixture.grants[code] = oidcTestGrant{challenge: parsed.Query().Get("code_challenge"), nonce: parsed.Query().Get("nonce")}
	fixture.mutex.Unlock()
	return envelope.Data.State, code
}

func oauthSiteCallback(site *model.AdtySite, host, state, code, terms string, browser bool) *httptest.ResponseRecorder {
	path := "/api/oauth/oidc?" + url.Values{"state": {state}, "code": {code}, "site_terms_version": {terms}}.Encode()
	return securityEnrollmentRequest(http.MethodGet, path, "", "", service.AuthIdentity{}, func(c *gin.Context) {
		c.Request.Host = host
		c.Params = gin.Params{{Key: "provider", Value: "oidc"}}
		if site != nil {
			c.Set(service.AdtySiteContextKey, site)
		}
		if browser {
			withOAuthTestBrowser(HandleOAuth)(c)
		} else {
			HandleOAuth(c)
		}
	})
}

func TestOAuthSiteCallbackHandoffPreservesOriginalBrowserAndFlow(t *testing.T) {
	fixture, site := setupOAuthSiteFixture(t)
	state, code := startOAuthSiteFlow(t, fixture, site, "")
	response := oauthSiteCallback(nil, "app.example", state, code, "", false)
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			URL string `json:"oauth_handoff_url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, response.Body.String())
	parsed, err := url.Parse(envelope.Data.URL)
	require.NoError(t, err)
	assert.Equal(t, "https://shop.sites.example/oauth/oidc", parsed.Scheme+"://"+parsed.Host+parsed.Path)
	assert.Equal(t, state, parsed.Query().Get("state"))
	assert.Equal(t, code, parsed.Query().Get("code"))
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Equal(t, "no-referrer", response.Header().Get("Referrer-Policy"))
	_, err = model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: "oidc"})
	require.NoError(t, err)
	fixture.mutex.Lock()
	_, unexchanged := fixture.grants[code]
	fixture.mutex.Unlock()
	assert.True(t, unexchanged, "canonical callback must not exchange the authorization code")
	assert.Equal(t, http.StatusForbidden, oauthSiteCallback(site, "evil.example", state, code, "", true).Code)
	assert.Equal(t, http.StatusForbidden, oauthSiteCallback(site, "shop.sites.example", state, code, "", false).Code)
	original := oauthSiteCallback(site, "shop.sites.example", state, code, "", true)
	assert.Contains(t, original.Body.String(), `"success":true`, original.Body.String())
	assert.Equal(t, http.StatusForbidden, oauthSiteCallback(site, "shop.sites.example", state, code, "", true).Code)
}

func TestOAuthSiteTermsAreBoundAtStartAndRechecked(t *testing.T) {
	for _, scenario := range []struct {
		name, accepted, injected string
		current                  int
		success                  bool
	}{
		{"accepted version is recorded", "1", "", 1, true},
		{"callback cannot add consent", "", "1", 1, false},
		{"changed terms require new consent", "1", "2", 2, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture, site := setupOAuthSiteFixture(t)
			adty_setting.GetLegalSetting().SiteCustomerTermsVersion = 1
			fixture.subject = "new-site-subject"
			state, code := startOAuthSiteFlow(t, fixture, site, scenario.accepted)
			adty_setting.GetLegalSetting().SiteCustomerTermsVersion = scenario.current
			response := oauthSiteCallback(site, "shop.sites.example", state, code, scenario.injected, true)
			var envelope securityEnrollmentResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
			assert.Equal(t, scenario.success, envelope.Success, response.Body.String())
			if scenario.success {
				var user model.User
				require.NoError(t, model.DB.Where("oidc_id = ?", fixture.subject).First(&user).Error)
				assert.Equal(t, site.Id, user.SiteId)
				var customer model.AdtySiteCustomer
				require.NoError(t, model.DB.First(&customer, "user_id = ?", user.Id).Error)
				assert.Equal(t, "1", customer.TermsVersion)
			} else {
				assert.Contains(t, response.Body.String(), "Accept the current site customer terms first")
			}
		})
	}
}

func TestOAuthSiteStartRejectsUntrustedHostAndStaleConsent(t *testing.T) {
	_, site := setupOAuthSiteFixture(t)
	adty_setting.GetLegalSetting().SiteCustomerTermsVersion = 2
	for _, scenario := range []struct{ host, terms string }{{"evil.example", "2"}, {"shop.sites.example", "1"}} {
		body, err := common.Marshal(map[string]string{"provider": "oidc", "intent": "login", "site_terms_version": scenario.terms})
		require.NoError(t, err)
		response := securityEnrollmentRequest(http.MethodPost, "/api/oauth/state", string(body), "", service.AuthIdentity{}, func(c *gin.Context) {
			c.Request.Host = scenario.host
			c.Set(service.AdtySiteContextKey, site)
			withOAuthTestBrowser(GenerateOAuthCode)(c)
		})
		assert.NotContains(t, response.Body.String(), `"success":true`, response.Body.String())
	}
}

func TestOAuthSiteHandoffKeepsBindAndVerificationSessionBound(t *testing.T) {
	fixture, site := setupOAuthSiteFixture(t)
	provider := &authFlowTestOAuthProvider{}
	oauth.Register("auth-flow-test", provider)
	t.Cleanup(func() { oauth.Unregister("auth-flow-test") })
	for _, intent := range []string{model.AuthFlowIntentBind, model.AuthFlowIntentVerify} {
		payload, err := common.Marshal(map[string]any{"site_id": site.Id, "site_origin": "https://shop.sites.example"})
		require.NoError(t, err)
		state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
			Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test", Intent: intent,
			UserId: fixture.user.Id, SessionId: "original-session", Payload: string(payload),
			ExpiresAt: time.Now().Add(time.Minute),
		})
		require.NoError(t, err)
		callback := func(host string, identity service.AuthIdentity) *httptest.ResponseRecorder {
			return securityEnrollmentRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+url.QueryEscape(state)+"&code=provider-code", "", "", identity, func(c *gin.Context) {
				c.Request.Host = host
				c.Params = gin.Params{{Key: "provider", Value: "auth-flow-test"}}
				HandleOAuth(c)
			})
		}
		response := callback("app.example", service.AuthIdentity{})
		assert.Contains(t, response.Body.String(), `"oauth_handoff_url":`, response.Body.String())
		assert.Equal(t, http.StatusForbidden, callback("shop.sites.example", service.AuthIdentity{}).Code)
		assert.Equal(t, http.StatusForbidden, callback("shop.sites.example", service.AuthIdentity{UserID: fixture.user.Id, SessionID: "other-session"}).Code)
		_, err = model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test"})
		require.NoError(t, err)
	}
	assert.Zero(t, provider.exchangeCalls)
}
