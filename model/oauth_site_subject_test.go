package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOAuthSiteSubjectDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &UserSession{}, &ExternalIdentityClaim{}))
	require.NoError(t, EnsureExternalIdentityClaimSiteIndexes(db))
	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
	require.NoError(t, EnsureSiteEmailUniqueIndex(db))
	return db
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

// Legacy login columns keep their per-site unique indexes even though no login
// path writes them any more; the migration must still converge old databases.
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
			require.NoError(t, db.AutoMigrate(&User{}, &ExternalIdentityClaim{}))
			require.NoError(t, EnsureExternalIdentityClaimSiteIndexes(db))
			require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
			require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))

			first := User{Username: "matrix-main", Password: "password", GitHubId: "matrix-subject", SiteId: 0}
			second := User{Username: "matrix-site", Password: "password", GitHubId: "matrix-subject", SiteId: 7}
			require.NoError(t, db.Create(&first).Error)
			require.NoError(t, db.Create(&second).Error)
			assert.Error(t, db.Create(&User{Username: "matrix-dup", Password: "password", GitHubId: "matrix-subject", SiteId: 0}).Error)
		})
	}
}
