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
	"gorm.io/gorm"
)

func setupOAuthSiteIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.UserSession{},
		&model.AdtySiteCustomer{},
		&model.AdtyUserFingerprint{},
		&model.ExternalIdentityClaim{},
	))
	require.NoError(t, model.EnsureExternalIdentityClaimSiteIndexes(db))
	require.NoError(t, model.EnsureBuiltinOAuthSubjectIndexes(db))
	require.NoError(t, model.EnsureSiteEmailUniqueIndex(db))
	previousRegister, previousRedis := common.RegisterEnabled, common.RedisEnabled
	common.RegisterEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RegisterEnabled = previousRegister
		common.RedisEnabled = previousRedis
	})
	return db
}

func oauthSiteContext(siteID int) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/oauth/telegram", nil)
	if siteID > 0 {
		c.Set(service.AdtySiteContextKey, &model.AdtySite{Id: siteID, Status: model.AdtySiteStatusActive, Subdomain: "shop"})
	}
	return c
}

// Telegram is the only provider still registered. It never signs anyone in, but
// its historical binding lookup still has to stay scoped to the current site.
func TestFindOrCreateTelegramUserIsSiteScoped(t *testing.T) {
	db := setupOAuthSiteIdentityDB(t)
	mainUser := &model.User{
		Username: "tg-identity-main", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, SiteId: 0, TelegramId: "tg-identity-subject",
	}
	require.NoError(t, db.Create(mainUser).Error)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.ClaimExternalIdentityWithTx(tx, model.ExternalIdentityProviderTelegram, "tg-identity-subject", mainUser.Id)
	}))

	provider := &oauth.TelegramProvider{}
	oauthUser := &oauth.OAuthUser{ProviderUserID: "tg-identity-subject"}
	found, err := findOrCreateOAuthUser(oauthSiteContext(0), provider, oauthUser)
	require.NoError(t, err)
	assert.Equal(t, mainUser.Id, found.Id)

	missing, err := findOrCreateOAuthUser(oauthSiteContext(11), provider, oauthUser)
	assert.Nil(t, missing)
	assert.ErrorIs(t, err, oauth.ErrTelegramAccountNotBound)
}
