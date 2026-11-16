package router

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Subscriptions, redemption codes and upstream invite rewards were deleted in
// tasks 1.2-1.5 (docs/adty/deleted-features.md). An upstream merge must not
// bring their routes back, in the api router or in any other router file.
func TestAdtyDeletedRoutesAbsent(t *testing.T) {
	routes := newAccessTokenScopeTestEngine(t).Routes()
	require.NotEmpty(t, routes)

	// Third-party sign-in (GitHub, Discord, LinuxDO, OIDC, custom OAuth, WeChat
	// and Telegram) and its provider framework are gone; only the email binding
	// routes remain under /api/oauth.
	// Stripe, Creem, Waffo and Waffo Pancake, check-in, the payment compliance
	// confirmation and the upstream single-channel Epay entries were deleted in
	// the 2026-11 slimming (docs/adty/deleted-features.md).
	deletedPrefixes := []string{"/api/subscription", "/api/redemption", "/api/custom-oauth-provider",
		"/api/stripe", "/api/creem", "/api/waffo", "/api/user/stripe", "/api/user/creem", "/api/user/waffo",
		"/api/option/waffo-pancake", "/api/user/checkin"}
	deletedRoutes := []struct{ method, path string }{
		{http.MethodPost, "/api/user/topup"},
		{http.MethodGet, "/api/user/aff"},
		{http.MethodPost, "/api/user/aff_transfer"},
		{http.MethodPost, "/api/option/waffo-pancake/subscription-product"},
		{http.MethodGet, "/api/option/waffo-pancake/subscription-product-options"},
		{http.MethodGet, "/api/oauth/wechat"},
		{http.MethodPost, "/api/oauth/wechat/bind"},
		{http.MethodGet, "/api/user/oauth/bindings"},
		{http.MethodDelete, "/api/user/oauth/bindings/:provider_id"},
		{http.MethodGet, "/api/user/:id/oauth/bindings"},
		{http.MethodDelete, "/api/user/:id/oauth/bindings/:provider_id"},
		{http.MethodPost, "/api/oauth/state"},
		{http.MethodGet, "/api/oauth/:provider"},
		{http.MethodGet, "/api/oauth/telegram/login"},
		{http.MethodPost, "/api/oauth/telegram/bind/start"},
		{http.MethodGet, "/api/oauth/telegram/bind/:flow_token"},
		{http.MethodPost, "/api/user/topup/complete"},
		{http.MethodGet, "/api/user/topup/info"},
		{http.MethodGet, "/api/user/topup/self"},
		{http.MethodPost, "/api/user/amount"},
		{http.MethodGet, "/api/user/epay/notify"},
		{http.MethodPost, "/api/user/epay/notify"},
		{http.MethodPost, "/api/option/payment_compliance"},
		{http.MethodPost, "/api/adty/topup/hosted/pay"},
		{http.MethodGet, "/api/adty/admin/reseller/invite-codes"},
		{http.MethodPost, "/api/adty/admin/reseller/invite-codes"},
		{http.MethodPut, "/api/adty/admin/reseller/invite-codes/:id"},
		{http.MethodDelete, "/api/adty/admin/reseller/invite-codes/:id"},
	}
	for _, route := range routes {
		for _, prefix := range deletedPrefixes {
			assert.Falsef(t, strings.HasPrefix(route.Path, prefix),
				"deleted route is registered again: %s %s", route.Method, route.Path)
		}
		for _, deleted := range deletedRoutes {
			assert.Falsef(t, route.Method == deleted.method && route.Path == deleted.path,
				"deleted route is registered again: %s %s", route.Method, route.Path)
		}
	}

	// GET /api/user/topup is the admin top-up history. It shares the path of
	// the deleted redeem endpoint and stays.
	assert.True(t, slices.ContainsFunc(routes, func(route gin.RouteInfo) bool {
		return route.Method == http.MethodGet && route.Path == "/api/user/topup"
	}))

	// The email binding routes keep their historical /api/oauth path.
	for _, kept := range []struct{ method, path string }{
		{http.MethodPost, "/api/oauth/email/bind/start"},
		{http.MethodPost, "/api/oauth/email/bind/resend"},
		{http.MethodPost, "/api/oauth/email/bind"},
		{http.MethodPost, "/api/user/login"},
		{http.MethodPost, "/api/user/register"},
	} {
		assert.Truef(t, slices.ContainsFunc(routes, func(route gin.RouteInfo) bool {
			return route.Method == kept.method && route.Path == kept.path
		}), "retained route is missing: %s %s", kept.method, kept.path)
	}
}
