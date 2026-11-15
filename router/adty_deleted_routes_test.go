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

	// Third-party sign-in (GitHub, Discord, LinuxDO, OIDC, custom OAuth and
	// WeChat) is gone; only the password login, email binding and the retired
	// Telegram legacy routes remain under /api/oauth.
	deletedPrefixes := []string{"/api/subscription", "/api/redemption", "/api/custom-oauth-provider"}
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

	// The generic registry callback, the flow starter, the email binding routes
	// and the retired Telegram legacy routes stay registered.
	for _, kept := range []struct{ method, path string }{
		{http.MethodGet, "/api/oauth/:provider"},
		{http.MethodPost, "/api/oauth/state"},
		{http.MethodPost, "/api/oauth/email/bind/start"},
		{http.MethodPost, "/api/oauth/email/bind/resend"},
		{http.MethodPost, "/api/oauth/email/bind"},
		{http.MethodGet, "/api/oauth/telegram/login"},
		{http.MethodPost, "/api/oauth/telegram/bind/start"},
		{http.MethodGet, "/api/oauth/telegram/bind/:flow_token"},
		{http.MethodPost, "/api/user/login"},
		{http.MethodPost, "/api/user/register"},
	} {
		assert.Truef(t, slices.ContainsFunc(routes, func(route gin.RouteInfo) bool {
			return route.Method == kept.method && route.Path == kept.path
		}), "retained route is missing: %s %s", kept.method, kept.path)
	}
}
