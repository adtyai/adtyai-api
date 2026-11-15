package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// legacyOAuthBindingDB 造一个「旧库」：表在，唯一范围还是平台级的
// (provider_id, provider_user_id)，还没有 (provider, site, subject) 那条索引。
func legacyOAuthBindingDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	db := isolatedMigrationTestDatabase(t, dialect)
	require.NoError(t, db.AutoMigrate(&User{}, &UserOAuthBinding{}))
	// 现在的模型带新口径的索引，旧库里没有它。只留一个人造出来的旧索引。
	if db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex) {
		require.NoError(t, db.Migrator().DropIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
	}
	return db
}

func insertOAuthBinding(t *testing.T, db *gorm.DB, userId, siteId, providerId int, subject string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO user_oauth_bindings (user_id, site_id, provider_id, provider_user_id) VALUES (?, ?, ?, ?)",
		userId, siteId, providerId, subject,
	).Error)
}

// ISS-R4-0115：收紧唯一性的迁移不能先删后建。
//
// 这里模拟的是「旧索引已经不在了（上一次迁移删完就失败），而窗口里留下了重复行」。
// 迁移必须当场停下、把重复数出来，而不是先建索引、等数据库抛一句 duplicate key。
func TestUserOAuthBindingMigrationReportsDuplicates(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := legacyOAuthBindingDB(t, dialect)
			insertOAuthBinding(t, db, 1, 0, 7, "dup-subject")
			insertOAuthBinding(t, db, 2, 0, 7, "dup-subject")

			err := EnsureUserOAuthBindingSiteSubjectIndex(db)
			require.Error(t, err)
			// 报错要说清是重复数据要人工处理，不能只是把 DDL 的错误原样抛出来。
			assert.Contains(t, err.Error(), "duplicate user oauth bindings must be resolved")
			assert.False(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
		})
	}
}

// ISS-R4-0115：迁移失败不能留下「旧索引已经删掉」这种半成品状态。
//
// 旧索引 ux_provider_userid 被人工重建过、已经不是唯一索引时，表里可以存在重复行。
// 这时先删旧索引再建新索引，会在两条 DDL 之间把表放进「谁都不拦」的状态，建索引
// 失败后就永久留在那里。正确顺序是：先建、失败就什么都不动。
func TestUserOAuthBindingMigrationDoesNotDropLegacyIndexBeforeCreatingNewOne(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := legacyOAuthBindingDB(t, dialect)
			require.NoError(t, db.Exec(
				"CREATE INDEX ux_provider_userid ON user_oauth_bindings (provider_id, provider_user_id)",
			).Error)
			insertOAuthBinding(t, db, 1, 0, 7, "dup-subject")
			insertOAuthBinding(t, db, 2, 0, 7, "dup-subject")

			err := EnsureUserOAuthBindingSiteSubjectIndex(db)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must be resolved")
			// 旧索引还在：失败的迁移没有动过表结构。
			assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingSubjectIndex))
			assert.False(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))

			// 人工清掉重复之后重跑，迁移应当正常收尾。
			require.NoError(t, db.Exec("DELETE FROM user_oauth_bindings WHERE user_id = 2").Error)
			require.NoError(t, EnsureUserOAuthBindingSiteSubjectIndex(db))
			assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
			assert.False(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingSubjectIndex))
		})
	}
}

// ISS-R4-0115：迁移之后唯一性仍然成立，且范围是 (provider, site, subject)。
func TestUserOAuthBindingMigrationKeepsSiteScopedUniqueness(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := legacyOAuthBindingDB(t, dialect)
			require.NoError(t, db.Exec(
				"CREATE UNIQUE INDEX ux_provider_userid ON user_oauth_bindings (provider_id, provider_user_id)",
			).Error)

			require.NoError(t, EnsureUserOAuthBindingSiteSubjectIndex(db))
			assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
			assert.False(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingSubjectIndex))

			mainSite := UserOAuthBinding{UserId: 1, SiteId: 0, ProviderId: 7, ProviderUserId: "shared-subject"}
			require.NoError(t, db.Create(&mainSite).Error)
			// 同一个 provider 账号在另一个站点上：允许。
			otherSite := UserOAuthBinding{UserId: 2, SiteId: 11, ProviderId: 7, ProviderUserId: "shared-subject"}
			require.NoError(t, db.Create(&otherSite).Error)
			// 同站点重复绑定：仍然拦住。
			sameSite := UserOAuthBinding{UserId: 3, SiteId: 0, ProviderId: 7, ProviderUserId: "shared-subject"}
			assert.Error(t, db.Create(&sameSite).Error)
		})
	}
}
