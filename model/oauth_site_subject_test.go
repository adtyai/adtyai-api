package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOAuthSiteSubjectDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &UserSession{}, &ExternalIdentityClaim{}, &CustomOAuthProvider{}, &UserOAuthBinding{}))
	require.NoError(t, EnsureExternalIdentityClaimSiteIndexes(db))
	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
	require.NoError(t, EnsureUserOAuthBindingSiteSubjectIndex(db))
	require.NoError(t, EnsureSiteEmailUniqueIndex(db))
	return db
}

func createSiteSubjectUser(t *testing.T, username, subject string, siteID int, email string) *User {
	t.Helper()
	user := &User{
		Username: username,
		Password: "password",
		Email:    email,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		SiteId:   siteID,
		GitHubId: subject,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestBuiltinOAuthSubjectIsUniquePerSite(t *testing.T) {
	db := setupOAuthSiteSubjectDB(t)

	mainUser := createSiteSubjectUser(t, "gh-main-owner", "gh-shared-subject", 0, "shared-oauth@example.com")
	siteUser := createSiteSubjectUser(t, "gh-site-owner", "gh-shared-subject", 11, "shared-oauth@example.com")
	assert.NotEqual(t, mainUser.Id, siteUser.Id)
	assert.True(t, IsGitHubIdAlreadyTaken("gh-shared-subject", 0))
	assert.True(t, IsGitHubIdAlreadyTaken("gh-shared-subject", 11))
	assert.False(t, IsGitHubIdAlreadyTaken("gh-shared-subject", 12))

	filled := User{GitHubId: "gh-shared-subject", SiteId: 0}
	require.NoError(t, filled.FillUserByGitHubId())
	assert.Equal(t, mainUser.Id, filled.Id)
	filled = User{GitHubId: "gh-shared-subject", SiteId: 11}
	require.NoError(t, filled.FillUserByGitHubId())
	assert.Equal(t, siteUser.Id, filled.Id)

	sameSite := &User{
		Username: "gh-same-site-dup", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, SiteId: 0, GitHubId: "gh-shared-subject",
	}
	assert.Error(t, db.Create(sameSite).Error)

	require.NoError(t, AssignBuiltinProviderIDsWithTx(db, &User{Id: siteUser.Id, GitHubId: "gh-shared-subject"}))
	other := &User{
		Username: "gh-same-site-assign", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, SiteId: 11,
	}
	require.NoError(t, db.Create(other).Error)
	other.GitHubId = "gh-shared-subject"
	assert.ErrorIs(t, AssignBuiltinProviderIDsWithTx(db, other), ErrExternalIdentityAlreadyClaimed)
}

func TestBuiltinOAuthUnbindDoesNotStealOtherSiteBinding(t *testing.T) {
	db := setupOAuthSiteSubjectDB(t)
	mainUser := createSiteSubjectUser(t, "gh-main-unbind", "gh-unbind-subject", 0, "")
	siteUser := createSiteSubjectUser(t, "gh-site-unbind", "gh-unbind-subject", 11, "")

	require.NoError(t, siteUser.ClearBinding("github"))
	assert.Empty(t, siteUser.GitHubId)

	var stored User
	require.NoError(t, db.First(&stored, mainUser.Id).Error)
	assert.Equal(t, "gh-unbind-subject", stored.GitHubId)
	assert.True(t, IsGitHubIdAlreadyTaken("gh-unbind-subject", 0))
	assert.False(t, IsGitHubIdAlreadyTaken("gh-unbind-subject", 11))

	replacement := createSiteSubjectUser(t, "gh-site-rebind", "gh-unbind-subject", 11, "")
	require.NoError(t, db.First(&stored, mainUser.Id).Error)
	assert.Equal(t, "gh-unbind-subject", stored.GitHubId)
	assert.Equal(t, "gh-unbind-subject", replacement.GitHubId)
	assert.NotEqual(t, mainUser.Id, replacement.Id)
}

func TestTelegramLookupAndClaimAreSiteScoped(t *testing.T) {
	db := setupOAuthSiteSubjectDB(t)

	mainUser := &User{Username: "tg-main", Password: "password", TelegramId: "tg-shared", Status: common.UserStatusEnabled, SiteId: 0}
	siteUser := &User{Username: "tg-site", Password: "password", TelegramId: "tg-shared", Status: common.UserStatusEnabled, SiteId: 11}
	require.NoError(t, db.Create(mainUser).Error)
	require.NoError(t, db.Create(siteUser).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "tg-shared", mainUser.Id)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "tg-shared", siteUser.Id)
	}))

	found, err := GetUserByTelegramID("tg-shared", 0)
	require.NoError(t, err)
	assert.Equal(t, mainUser.Id, found.Id)
	found, err = GetUserByTelegramID("tg-shared", 11)
	require.NoError(t, err)
	assert.Equal(t, siteUser.Id, found.Id)
	_, err = GetUserByTelegramID("tg-shared", 12)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.True(t, IsTelegramIdAlreadyTaken("tg-shared", 0))
	assert.False(t, IsTelegramIdAlreadyTaken("tg-shared", 12))

	other := &User{Username: "tg-same-site", Password: "password", Status: common.UserStatusEnabled, SiteId: 0}
	require.NoError(t, db.Create(other).Error)
	err = db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "tg-shared", other.Id)
	})
	assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)

	require.NoError(t, siteUser.ClearBinding("telegram"))
	var stored User
	require.NoError(t, db.First(&stored, mainUser.Id).Error)
	assert.Equal(t, "tg-shared", stored.TelegramId)
	_, err = GetUserByTelegramID("tg-shared", 11)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestGenericOAuthBindingIsUniquePerSite(t *testing.T) {
	db := setupOAuthSiteSubjectDB(t)
	provider := &CustomOAuthProvider{Name: "Custom Site", Slug: "custom-site-scope", Enabled: true, ClientId: "client"}
	require.NoError(t, db.Create(provider).Error)

	mainUser := &User{Username: "generic-main", Password: "password", Status: common.UserStatusEnabled, SiteId: 0}
	siteUser := &User{Username: "generic-site", Password: "password", Status: common.UserStatusEnabled, SiteId: 11}
	sameSite := &User{Username: "generic-same", Password: "password", Status: common.UserStatusEnabled, SiteId: 0}
	require.NoError(t, db.Create(mainUser).Error)
	require.NoError(t, db.Create(siteUser).Error)
	require.NoError(t, db.Create(sameSite).Error)

	require.NoError(t, CreateUserOAuthBinding(&UserOAuthBinding{UserId: mainUser.Id, ProviderId: provider.Id, ProviderUserId: "generic-subject"}))
	require.NoError(t, CreateUserOAuthBinding(&UserOAuthBinding{UserId: siteUser.Id, ProviderId: provider.Id, ProviderUserId: "generic-subject"}))
	err := CreateUserOAuthBinding(&UserOAuthBinding{UserId: sameSite.Id, ProviderId: provider.Id, ProviderUserId: "generic-subject"})
	assert.EqualError(t, err, "this OAuth account is already bound to another user")

	assert.True(t, IsProviderUserIdTaken(provider.Id, "generic-subject", 0))
	assert.True(t, IsProviderUserIdTaken(provider.Id, "generic-subject", 11))
	assert.False(t, IsProviderUserIdTaken(provider.Id, "generic-subject", 12))

	found, err := GetUserByOAuthBinding(provider.Id, "generic-subject", 0)
	require.NoError(t, err)
	assert.Equal(t, mainUser.Id, found.Id)
	found, err = GetUserByOAuthBinding(provider.Id, "generic-subject", 11)
	require.NoError(t, err)
	assert.Equal(t, siteUser.Id, found.Id)

	require.NoError(t, DeleteUserOAuthBinding(siteUser.Id, provider.Id))
	assert.False(t, IsProviderUserIdTaken(provider.Id, "generic-subject", 11))
	assert.True(t, IsProviderUserIdTaken(provider.Id, "generic-subject", 0))
	found, err = GetUserByOAuthBinding(provider.Id, "generic-subject", 0)
	require.NoError(t, err)
	assert.Equal(t, mainUser.Id, found.Id)
}

func TestEnsureBuiltinOAuthSubjectIndexesAllowsCrossSiteAndDropsLegacy(t *testing.T) {
	db := setupOAuthSiteSubjectDB(t)
	for _, column := range []string{"github_id", "discord_id", "oidc_id", "linux_do_id", "wechat_id", "telegram_id"} {
		assert.False(t, db.Migrator().HasIndex(&User{}, legacyBuiltinOAuthSubjectIndexName(column)), column)
		assert.True(t, db.Migrator().HasIndex(&User{}, builtinOAuthSubjectIndexName(column)), column)
	}

	// Recreate the legacy global unique index and prove Ensure replaces it.
	require.NoError(t, db.Exec("DROP INDEX IF EXISTS ux_users_github_id_site_subject").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX ux_users_github_id_subject ON users (github_id) WHERE github_id <> ''").Error)
	require.NoError(t, db.Create(&User{Username: "legacy-main", Password: "password", GitHubId: "legacy-cross", SiteId: 0}).Error)
	assert.Error(t, db.Create(&User{Username: "legacy-site", Password: "password", GitHubId: "legacy-cross", SiteId: 11}).Error)

	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
	assert.False(t, db.Migrator().HasIndex(&User{}, "ux_users_github_id_subject"))
	assert.True(t, db.Migrator().HasIndex(&User{}, "ux_users_github_id_site_subject"))
	require.NoError(t, db.Create(&User{Username: "legacy-site", Password: "password", GitHubId: "legacy-cross", SiteId: 11}).Error)
	assert.Error(t, db.Create(&User{Username: "legacy-main-dup", Password: "password", GitHubId: "legacy-cross", SiteId: 0}).Error)
}

func TestOAuthSiteSubjectIndexesDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := isolatedMigrationTestDatabase(t, dialect)
			require.NoError(t, db.AutoMigrate(&User{}, &UserSession{}, &ExternalIdentityClaim{}, &CustomOAuthProvider{}, &UserOAuthBinding{}))
			require.NoError(t, EnsureExternalIdentityClaimSiteIndexes(db))
			require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
			require.NoError(t, EnsureUserOAuthBindingSiteSubjectIndex(db))
			require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))

			first := User{Username: "matrix-main", Password: "password", GitHubId: "matrix-subject", SiteId: 0}
			second := User{Username: "matrix-site", Password: "password", GitHubId: "matrix-subject", SiteId: 7}
			require.NoError(t, db.Create(&first).Error)
			require.NoError(t, db.Create(&second).Error)
			assert.Error(t, db.Create(&User{Username: "matrix-dup", Password: "password", GitHubId: "matrix-subject", SiteId: 0}).Error)

			identities := make([]AuthSessionIdentity, 2)
			for i, user := range []User{first, second} {
				session := UserSession{
					SID: fmt.Sprintf("matrix-%s-%d", dialect, i), UserID: user.Id, Version: 1,
					UserAuthVersion: 1, Status: UserSessionStatusActive, ExpiresAt: time.Now().Add(time.Hour).Unix(),
				}
				require.NoError(t, db.Create(&session).Error)
				identities[i] = AuthSessionIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
			}
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				return UpdateUserBindColumnForSessionWithTx(tx, identities[1], "github_id", "matrix-rebind")
			}))
			var stored User
			require.NoError(t, db.First(&stored, first.Id).Error)
			assert.Equal(t, "matrix-subject", stored.GitHubId)
		})
	}
}
