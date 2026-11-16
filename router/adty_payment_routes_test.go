package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func routeHandler(t *testing.T, engine *gin.Engine, method, path string) string {
	t.Helper()
	for _, route := range engine.Routes() {
		if route.Method == method && route.Path == path {
			return route.Handler
		}
	}
	t.Fatalf("route not registered: %s %s", method, path)
	return ""
}

func TestRestoredPaymentRoutesRequireAuthentication(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/api/user/pay", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "not available")
}

func TestOfficialEpayNotifyIsNotClosed(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	require.Contains(t, routeHandler(t, engine, http.MethodGet, "/api/user/epay/notify/:channelId"), "AdtyEpayNotify")
	require.Contains(t, routeHandler(t, engine, http.MethodPost, "/api/user/epay/notify/:channelId"), "AdtyEpayNotify")
	require.Contains(t, routeHandler(t, engine, http.MethodPost, "/api/user/pay"), "RequestAdtyEpayPay")
}

func TestAdminCustomDomainDisableRouteIsMounted(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/adty/admin/sites/domains/4", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}
