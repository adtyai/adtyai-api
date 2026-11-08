package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The old shape deliberately has no scope_version column.
type accessTokenBeforeActionVersion struct {
	Id         int
	UserId     int
	Name       string
	TokenHash  string `gorm:"uniqueIndex"`
	TokenHint  string
	Scopes     string
	ExpiresAt  int64
	LastUsedAt int64
	LastUsedIp string
	CreatedAt  int64
}

func (accessTokenBeforeActionVersion) TableName() string { return "user_access_tokens" }

func TestAccessTokenActionVersionPreservesLegacyAndVersionsNewGrants(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var db *gorm.DB
			if dialect == "postgres" {
				db = adtyTestDB(t)
			} else {
				var err error
				db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
				require.NoError(t, err)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)
				previous := DB
				DB = db
				t.Cleanup(func() { DB = previous; require.NoError(t, sqlDB.Close()) })
			}
			require.NoError(t, db.AutoMigrate(&User{}, &accessTokenBeforeActionVersion{}))
			user := User{Username: "scope-version", Password: "unused", Group: "default"}
			require.NoError(t, db.Create(&user).Error)
			old := accessTokenBeforeActionVersion{UserId: user.Id, Name: "legacy", TokenHash: "legacy-scope-hash", Scopes: `["user:write"]`}
			require.NoError(t, db.Create(&old).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&UserAccessToken{}))
			}
			require.True(t, db.Migrator().HasColumn(&UserAccessToken{}, "scope_version"), "token scope semantics need a durable version")
			var version int
			require.NoError(t, db.Model(&UserAccessToken{}).Where("id = ?", old.Id).Select("scope_version").Scan(&version).Error)
			assert.Zero(t, version, "migration must not relabel an existing grant as a new one")
			renamed, err := UpdateUserAccessToken(user.Id, old.Id, "renamed", nil)
			require.NoError(t, err)
			assert.Equal(t, []string{"user:write"}, renamed.GetScopes())
			require.NoError(t, db.Model(&UserAccessToken{}).Where("id = ?", old.Id).Select("scope_version").Scan(&version).Error)
			assert.Zero(t, version, "renaming must preserve the old compatibility boundary")
			updated, err := UpdateUserAccessToken(user.Id, old.Id, "new grant", []string{"adty_user:create"})
			require.NoError(t, err)
			assert.Equal(t, []string{"adty_user:create"}, updated.GetScopes())
			require.NoError(t, db.Model(&UserAccessToken{}).Where("id = ?", old.Id).Select("scope_version").Scan(&version).Error)
			assert.Equal(t, 2, version, "an explicit replacement grant uses the new catalog")
			fresh := &UserAccessToken{Name: "fresh", TokenHash: "new-scope-hash", Scopes: `["adty_user:read"]`}
			require.NoError(t, CreateUserAccessToken(user.Id, fresh, 20))
			require.NoError(t, db.Model(&UserAccessToken{}).Where("id = ?", fresh.Id).Select("scope_version").Scan(&version).Error)
			assert.Equal(t, 2, version, "new tokens cannot inherit legacy aliases")
			require.NoError(t, db.Model(&UserAccessToken{}).Where("id = ?", fresh.Id).Update("scope_version", 99).Error)
			unknown, err := GetUserAccessToken(user.Id, fresh.Id)
			require.NoError(t, err)
			assert.Empty(t, unknown.GetScopes(), "unknown scope versions grant nothing")
		})
	}
}
