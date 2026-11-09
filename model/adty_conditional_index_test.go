package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConditionalIndexesDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, kind := range []string{"oauth", "plugin", "model", "root"} {
			t.Run(dialect+"/"+kind, func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				if dialect != "sqlite" {
					sqlDB, err := db.DB()
					require.NoError(t, err)
					sqlDB.SetMaxOpenConns(4)
				}
				switch kind {
				case "oauth":
					require.NoError(t, db.AutoMigrate(&User{}, &UserSession{}))
					require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
					require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
					for _, column := range []string{"github_id", "discord_id", "oidc_id", "linux_do_id", "wechat_id"} {
						t.Run(column, func(t *testing.T) {
							first := User{Username: column + "-1", Password: "password"}
							second := User{Username: column + "-2", Password: "password"}
							require.NoError(t, db.Create(&first).Error)
							require.NoError(t, db.Create(&second).Error)
							identities := make([]AuthSessionIdentity, 2)
							for i, user := range []User{first, second} {
								session := UserSession{SID: fmt.Sprintf("%s-%d", column, i), UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: UserSessionStatusActive, ExpiresAt: time.Now().Add(time.Hour).Unix()}
								require.NoError(t, db.Create(&session).Error)
								identities[i] = AuthSessionIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
							}
							start := make(chan struct{})
							errs := make([]error, 2)
							var wg sync.WaitGroup
							for i := range identities {
								wg.Go(func() {
									<-start
									errs[i] = db.Transaction(func(tx *gorm.DB) error {
										return UpdateUserBindColumnForSessionWithTx(tx, identities[i], column, "same-subject")
									})
								})
							}
							close(start)
							wg.Wait()
							wins := 0
							for _, err := range errs {
								if err == nil {
									wins++
								} else {
									assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)
								}
							}
							assert.Equal(t, 1, wins)
							var owners []User
							require.NoError(t, db.Where(column+" = ?", "same-subject").Find(&owners).Error)
							require.Len(t, owners, 1)
							loser := first.Id
							if owners[0].Id == first.Id {
								loser = second.Id
							}
							require.NoError(t, db.Delete(&owners[0]).Error)
							assert.Error(t, db.Model(&User{}).Where("id = ?", loser).Update(column, "same-subject").Error, "soft deletion cannot release a login identity")
						})
					}
				case "plugin":
					require.NoError(t, db.AutoMigrate(&TaskPlugin{}))
					old := TaskPlugin{Key: "same", Version: "1", Active: true}
					latest := TaskPlugin{Key: "same", Version: "2", Active: true}
					require.NoError(t, db.Create(&old).Error)
					require.NoError(t, db.Create(&latest).Error)
					require.NoError(t, EnsureTaskPluginOneActiveIndex(db))
					require.NoError(t, EnsureTaskPluginOneActiveIndex(db))
					require.NoError(t, db.First(&old, old.Id).Error)
					assert.False(t, old.Active)
					assert.Error(t, db.Model(&old).Update("active", true).Error)
					assert.NoError(t, db.Create(&TaskPlugin{Key: "same", Version: "3"}).Error)
				case "model":
					require.NoError(t, db.AutoMigrate(&Model{}))
					old := Model{ModelName: "same"}
					latest := Model{ModelName: "same"}
					require.NoError(t, db.Create(&old).Error)
					require.NoError(t, db.Create(&latest).Error)
					require.NoError(t, EnsureLiveModelNameIndex(db))
					require.NoError(t, EnsureLiveModelNameIndex(db))
					require.NoError(t, db.Unscoped().First(&old, old.Id).Error)
					assert.True(t, old.DeletedAt.Valid)
					assert.Error(t, db.Create(&Model{ModelName: "same"}).Error)
					require.NoError(t, db.Delete(&latest).Error)
					assert.NoError(t, db.Create(&Model{ModelName: "same"}).Error)
				case "root":
					require.NoError(t, db.AutoMigrate(&User{}))
					require.NoError(t, EnsureOneRootUserIndex(db))
					require.NoError(t, EnsureOneRootUserIndex(db))
					root := User{Username: "root-one", Password: "password", Role: common.RoleRootUser}
					require.NoError(t, db.Create(&root).Error)
					assert.Error(t, db.Create(&User{Username: "root-two", Password: "password", Role: common.RoleRootUser}).Error)
					require.NoError(t, db.Delete(&root).Error)
					assert.NoError(t, db.Create(&User{Username: "root-three", Password: "password", Role: common.RoleRootUser}).Error)
				}
			})
		}
	}
}

func TestFullStartupMigrationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade=%t", dialect, upgrade), func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				if upgrade {
					require.NoError(t, db.AutoMigrate(&User{}))
					require.NoError(t, db.Create(&User{Id: 101, Username: "before-upgrade", Password: "password", GitHubId: "history", Quota: 123456}).Error)
				}
				for range 2 {
					require.NoError(t, migrateDB())
				}
				if upgrade {
					var stored User
					require.NoError(t, db.First(&stored, 101).Error)
					assert.Equal(t, "history", stored.GitHubId)
					assert.Equal(t, 123456, stored.Quota)
					assert.Error(t, db.Create(&User{Username: "new-owner", Password: "password", GitHubId: "history"}).Error)
				}
				logDB := isolatedMigrationTestDatabase(t, dialect)
				DB, LOG_DB = db, logDB
				if upgrade {
					require.NoError(t, logDB.AutoMigrate(&Log{}, &AuditLog{}))
					require.NoError(t, logDB.Create(&Log{Id: 201, UserId: 101, Type: LogTypeConsume, Content: "historical usage", Quota: 789}).Error)
					require.NoError(t, logDB.Create(&AuditLog{Id: 301, EventId: "historical-audit", UserId: 101, Content: "historical audit"}).Error)
				}
				for range 2 {
					require.NoError(t, migrateLOGDB())
				}
				if upgrade {
					var usage Log
					require.NoError(t, logDB.First(&usage, 201).Error)
					assert.Equal(t, "historical usage", usage.Content)
					assert.Equal(t, 789, usage.Quota)
					var audit AuditLog
					require.NoError(t, logDB.First(&audit, 301).Error)
					assert.Equal(t, "historical audit", audit.Content)
					assert.Error(t, logDB.Create(&AuditLog{EventId: "historical-audit"}).Error)
				}
			})
		}
	}
}

func TestOAuthDuplicateUpgradeDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := isolatedMigrationTestDatabase(t, dialect)
			require.NoError(t, db.AutoMigrate(&User{}))
			for _, name := range []string{"owner-one", "owner-two"} {
				require.NoError(t, db.Create(&User{Username: name, Password: "password", WeChatId: "legacy-duplicate"}).Error)
			}
			assert.ErrorContains(t, EnsureBuiltinOAuthSubjectIndexes(db), "duplicate wechat_id")
			var owners int64
			require.NoError(t, db.Model(&User{}).Where("wechat_id = ?", "legacy-duplicate").Count(&owners).Error)
			assert.EqualValues(t, 2, owners)
		})
	}
}

func TestMySQLConditionalIndexSourceUpgrade(t *testing.T) {
	db := isolatedMigrationTestDatabase(t, "mysql")
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
	require.NoError(t, db.Exec("ALTER TABLE users MODIFY github_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
	require.NoError(t, EnsureBuiltinOAuthSubjectIndexes(db))
	for i, subject := range []string{strings.Repeat("a", 240) + "A", strings.Repeat("a", 240) + "a"} {
		require.NoError(t, db.Create(&User{Username: fmt.Sprintf("long-%d", i), Password: "password", GitHubId: subject}).Error)
	}
}

func TestMySQLMigrationTableOptions(t *testing.T) {
	db := isolatedMigrationTestDatabase(t, "mysql")
	for _, tc := range []struct {
		name, options, format string
	}{
		{"default", "", "Dynamic"},
		{"custom", "ENGINE=InnoDB COMMENT='migration option'", "Dynamic"},
		{"explicit", "ENGINE=InnoDB ROW_FORMAT=COMPACT COMMENT='migration option'", "Compact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := "migration_options_" + tc.name
			require.NoError(t, db.Table(table).Set("gorm:table_options", tc.options).AutoMigrate(&migrationConstraintV1{}))
			var metadata struct {
				RowFormat, Engine, TableComment string
			}
			require.NoError(t, db.Raw("SELECT ROW_FORMAT AS row_format, ENGINE AS engine, TABLE_COMMENT AS table_comment FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", table).Scan(&metadata).Error)
			assert.Equal(t, tc.format, metadata.RowFormat)
			assert.Equal(t, "InnoDB", metadata.Engine)
			if tc.options != "" {
				assert.Equal(t, "migration option", metadata.TableComment)
			}
		})
	}
}
