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

	deletedPrefixes := []string{"/api/subscription", "/api/redemption"}
	deletedRoutes := []struct{ method, path string }{
		{http.MethodPost, "/api/user/topup"},
		{http.MethodGet, "/api/user/aff"},
		{http.MethodPost, "/api/user/aff_transfer"},
		{http.MethodPost, "/api/option/waffo-pancake/subscription-product"},
		{http.MethodGet, "/api/option/waffo-pancake/subscription-product-options"},
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
}
