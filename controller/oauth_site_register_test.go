package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthRegistrationUsesRequestSite(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySiteCustomer{}, &model.AdtyUserFingerprint{}))
	previousRegister, previousRedis := common.RegisterEnabled, common.RedisEnabled
	common.RegisterEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RegisterEnabled = previousRegister
		common.RedisEnabled = previousRedis
	})
	provider := &authFlowTestOAuthProvider{}
	oauthUser := &oauth.OAuthUser{ProviderUserID: "oauth-site-subject", Username: "oauthsiteuser"}

	closing, _ := gin.CreateTestContext(httptest.NewRecorder())
	closing.Request = httptest.NewRequest("GET", "/api/oauth/github", nil)
	closing.Set(service.AdtySiteContextKey, &model.AdtySite{Id: 8, Status: model.AdtySiteStatusClosing, Subdomain: "closing"})
	_, _, err := findOrCreateOAuthUser(closing, provider, oauthUser, &oauth.OAuthToken{})
	var disabled *OAuthRegistrationDisabledError
	assert.ErrorAs(t, err, &disabled)
	var created int64
	require.NoError(t, db.Model(&model.User{}).Count(&created).Error)
	assert.Zero(t, created)

	active, _ := gin.CreateTestContext(httptest.NewRecorder())
	active.Request = httptest.NewRequest("GET", "/api/oauth/github", nil)
	active.Set(service.AdtySiteContextKey, &model.AdtySite{Id: 9, Status: model.AdtySiteStatusActive, Subdomain: "shop"})
	user, migration, err := findOrCreateOAuthUser(active, provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Nil(t, migration)
	require.NotNil(t, user)
	assert.Equal(t, 9, user.SiteId)
	var customer model.AdtySiteCustomer
	require.NoError(t, db.First(&customer, user.Id).Error)
	assert.Equal(t, 9, customer.SiteId)
}
