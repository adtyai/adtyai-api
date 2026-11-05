package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Excluded products must not stay callable: check-in grants quota, and the
// upstream single-channel pay/webhook handlers credit a wallet without the
// adty order, bonus, commission, and ledger path.
func TestClosedFeatureRoutesRejectBeforeCredit(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/user/pay", `{"amount":10,"payment_method":"alipay"}`},
		{http.MethodPost, "/api/user/stripe/pay", `{"amount":10}`},
		{http.MethodPost, "/api/user/stripe/amount", `{"amount":10}`},
		{http.MethodPost, "/api/user/creem/pay", `{"product_id":"prod"}`},
		{http.MethodPost, "/api/user/waffo/pay", `{"amount":10}`},
		{http.MethodPost, "/api/user/waffo/amount", `{"amount":10}`},
		{http.MethodPost, "/api/user/waffo-pancake/pay", `{"amount":10}`},
		{http.MethodPost, "/api/user/waffo-pancake/amount", `{"amount":10}`},
		{http.MethodPost, "/api/user/epay/notify", "out_trade_no=x&trade_status=TRADE_SUCCESS"},
		{http.MethodGet, "/api/user/epay/notify?out_trade_no=x&trade_status=TRADE_SUCCESS", ""},
		{http.MethodPost, "/api/stripe/webhook", `{"type":"checkout.session.completed"}`},
		{http.MethodPost, "/api/creem/webhook", `{"eventType":"checkout.completed"}`},
		{http.MethodPost, "/api/waffo/webhook", `{}`},
		{http.MethodPost, "/api/waffo-pancake/webhook/sandbox", `{}`},
		{http.MethodPost, "/api/user/checkin", `{}`},
		{http.MethodGet, "/api/user/checkin", ""},
		{http.MethodPost, "/api/user/topup/complete", `{"trade_no":"OLD-1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "not available")
		})
	}
	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	require.True(t, registered["POST /api/user/epay/notify/:channelId"])
	require.True(t, registered["GET /api/user/epay/notify/:channelId"])
	require.True(t, registered["DELETE /api/adty/admin/sites/domains/:id"])
}

func TestAdminCustomDomainDisableRouteIsMounted(t *testing.T) {
	engine := newAccessTokenScopeTestEngine(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/adty/admin/sites/domains/4", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}
