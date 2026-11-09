package model

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 客户归属只由分站服务改（执行层决定 29）：用户改资料、管理员编辑用户都不能改 site_id。
func TestUserUpdateKeepsSiteId(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	user := User{Username: "site_customer", Password: "x", Group: "default", Status: 1, Role: 1, SiteId: 7}
	require.NoError(t, db.Create(&user).Error)

	update := User{Id: user.Id, DisplayName: "renamed", SiteId: 0}
	require.NoError(t, update.Update(false))
	update = User{Id: user.Id, DisplayName: "renamed again", SiteId: 9}
	require.NoError(t, update.Update(false))

	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 7, stored.SiteId)
	assert.Equal(t, "renamed again", stored.DisplayName)
}

func TestSiteMigrationRevokesKeysDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := isolatedMigrationTestDatabase(t, dialect)
			require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
			server := useUserCacheMiniRedis(t)
			user := User{Username: "moved", Password: "x", Group: "default", SiteId: 7, Quota: 100}
			require.NoError(t, db.Create(&user).Error)
			token := Token{UserId: user.Id, Key: "migrationCachedSecret", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, RemainQuota: 60, UsedQuota: 40}
			require.NoError(t, db.Create(&token).Error)
			require.NoError(t, cacheSetTokenForTest(token))
			_, err := ValidateUserToken(token.Key)
			require.NoError(t, err)
			server.SetError("READONLY")
			err = db.Transaction(func(tx *gorm.DB) error {
				_, err := AdtySetUsersSiteTx(tx, 7, 9)
				return err
			})
			require.Error(t, err, "failed credential invalidation must not migrate the user")
			server.SetError("")
			var before User
			require.NoError(t, db.First(&before, user.Id).Error)
			assert.Equal(t, 7, before.SiteId)

			writeFailure := errors.New("migration membership write failed")
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("migration_fail_user", func(tx *gorm.DB) {
				updates, ok := tx.Statement.Dest.(map[string]any)
				if _, moving := updates["site_id"]; ok && moving && tx.Statement.Table == "users" {
					_ = tx.AddError(writeFailure)
				}
			}))
			err = db.Transaction(func(tx *gorm.DB) error {
				_, err := AdtySetUsersSiteTx(tx, 7, 9)
				return err
			})
			require.ErrorIs(t, err, writeFailure)
			require.NoError(t, db.Callback().Update().Remove("migration_fail_user"))
			var stored User
			require.NoError(t, db.First(&stored, user.Id).Error)
			assert.Equal(t, 7, stored.SiteId)
			assert.Equal(t, user.AuthVersion, stored.AuthVersion)
			_, err = ValidateUserToken(token.Key)
			require.NoError(t, err, "a rolled back migration must leave the original credential valid")

			var moved []int
			err = db.Transaction(func(tx *gorm.DB) error {
				var err error
				moved, err = AdtySetUsersSiteTx(tx, 7, 9)
				return err
			})
			require.NoError(t, err)
			assert.Equal(t, []int{user.Id}, moved)
			AdtyInvalidateUserCaches(moved)
			require.NoError(t, db.First(&stored, user.Id).Error)
			assert.Equal(t, 9, stored.SiteId)
			assert.Equal(t, user.AuthVersion+1, stored.AuthVersion)
			assert.Equal(t, 100, stored.Quota)
			var retired Token
			require.NoError(t, db.Unscoped().First(&retired, token.Id).Error)
			assert.True(t, retired.DeletedAt.Valid)
			assert.Equal(t, 60, retired.RemainQuota)
			assert.Equal(t, 40, retired.UsedQuota)
			// Restore the complete pre-migration Redis history, including generation.
			server.FlushAll()
			require.NoError(t, cacheSetTokenForTest(token))
			cached, err := cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			assert.Equal(t, common.TokenStatusEnabled, cached.Status)
			_, err = ValidateUserToken(token.Key)
			assert.Error(t, err, "recovered Redis metadata cannot authorize a retired credential")
			assert.ErrorIs(t, db.First(&Token{}, token.Id).Error, gorm.ErrRecordNotFound)
			token.Status = common.TokenStatusEnabled
			assert.ErrorIs(t, token.Update(), gorm.ErrRecordNotFound, "editing the old token must not restore the secret")
		})
	}
}

func TestSiteMigrationSerializesTokenCreation(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		for _, first := range []string{"creation", "migration"} {
			t.Run(dialect+"/"+first, func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(4)
				require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
				previousRedis := common.RedisEnabled
				common.RedisEnabled = false
				t.Cleanup(func() { common.RedisEnabled = previousRedis })
				user := User{Username: "concurrent-key-owner", Password: "x", Group: "default", SiteId: 7}
				require.NoError(t, db.Create(&user).Error)
				token := Token{UserId: user.Id, Key: "concurrentMigrationKey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
				locked, attempted, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var releaseOnce, attemptedOnce sync.Once
				t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
				if first == "creation" {
					require.NoError(t, db.Callback().Create().Before("gorm:create").Register("migration_hold_creation", func(tx *gorm.DB) {
						if tx.Statement.Table == "tokens" {
							close(locked)
							<-release
						}
					}))
					t.Cleanup(func() { _ = db.Callback().Create().Remove("migration_hold_creation") })
				} else {
					require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("migration_hold_delete", func(tx *gorm.DB) {
						if tx.Statement.Table == "tokens" {
							close(locked)
							<-release
						}
					}))
					t.Cleanup(func() { _ = db.Callback().Delete().Remove("migration_hold_delete") })
				}
				migrate := func() error {
					return db.Transaction(func(tx *gorm.DB) error {
						_, err := AdtySetUsersSiteTx(tx, 7, 9)
						return err
					})
				}
				create := func() error { return token.InsertWithLimit(100) }
				leading, following := create, migrate
				if first == "migration" {
					leading, following = migrate, create
				}
				results := make(chan error, 2)
				go func() { results <- leading() }()
				select {
				case <-locked:
				case <-time.After(10 * time.Second):
					t.Fatal("leading operation did not lock the user")
				}
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("migration_follower_lock", func(tx *gorm.DB) {
					if tx.Statement.Table == "users" {
						attemptedOnce.Do(func() { close(attempted) })
					}
				}))
				t.Cleanup(func() { _ = db.Callback().Query().Remove("migration_follower_lock") })
				go func() { results <- following() }()
				select {
				case <-attempted:
				case <-time.After(10 * time.Second):
					t.Fatal("following operation did not attempt the user lock")
				}
				releaseOnce.Do(func() { close(release) })
				for range 2 {
					select {
					case err := <-results:
						require.NoError(t, err)
					case <-time.After(10 * time.Second):
						t.Fatal("migration and token creation did not finish")
					}
				}
				var stored Token
				require.NoError(t, db.Unscoped().First(&stored, token.Id).Error)
				assert.Equal(t, first == "creation", stored.DeletedAt.Valid, "only a key committed before migration is retired")
				var moved User
				require.NoError(t, db.First(&moved, user.Id).Error)
				assert.Equal(t, 9, moved.SiteId)
			})
		}
	}
}
