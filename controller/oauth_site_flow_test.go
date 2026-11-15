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
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOAuthCallbackSiteFixture(t *testing.T) (*model.User, *model.AdtySite) {
	t.Helper()
	user, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AdtySiteDomain{}, &model.AdtySiteCustomer{}, &model.AdtyUserFingerprint{}))
	settings := adty_setting.GetResellerSetting()
	previous, previousLegal := *settings, *adty_setting.GetLegalSetting()
	previousRegister := common.RegisterEnabled
	previousAddress := system_setting.ServerAddress
	*settings = adty_setting.DefaultResellerSetting()
	settings.Enabled, settings.RootDomains = true, []string{"sites.example"}
	common.RegisterEnabled = true
	system_setting.ServerAddress = "https://app.example"
	t.Cleanup(func() {
		*settings, *adty_setting.GetLegalSetting() = previous, previousLegal
		common.RegisterEnabled = previousRegister
		system_setting.ServerAddress = previousAddress
		service.InvalidateAdtySiteCache()
	})
	site := &model.AdtySite{OwnerUserId: user.Id, Subdomain: "shop", RootDomain: "sites.example", Status: model.AdtySiteStatusActive}
	require.NoError(t, model.DB.Create(site).Error)
	require.NoError(t, model.DB.Model(user).Update("site_id", site.Id).Error)
	service.InvalidateAdtySiteCache()
	return user, site
}

// Bind and verification callbacks must return to the platform origin that owns
// the host-only browser cookie, and must stay bound to the session that started
// them. The provider itself is a test double; the flow plumbing under test is
// shared by every provider.
func TestOAuthSiteHandoffKeepsBindAndVerificationSessionBound(t *testing.T) {
	user, site := setupOAuthCallbackSiteFixture(t)
	provider := &authFlowTestOAuthProvider{}
	oauth.Register("auth-flow-test", provider)
	t.Cleanup(func() { oauth.Unregister("auth-flow-test") })
	for _, intent := range []string{model.AuthFlowIntentBind, model.AuthFlowIntentVerify} {
		payload, err := common.Marshal(map[string]any{"site_id": site.Id, "site_origin": "https://shop.sites.example"})
		require.NoError(t, err)
		state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
			Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test", Intent: intent,
			UserId: user.Id, SessionId: "original-session", Payload: string(payload),
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
		assert.Equal(t, http.StatusForbidden, callback("shop.sites.example", service.AuthIdentity{UserID: user.Id, SessionID: "other-session"}).Code)
		_, err = model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test"})
		require.NoError(t, err)
	}
	assert.Zero(t, provider.exchangeCalls)
}
