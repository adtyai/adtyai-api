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
		&model.CustomOAuthProvider{},
		&model.UserOAuthBinding{},
	))
	require.NoError(t, model.EnsureExternalIdentityClaimSiteIndexes(db))
	require.NoError(t, model.EnsureBuiltinOAuthSubjectIndexes(db))
	require.NoError(t, model.EnsureUserOAuthBindingSiteSubjectIndex(db))
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
	c.Request = httptest.NewRequest("GET", "/api/oauth/github", nil)
	if siteID > 0 {
		c.Set(service.AdtySiteContextKey, &model.AdtySite{Id: siteID, Status: model.AdtySiteStatusActive, Subdomain: "shop"})
	}
	return c
}

func TestFindOrCreateOAuthUserSplitsSubjectBySite(t *testing.T) {
	db := setupOAuthSiteIdentityDB(t)
	provider := &oauth.GitHubProvider{}
	oauthUser := &oauth.OAuthUser{ProviderUserID: "gh-site-subject", Username: "ghsubjectuser"}

	main, migration, err := findOrCreateOAuthUser(oauthSiteContext(0), provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Nil(t, migration)
	require.NotNil(t, main)
	assert.Zero(t, main.SiteId)
	assert.Equal(t, "gh-site-subject", main.GitHubId)

	site, migration, err := findOrCreateOAuthUser(oauthSiteContext(11), provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Nil(t, migration)
	require.NotNil(t, site)
	assert.Equal(t, 11, site.SiteId)
	assert.Equal(t, "gh-site-subject", site.GitHubId)
	assert.NotEqual(t, main.Id, site.Id)

	var stored model.User
	require.NoError(t, db.First(&stored, main.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, "gh-site-subject", stored.GitHubId)
	assert.NotEqual(t, stored.Username, site.Username)

	// Login on site B still returns the site B user, not the main-site account.
	again, _, err := findOrCreateOAuthUser(oauthSiteContext(11), provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Equal(t, site.Id, again.Id)
	assert.Equal(t, 11, again.SiteId)

	// Registration disabled on a third site must not fall back to another site's user.
	previous := common.RegisterEnabled
	common.RegisterEnabled = false
	t.Cleanup(func() { common.RegisterEnabled = previous })
	missing, _, err := findOrCreateOAuthUser(oauthSiteContext(12), provider, oauthUser, &oauth.OAuthToken{})
	assert.Nil(t, missing)
	var disabled *OAuthRegistrationDisabledError
	assert.ErrorAs(t, err, &disabled)
}

func TestFindOrCreateOAuthUserRejectsSameSiteSubjectDuplicate(t *testing.T) {
	db := setupOAuthSiteIdentityDB(t)
	existing := &model.User{
		Username: "gh-existing", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, SiteId: 11, GitHubId: "gh-same-site",
	}
	require.NoError(t, db.Create(existing).Error)

	other := &model.User{
		Username: "gh-other-same-site", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, SiteId: 11,
	}
	require.NoError(t, db.Create(other).Error)
	other.GitHubId = "gh-same-site"
	assert.ErrorIs(t, model.AssignBuiltinProviderIDsWithTx(db, other), model.ErrExternalIdentityAlreadyClaimed)

	user, _, err := findOrCreateOAuthUser(oauthSiteContext(11), &oauth.GitHubProvider{}, &oauth.OAuthUser{ProviderUserID: "gh-same-site"}, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Equal(t, existing.Id, user.Id)
}

func TestFindOrCreateOAuthUserDoesNotMergeEmailAcrossSites(t *testing.T) {
	setupOAuthSiteIdentityDB(t)
	provider := &oauth.GitHubProvider{}
	main, _, err := findOrCreateOAuthUser(oauthSiteContext(0), provider, &oauth.OAuthUser{
		ProviderUserID: "gh-email-main", Username: "ghemailmain", Email: "Shared.OAuth@Example.com",
	}, &oauth.OAuthToken{})
	require.NoError(t, err)
	require.Equal(t, "shared.oauth@example.com", main.Email)
	assert.Zero(t, main.SiteId)

	site, _, err := findOrCreateOAuthUser(oauthSiteContext(11), provider, &oauth.OAuthUser{
		ProviderUserID: "gh-email-site", Username: "ghemailsite", Email: "shared.oauth@example.com",
	}, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.NotEqual(t, main.Id, site.Id)
	assert.Equal(t, 11, site.SiteId)
	assert.Equal(t, "shared.oauth@example.com", site.Email)
	assert.Equal(t, "gh-email-site", site.GitHubId)
	assert.Equal(t, "gh-email-main", main.GitHubId)

	_, _, err = findOrCreateOAuthUser(oauthSiteContext(11), provider, &oauth.OAuthUser{
		ProviderUserID: "gh-email-site-dup", Username: "ghemaildup", Email: "shared.oauth@example.com",
	}, &oauth.OAuthToken{})
	var taken *OAuthEmailAlreadyTakenError
	assert.ErrorAs(t, err, &taken)
}

func TestFindOrCreateGenericOAuthUserIsSiteScoped(t *testing.T) {
	db := setupOAuthSiteIdentityDB(t)
	cfg := &model.CustomOAuthProvider{Name: "Custom", Slug: "custom-identity-site", Enabled: true, ClientId: "client"}
	require.NoError(t, db.Create(cfg).Error)
	provider := oauth.NewGenericOAuthProvider(cfg)
	oauthUser := &oauth.OAuthUser{ProviderUserID: "generic-site-subject", Username: "genericusera"}

	main, _, err := findOrCreateOAuthUser(oauthSiteContext(0), provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Zero(t, main.SiteId)
	assert.True(t, model.IsProviderUserIdTaken(cfg.Id, "generic-site-subject", 0))
	assert.False(t, model.IsProviderUserIdTaken(cfg.Id, "generic-site-subject", 11))

	site, _, err := findOrCreateOAuthUser(oauthSiteContext(11), provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Equal(t, 11, site.SiteId)
	assert.NotEqual(t, main.Id, site.Id)
	assert.True(t, model.IsProviderUserIdTaken(cfg.Id, "generic-site-subject", 11))

	found, err := model.GetUserByOAuthBinding(cfg.Id, "generic-site-subject", 0)
	require.NoError(t, err)
	assert.Equal(t, main.Id, found.Id)
	found, err = model.GetUserByOAuthBinding(cfg.Id, "generic-site-subject", 11)
	require.NoError(t, err)
	assert.Equal(t, site.Id, found.Id)
}

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
	found, _, err := findOrCreateOAuthUser(oauthSiteContext(0), provider, oauthUser, &oauth.OAuthToken{})
	require.NoError(t, err)
	assert.Equal(t, mainUser.Id, found.Id)

	missing, _, err := findOrCreateOAuthUser(oauthSiteContext(11), provider, oauthUser, &oauth.OAuthToken{})
	assert.Nil(t, missing)
	assert.ErrorIs(t, err, oauth.ErrTelegramAccountNotBound)
}
