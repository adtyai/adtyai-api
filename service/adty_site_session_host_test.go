package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionNotIssuedOnCustomOrClosedHost(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "sess")
	customer := siteCustomer(t, db, site, "sess-customer")
	require.NoError(t, db.Create(&model.AdtySiteDomain{
		SiteId: site.Id, Hostname: "api.sess.example", Status: model.AdtySiteDomainActive,
	}).Error)
	InvalidateAdtySiteCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/user/login/2fa", nil)
	ctx.Request.Host = "api.sess.example"
	assert.False(t, AdtySiteLoginAllowed(ctx, customer.Id), "a custom domain must not finish login or 2FA")

	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusClosed).Error)
	InvalidateAdtySiteCache()
	main := referralUser(t, db, "sess-main")
	ctx.Request.Host = site.Subdomain + "." + site.RootDomain
	assert.False(t, AdtySiteLoginAllowed(ctx, main.Id), "a closed site host must not issue a session")

	ctx.Request.Host = "localhost"
	assert.True(t, AdtySiteLoginAllowed(ctx, main.Id))
}
