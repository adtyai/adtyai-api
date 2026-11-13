package router

import (
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdtyGapRoutesRegistered(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	found := map[string]bool{}
	for _, route := range engine.Routes() {
		found[route.Method+" "+route.Path] = true
	}
	for _, key := range []string{
		"POST /api/adty/notifications/hide",
		"POST /api/adty/notifications/todo/complete",
		"POST /api/adty/staff/balance-requests",
		"POST /api/adty/site/balance-requests/:id/review",
		"POST /api/adty/admin/balance-requests/:id/review",
		"POST /api/adty/site/wallets/earning-to-margin",
		"GET /api/adty/notifications/announcements/:key",
		"GET /api/adty/admin/fund-anomalies",
		"POST /api/adty/site/withdrawals",
		"POST /api/adty/site/withdrawals/:id/cancel",
		"PUT /api/adty/admin/site-withdrawals/rules",
		"POST /api/adty/admin/site-withdrawals/:id/paying",
		"POST /api/adty/admin/site-withdrawals/:id/reject",
		"POST /api/adty/admin/site-withdrawals/:id/paid",
	} {
		assert.Truef(t, found[key], "missing route %s", key)
	}
}

func TestAdtyActionRoutesRequireIndependentScopes(t *testing.T) {
	restore := middleware.EnableAdtyAdminPermissions()
	t.Cleanup(restore)
	newAccessTokenScopeTestEngine(t)
	for _, route := range []struct{ key, scope string }{
		{"POST /api/user/", "adty_user:create"},
		{"DELETE /api/user/:id", "adty_user:delete"},
		{"GET /api/log/export", "adty_log:export"},
		{"POST /api/models/", "adty_model:create"},
		{"POST /api/vendors/", "adty_model:create"},
		{"POST /api/prefill_group/", "adty_model:create"},
		{"DELETE /api/models/:id", "adty_model:delete"},
		{"POST /api/models/delete", "adty_model:delete"},
		{"DELETE /api/vendors/:id", "adty_model:delete"},
		{"DELETE /api/prefill_group/:id", "adty_model:delete"},
		{"POST /api/channel/", "channel:create"},
		{"POST /api/channel/copy/:id", "channel:create"},
		{"DELETE /api/channel/disabled", "channel:delete"},
		{"DELETE /api/channel/:id", "channel:delete"},
		{"POST /api/channel/batch", "channel:delete"},
		{"POST /api/adty/admin/sites", "option:write"},
		{"POST /api/adty/admin/sites/:id/review", "option:write"},
		{"DELETE /api/adty/admin/sites/:id/sender-domain", "adty_site:delete"},
		{"DELETE /api/adty/admin/sites/domains/:id", "adty_site:delete"},
		{"DELETE /api/adty/admin/sites/:id/notice", "adty_site:delete"},
		{"GET /api/adty/admin/reseller/settings", "adty_site:settings_read"},
		{"GET /api/adty/admin/reseller/tiers", "option:read"},
		{"PUT /api/adty/admin/reseller/tiers", "option:write"},
		{"GET /api/adty/admin/reseller/invite-codes", "adty_site:settings_read"},
		{"POST /api/adty/admin/reseller/invite-codes", "adty_site:settings_create"},
		{"DELETE /api/adty/admin/reseller/invite-codes/:id", "adty_site:settings_delete"},
		{"GET /api/user/admin/referral/settings", "adty_referral:settings_read"},
	} {
		t.Run(route.key, func(t *testing.T) {
			rule, ok := middleware.AccessTokenRouteRule(route.key)
			require.True(t, ok)
			assert.Equal(t, "scope", rule.Kind())
			assert.Equal(t, route.scope, rule.Scope())
		})
	}
}

func TestAdtyAdminReadWriteRoutesUseCurrentCatalog(t *testing.T) {
	restore := middleware.EnableAdtyAdminPermissions()
	t.Cleanup(restore)
	newAccessTokenScopeTestEngine(t)
	for _, route := range []struct{ key, scope string }{
		{"GET /api/user/", "adty_user:read"},
		{"GET /api/user/search", "adty_user:read"},
		{"GET /api/user/:id", "adty_user:read"},
		{"GET /api/user/:id/oauth/bindings", "adty_user:read"},
		{"GET /api/user/2fa/stats", "adty_user:read"},
		{"GET /api/authz/catalog", "adty_user:read"},
		{"PUT /api/user/", "adty_user:write"},
		{"DELETE /api/user/:id/oauth/bindings/:provider_id", "adty_user:write"},
		{"DELETE /api/user/:id/bindings/:binding_type", "adty_user:write"},
		{"DELETE /api/user/:id/reset_passkey", "adty_user:write"},
		{"DELETE /api/user/:id/2fa", "adty_user:write"},
		{"GET /api/models/", "adty_model:read"},
		{"GET /api/models/search", "adty_model:read"},
		{"GET /api/models/:id", "adty_model:read"},
		{"GET /api/models/missing", "adty_model:read"},
		{"GET /api/models/sync_upstream/preview", "adty_model:read"},
		{"GET /api/vendors/", "adty_model:read"},
		{"GET /api/vendors/search", "adty_model:read"},
		{"GET /api/vendors/:id", "adty_model:read"},
		{"POST /api/vendors/operations/preview", "adty_model:read"},
		{"PUT /api/models/", "adty_model:write"},
		{"PUT /api/vendors/", "adty_model:write"},
		{"GET /api/prefill_group/", "adty_model:read"},
		{"PUT /api/prefill_group/", "adty_model:write"},
		{"GET /api/status/test", "adty_log:read"},
		{"GET /api/log/", "adty_log:read"},
		{"GET /api/log/stat", "adty_log:read"},
		{"GET /api/log/channel_affinity_usage_cache", "adty_log:read"},
		{"GET /api/log/search", "adty_log:read"},
		{"GET /api/data/", "adty_log:read"},
		{"GET /api/data/users", "adty_log:read"},
		{"GET /api/data/flow", "adty_log:read"},
		{"GET /api/mj/", "adty_log:read"},
		{"GET /api/task", "adty_log:read"},
		{"GET /api/user/topup", "adty_finance:read"},
	} {
		t.Run(route.key, func(t *testing.T) {
			rule, ok := middleware.AccessTokenRouteRule(route.key)
			require.True(t, ok)
			assert.Equal(t, "scope", rule.Kind())
			assert.Equal(t, route.scope, rule.Scope())
		})
	}
	for _, key := range []string{"POST /api/models/sync_upstream", "POST /api/vendors/operations"} {
		rule, ok := middleware.AccessTokenRouteRule(key)
		require.True(t, ok)
		assert.Equal(t, "any", rule.Kind(), "compound actions are decided by the transaction/handler")
	}
}
