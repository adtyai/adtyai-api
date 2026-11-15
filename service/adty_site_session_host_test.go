package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 自定义域名承接完整客户端体验：还在服务的站点上可以登录和二次验证，
// 站点关掉后自定义域名和平台子域名一样拒绝。
func TestAdtySiteSessionIssuedOnServingCustomHost(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "sess")
	customer := siteCustomer(t, db, site, "sess-customer")
	main := referralUser(t, db, "sess-main")
	require.NoError(t, db.Create(&model.AdtySiteDomain{
		SiteId: site.Id, Hostname: "api.sess.example", Status: model.AdtySiteDomainActive,
	}).Error)
	InvalidateAdtySiteCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/user/login/2fa", nil)
	ctx.Request.Host = "api.sess.example"
	ctx.Set(AdtySiteContextKey, site)
	assert.True(t, AdtySiteLoginAllowed(ctx, customer.Id), "a serving custom domain can finish login or 2FA")
	assert.False(t, AdtySiteLoginAllowed(ctx, main.Id), "a custom domain only signs in its own customers")

	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusClosed).Error)
	InvalidateAdtySiteCache()
	assert.False(t, AdtySiteLoginAllowed(ctx, customer.Id), "a closed site's custom domain must not issue a session")
	ctx.Request.Host = site.Subdomain + "." + site.RootDomain
	assert.False(t, AdtySiteLoginAllowed(ctx, main.Id), "a closed site host must not issue a session")

	// 非分站 Host（主站）不写站点上下文，和中间件的处理一样。
	mainCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	mainCtx.Request = httptest.NewRequest("POST", "/api/user/login/2fa", nil)
	mainCtx.Request.Host = "localhost"
	assert.True(t, AdtySiteLoginAllowed(mainCtx, main.Id))
}
