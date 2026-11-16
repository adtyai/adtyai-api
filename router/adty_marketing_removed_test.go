package router

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetiredMarketingEndpointsReturnNotFound(t *testing.T) {
	f := newAdtyGuardFixture(t)
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/adty/marketing/consent-default"},
		{http.MethodGet, "/api/adty/marketing/self"},
		{http.MethodGet, "/api/adty/marketing/unsubscribe"},
		{http.MethodPost, "/api/adty/marketing/resend-webhook"},
		{http.MethodGet, "/api/adty/admin/marketing/settings"},
		{http.MethodGet, "/api/adty/admin/marketing/campaigns"},
		{http.MethodGet, "/api/adty/admin/marketing/triggers"},
		{http.MethodGet, "/api/adty/admin/marketing/sources"},
		{http.MethodGet, "/api/adty/admin/marketing/promo-codes"},
	} {
		response := adtyJSON(t, f.engine, request.method, request.path, `{}`, f.root)
		assert.Equal(t, http.StatusNotFound, response.Code, "%s %s", request.method, request.path)
	}
}
