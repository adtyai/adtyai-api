package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelaySubmitSurfacesRequireCurrentSiteTerms(t *testing.T) {
	require.NoError(t, i18n.Init())
	f := newOwnerFixture(t)
	legal := adty_setting.GetLegalSetting()
	previous := legal.SiteCustomerTermsVersion
	legal.SiteCustomerTermsVersion = 7
	t.Cleanup(func() { legal.SiteCustomerTermsVersion = previous })
	customer := f.customer(t, f.site, "relay-terms-customer", "terms@example.test")
	token := model.Token{UserId: customer.Id, Key: "relaytermscustomerkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, f.db.Create(&token).Error)
	engine := gin.New()
	SetRelayRouter(engine)
	SetTaskRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetVideoRouter(engine)
	plugin := compileRouterPlugin(t, "terms-submit", "1.0.0", `[
		{method: "POST", path: "/vendor/terms/submit", type: "submit"},
		{method: "POST", path: "/vendor/terms/dynamic", type: "dynamic"}
	]`)
	dynamic, registry := newPluginRouterTest(t, []*jsplugin.LoadedPlugin{plugin}, productionPluginRouteHandlers)
	dynamic.NoRoute((&pluginRouteDispatcher{registry: registry}).dispatch)

	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/v1/responses"},
		{http.MethodGet, "/v1/responses"},
		{http.MethodPost, "/v1/images/generations"},
		{http.MethodPost, "/v1/images/edits"},
		{http.MethodPost, "/v1/videos"},
		{http.MethodPost, "/v1/video/generations"},
		{http.MethodPost, "/v1/videos/previous/remix"},
		{http.MethodPost, "/v1/tasks/test-plugin"},
		{http.MethodPost, "/vendor/terms/submit"},
		{http.MethodPost, "/vendor/terms/dynamic"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"model":"terms-test","input":"test"}`))
			request.Header.Set("Authorization", "Bearer sk-"+token.Key)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			if strings.HasPrefix(route.path, "/vendor/") {
				dynamic.ServeHTTP(response, request)
			} else {
				engine.ServeHTTP(response, request)
			}
			require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "Accept the current site customer terms first")
		})
	}
	var stored model.User
	require.NoError(t, f.db.First(&stored, customer.Id).Error)
	require.Equal(t, customer.Quota, stored.Quota)
	require.Zero(t, stored.QuotaPending)
}
