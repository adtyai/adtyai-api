package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tokenInvalidationFailureHook struct{ fail atomic.Bool }

func (hook *tokenInvalidationFailureHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	args := cmd.Args()
	if hook.fail.Load() && cmd.Name() == "eval" && len(args) > 1 {
		if script, ok := args[1].(string); ok && strings.Contains(script, "redis.call('INCR'") {
			return ctx, errors.New("READONLY token invalidation write rejected")
		}
	}
	return ctx, nil
}

func (*tokenInvalidationFailureHook) AfterProcess(context.Context, redis.Cmder) error { return nil }
func (*tokenInvalidationFailureHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (*tokenInvalidationFailureHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestTokenConcurrencyPostgreSQL(t *testing.T) {
	dsn, redisAddr := os.Getenv("TEST_AUTH_TOKEN_POSTGRES_DSN"), os.Getenv("TEST_AUTH_TOKEN_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("isolated token PostgreSQL and Redis endpoints are not configured")
	}
	config, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.Equal(t, "adty_auth_token_20261108", config.Database, "refuse another database")
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, config.Host)
	previousDB, previousLogDB := DB, LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousRDB := common.RedisEnabled, common.RDB
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	initCol()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	newRedis := func() *redis.Client { return redis.NewClient(&redis.Options{Addr: redisAddr, DB: 7}) }
	common.RedisEnabled, common.RDB = true, newRedis()
	require.NoError(t, common.RDB.Ping(t.Context()).Err())
	t.Cleanup(func() {
		var version int
		assert.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
		t.Logf("PostgreSQL after: %d; Redis DB: 7", version)
		_ = common.RDB.Close()
		_ = sqlDB.Close()
		DB, LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.RDB = previousRedis, previousRDB
		common.SetDatabaseTypes(previousMain, previousLog)
		initCol()
	})
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Positive(t, version)
	t.Logf("server_version_num=%d; Redis DB: 7", version)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &AdtyGroup{}, &Channel{}, &Ability{}))

	seed := func(t *testing.T) Token {
		t.Helper()
		name := fmt.Sprintf("auth-token-%d", time.Now().UnixNano())
		owner := User{Username: name, Password: "password", Group: "default", Status: common.UserStatusEnabled}
		require.NoError(t, db.Create(&owner).Error)
		token := Token{UserId: owner.Id, Key: name, Name: name, Group: "auto", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}
		require.NoError(t, token.Insert())
		t.Cleanup(func() {
			var owned []Token
			require.NoError(t, db.Unscoped().Where("user_id = ? OR id = ?", owner.Id, token.Id).Find(&owned).Error)
			for _, item := range owned {
				require.NoError(t, common.RDB.Del(context.Background(), getTokenCacheKey(item.Key), getTokenCacheFenceKey(item.Key), getTokenCacheGenerationKey(item.Key)).Err())
			}
			require.NoError(t, common.RDB.Del(context.Background(), getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key), getTokenCacheGenerationKey(token.Key)).Err())
			require.NoError(t, db.Unscoped().Where("user_id = ? OR id = ?", owner.Id, token.Id).Delete(&Token{}).Error)
			require.NoError(t, db.Unscoped().Delete(&owner).Error)
		})
		return token
	}
	seedMigration := func(t *testing.T) (Token, string, string) {
		t.Helper()
		token := seed(t)
		from, to := fmt.Sprintf("from-%d", token.Id), fmt.Sprintf("to-%d", token.Id)
		for _, id := range []string{from, to} {
			require.NoError(t, db.Create(&AdtyGroup{ID: id, Names: AdtyLangText{"en": id}, Descs: AdtyLangText{}}).Error)
		}
		t.Cleanup(func() {
			require.NoError(t, db.Unscoped().Where("id IN ?", []string{from, to}).Delete(&AdtyGroup{}).Error)
			require.NoError(t, common.RDB.Del(context.Background(), getUserCacheKey(token.UserId), getUserAuthVersionKey(token.UserId), getUserAuthFenceKey(token.UserId)).Err())
		})
		return token, from, to
	}

	for _, mutation := range []string{"disable", "expire", "delete", "group_migration", "metadata", "key_rotation"} {
		t.Run("redis_snapshot_restore_"+mutation, func(t *testing.T) {
			token, from, to := seedMigration(t)
			token.Group = from
			token.UnlimitedQuota, token.AllowUserQuotaQuery = true, true
			require.NoError(t, token.SetAutoGroups([]string{from, to}))
			require.NoError(t, token.Update())
			_, err := GetTokenByKey(token.Key, true)
			require.NoError(t, err)
			oldHash, err := common.RDB.Dump(t.Context(), getTokenCacheKey(token.Key)).Result()
			require.NoError(t, err)
			oldGeneration, err := common.RDB.Dump(t.Context(), getTokenCacheGenerationKey(token.Key)).Result()
			require.NoError(t, err)
			switch mutation {
			case "disable":
				token.Status = common.TokenStatusDisabled
				require.NoError(t, token.Update())
			case "expire":
				token.ExpiredTime = time.Now().Unix() - 60
				require.NoError(t, token.Update())
			case "delete":
				require.NoError(t, token.Delete())
			case "group_migration":
				_, err := MigrateAdtyGroup(from, to)
				require.NoError(t, err)
			case "metadata":
				token.Group, token.Name = to, "restricted"
				token.ModelLimitsEnabled, token.ModelLimits = true, "authorized-model"
				token.CrossGroupRetry, token.AllowUserQuotaQuery = true, false
				token.UnlimitedQuota = false
				ipRange := "203.0.113.0/24"
				token.AllowIps = &ipRange
				require.NoError(t, token.SetAutoGroups([]string{to}))
				require.NoError(t, token.Update())
				other := seed(t)
				token.UserId = other.UserId
				require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Update("user_id", other.UserId).Error)
			case "key_rotation":
				require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Update(commonKeyCol, token.Key+"-rotated").Error)
			}
			// Restore the actual old Redis values, including the generation. Client
			// reconnects alone do not reproduce an old replica/snapshot returning.
			require.NoError(t, common.RDB.RestoreReplace(t.Context(), getTokenCacheKey(token.Key), time.Minute, oldHash).Err())
			require.NoError(t, common.RDB.RestoreReplace(t.Context(), getTokenCacheGenerationKey(token.Key), 0, oldGeneration).Err())
			old, err := cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			require.Equal(t, common.TokenStatusEnabled, old.Status)
			require.Equal(t, from, old.Group, "the real Redis restore must expose the old snapshot")
			actual, err := ValidateUserToken(token.Key)
			if mutation == "disable" || mutation == "expire" || mutation == "delete" || mutation == "key_rotation" {
				assert.ErrorIs(t, err, ErrTokenInvalid, "a restored Redis snapshot cannot revive authorization")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, to, actual.Group)
			groups, err := actual.GetAutoGroups()
			require.NoError(t, err)
			assert.Equal(t, []string{to}, groups)
			if mutation == "metadata" {
				assert.Equal(t, token.UserId, actual.UserId)
				assert.Equal(t, "restricted", actual.Name)
				assert.True(t, actual.ModelLimitsEnabled)
				assert.Equal(t, "authorized-model", actual.ModelLimits)
				assert.Equal(t, token.AllowIps, actual.AllowIps)
				assert.True(t, actual.CrossGroupRetry)
				assert.False(t, actual.AllowUserQuotaQuery)
				assert.False(t, actual.UnlimitedQuota)
			}
		})
	}

	t.Run("hot_cache_database_failure_is_closed", func(t *testing.T) {
		token := seed(t)
		_, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		callback := "test:hot-cache-database-failure"
		require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "tokens" {
				tx.AddError(assert.AnError)
			}
		}))
		t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
		actual, err := GetTokenByKey(token.Key, false)
		assert.Nil(t, actual)
		assert.Error(t, err, "Redis must not authorize when durable metadata cannot be checked")
		_, err = ValidateUserToken(token.Key)
		assert.ErrorIs(t, err, ErrDatabase)
	})

	for _, operation := range []string{"create", "update"} {
		t.Run("migration_lock_order_"+operation, func(t *testing.T) {
			token, from, to := seedMigration(t)
			if operation == "create" {
				require.NoError(t, db.Model(&User{}).Where("id = ?", token.UserId).Update(commonGroupCol, from).Error)
			} else {
				token.Group = from
				require.NoError(t, db.Model(&token).Update(commonGroupCol, from).Error)
			}
			migrationAtUsers, proceed := make(chan struct{}), make(chan struct{})
			var seen atomic.Bool
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(proceed) }) })
			callback := "test:migration-lock-order"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				where, ok := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
				if !ok || tx.Statement.Table != "users" {
					return
				}
				for _, expression := range where.Exprs {
					if expression, ok := expression.(clause.Expr); ok && expression.SQL == commonGroupCol+" = ?" && len(expression.Vars) == 1 && expression.Vars[0] == from && seen.CompareAndSwap(false, true) {
						close(migrationAtUsers)
						select {
						case <-proceed:
						case <-t.Context().Done():
						}
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
			migrated := make(chan error, 1)
			go func() { _, err := MigrateAdtyGroup(from, to); migrated <- err }()
			select {
			case <-migrationAtUsers:
			case <-time.After(5 * time.Second):
				t.Fatal("migration did not acquire the group locks")
			}
			changed := make(chan error, 1)
			go func() {
				if operation == "create" {
					next := Token{UserId: token.UserId, Key: token.Key + "-created", Group: from}
					changed <- next.InsertWithLimit(2)
					return
				}
				changed <- token.Update()
			}()
			waiting := assert.Eventually(t, func() bool {
				var count int
				err := db.Raw("SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%adty_groups%' AND query LIKE '%FOR UPDATE%'").Scan(&count).Error
				return err == nil && count > 0
			}, 5*time.Second, 10*time.Millisecond, "the writer must reach the held group lock")
			release.Do(func() { close(proceed) })
			migrationErr, changeErr := <-migrated, <-changed
			require.True(t, waiting)
			assert.NoError(t, migrationErr, "group migration must share the writer's lock order")
			assert.NoError(t, changeErr, "token writer must not invert the migration's group lock order")
		})
	}

	for _, failureAt := range []string{"before_sql", "after_sql"} {
		t.Run("group_migration_invalidation_"+failureAt, func(t *testing.T) {
			token, from, to := seedMigration(t)
			token.Group = from
			require.NoError(t, db.Model(&User{}).Where("id = ?", token.UserId).Update(commonGroupCol, from).Error)
			require.NoError(t, token.SetAutoGroups([]string{from, to}))
			require.NoError(t, db.Model(&token).Updates(map[string]any{commonGroupCol: from, "auto_groups": token.AutoGroups}).Error)
			snapshot, err := GetTokenByKey(token.Key, true)
			require.NoError(t, err)
			hook := &tokenInvalidationFailureHook{}
			common.RDB.AddHook(hook)
			t.Cleanup(func() { hook.fail.Store(false) })
			if failureAt == "before_sql" {
				hook.fail.Store(true)
			} else {
				callback := "test:migration-post-sql-redis-failure"
				require.NoError(t, db.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "tokens" && tx.Error == nil {
						hook.fail.Store(true)
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(callback)) })
			}
			moved, err := MigrateAdtyGroup(from, to)
			var stored Token
			require.NoError(t, db.First(&stored, token.Id).Error)
			if failureAt == "before_sql" {
				assert.Error(t, err, "failed pre-invalidation must roll back the entire migration")
				assert.Equal(t, AdtyGroupRefs{}, moved)
				assert.Equal(t, from, stored.Group)
				assert.Equal(t, snapshot.AutoGroups, stored.AutoGroups)
				var owner User
				require.NoError(t, db.First(&owner, token.UserId).Error)
				assert.Equal(t, from, owner.Group)
				return
			}
			require.NoError(t, err, "post-commit invalidation failure must not misreport a rollback")
			assert.EqualValues(t, 1, moved.Tokens)
			assert.Equal(t, to, stored.Group)
			_, err = cacheInitToken(*snapshot)
			require.NoError(t, err)
			validated, err := ValidateUserToken(token.Key)
			require.NoError(t, err)
			assert.Equal(t, to, validated.Group, "readable Redis must never retain the old authorization group")
			groups, err := validated.GetAutoGroups()
			require.NoError(t, err)
			assert.Equal(t, []string{to}, groups)
		})
	}

	t.Run("owner_limit", func(t *testing.T) {
		token := seed(t)
		start := make(chan struct{})
		errorsByRequest := make([]error, 2)
		var wg sync.WaitGroup
		for i := range errorsByRequest {
			wg.Go(func() {
				<-start
				next := Token{UserId: token.UserId, Key: fmt.Sprintf("%s-%d", token.Key, i), Group: "auto"}
				errorsByRequest[i] = next.InsertWithLimit(2)
			})
		}
		close(start)
		wg.Wait()
		succeeded, rejected := 0, 0
		for _, err := range errorsByRequest {
			if err == nil {
				succeeded++
			} else {
				assert.ErrorIs(t, err, ErrTokenLimit)
				rejected++
			}
		}
		assert.Equal(t, 1, succeeded)
		assert.Equal(t, 1, rejected)
		var count int64
		require.NoError(t, db.Model(&Token{}).Where("user_id = ?", token.UserId).Count(&count).Error)
		assert.EqualValues(t, 2, count)
	})

	for _, mutation := range []string{"disable", "delete", "batch_delete"} {
		t.Run(mutation, func(t *testing.T) {
			token := seed(t)
			snapshot, err := GetTokenByKey(token.Key, true)
			require.NoError(t, err)
			switch mutation {
			case "disable":
				token.Status = common.TokenStatusDisabled
				require.NoError(t, token.Update())
			case "delete":
				require.NoError(t, token.Delete())
			case "batch_delete":
				n, err := BatchDeleteTokens([]int{token.Id}, token.UserId)
				require.NoError(t, err)
				assert.Equal(t, 1, n)
			}
			_, err = cacheInitToken(*snapshot)
			require.NoError(t, err)
			_, err = ValidateUserToken(token.Key)
			assert.ErrorIs(t, err, ErrTokenInvalid)
		})
	}

	t.Run("rollback_and_redis_recovery", func(t *testing.T) {
		token := seed(t)
		snapshot, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		forced := errors.New("token metadata rollback")
		err = token.mutateMetadata(func(tx *gorm.DB) error {
			if err := tx.Model(&token).Update("status", common.TokenStatusDisabled).Error; err != nil {
				return err
			}
			return forced
		})
		require.ErrorIs(t, err, forced)
		_, err = ValidateUserToken(token.Key)
		require.NoError(t, err)
		_, err = cacheGetTokenByKey(token.Key)
		require.NoError(t, err, "rollback must not strand cache hydration")
		require.NoError(t, common.RDB.Close())
		token.Status = common.TokenStatusDisabled
		require.Error(t, token.Update(), "a failed pre-write invalidation must stop the SQL mutation")
		unchanged, err := ValidateUserToken(token.Key)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusEnabled, unchanged.Status)
		common.RDB = newRedis()
		require.NoError(t, common.RDB.Ping(t.Context()).Err())
		// Disconnect this test client after the SQL write, before COMMIT returns.
		// The server and its generation remain intact, as during a transport fault.
		err = token.mutateMetadata(func(tx *gorm.DB) error {
			if err := tx.Model(&token).Update("status", common.TokenStatusDisabled).Error; err != nil {
				return err
			}
			return common.RDB.Close()
		})
		require.NoError(t, err, "a committed mutation remains successful when post-commit cleanup fails")
		_, err = ValidateUserToken(token.Key)
		assert.ErrorIs(t, err, ErrTokenInvalid)
		common.RDB = newRedis()
		require.NoError(t, common.RDB.Ping(t.Context()).Err())
		_, err = cacheInitToken(*snapshot)
		require.NoError(t, err)
		_, err = ValidateUserToken(token.Key)
		assert.ErrorIs(t, err, ErrTokenInvalid)
		cached, err := cacheGetTokenByKey(token.Key)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusDisabled, cached.Status)
	})

	t.Run("live_quota_is_not_overwritten", func(t *testing.T) {
		token := seed(t)
		snapshot, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 70))
		_, err = cacheInitToken(*snapshot)
		require.NoError(t, err)
		authorized, err := GetTokenByKey(token.Key, false)
		require.NoError(t, err)
		assert.Equal(t, 30, authorized.RemainQuota, "stale cache publication cannot restore a committed spend")
	})

	t.Run("reader_waits_for_slow_metadata_commit", func(t *testing.T) {
		token := seed(t)
		writerAtSQL, readerAtSQL, proceed := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var updateSeen, querySeen atomic.Bool
		var release sync.Once
		t.Cleanup(func() { release.Do(func() { close(proceed) }) })
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:slow-token-update", func(tx *gorm.DB) {
			if tx.Statement.Table == "tokens" && updateSeen.CompareAndSwap(false, true) {
				close(writerAtSQL)
				select {
				case <-proceed:
				case <-t.Context().Done():
				}
			}
		}))
		require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:slow-token-reader", func(tx *gorm.DB) {
			where, ok := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
			if !ok || tx.Statement.Table != "tokens" {
				return
			}
			for _, expr := range where.Exprs {
				if expr, ok := expr.(clause.Expr); ok {
					for _, value := range expr.Vars {
						if value == token.Key && querySeen.CompareAndSwap(false, true) {
							close(readerAtSQL)
						}
					}
				}
			}
		}))
		t.Cleanup(func() {
			require.NoError(t, db.Callback().Update().Remove("test:slow-token-update"))
			require.NoError(t, db.Callback().Query().Remove("test:slow-token-reader"))
		})
		token.Status = common.TokenStatusDisabled
		updated := make(chan error, 1)
		go func() { updated <- token.Update() }()
		select {
		case <-writerAtSQL:
		case <-time.After(10 * time.Second):
			t.Fatal("writer did not reach the controlled SQL boundary")
		}
		type readResult struct {
			token *Token
			err   error
		}
		read := make(chan readResult, 1)
		go func() { token, err := GetTokenByKey(token.Key, false); read <- readResult{token, err} }()
		select {
		case <-readerAtSQL:
		case <-time.After(10 * time.Second):
			t.Fatal("reader did not reach the controlled SQL boundary")
		}
		// Exercise a real pause longer than the previous ten-second Redis fence.
		timer := time.NewTimer(11 * time.Second)
		<-timer.C
		release.Do(func() { close(proceed) })
		require.NoError(t, <-updated)
		result := <-read
		require.NoError(t, result.err)
		require.NotNil(t, result.token)
		assert.Equal(t, common.TokenStatusDisabled, result.token.Status)
		_, err := ValidateUserToken(token.Key)
		assert.ErrorIs(t, err, ErrTokenInvalid)
	})
}

func TestTokenOwnerLimitSQLiteMultipleConnections(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	dsn := filepath.Join(t.TempDir(), "tokens.db") + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		initCol()
	})
	useUserCacheMiniRedis(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &AdtyGroup{}))
	owner := User{Username: "sqlite-owner", Password: "password", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&owner).Error)
	start := make(chan struct{})
	firstAtInsert, proceed := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(proceed) }) })
	callback := "test:sqlite-owner-create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" && first.CompareAndSwap(false, true) {
			close(firstAtInsert)
			select {
			case <-proceed:
			case <-t.Context().Done():
			}
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(callback)) })
	errorsByRequest := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errorsByRequest {
		wg.Go(func() {
			<-start
			token := Token{UserId: owner.Id, Key: fmt.Sprintf("sqlite-token-%d", i), Group: "auto", ExpiredTime: -1, UnlimitedQuota: true}
			errorsByRequest[i] = token.InsertWithLimit(1)
		})
	}
	close(start)
	select {
	case <-firstAtInsert:
	case <-time.After(5 * time.Second):
		t.Fatal("the first SQLite writer did not reach the locked insert")
	}
	waiting := assert.Eventually(t, func() bool { return sqlDB.Stats().InUse >= 2 }, 5*time.Second, 10*time.Millisecond,
		"the competing writer must use a separate SQLite connection")
	release.Do(func() { close(proceed) })
	wg.Wait()
	require.True(t, waiting)
	succeeded, rejected := 0, 0
	for _, err := range errorsByRequest {
		if err == nil {
			succeeded++
		} else {
			assert.ErrorIs(t, err, ErrTokenLimit)
			rejected++
		}
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, rejected)
	var token Token
	require.NoError(t, db.Where("user_id = ?", owner.Id).First(&token).Error)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, db.Model(&token).Update("status", common.TokenStatusDisabled).Error)
	_, err = ValidateUserToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid, "a committed SQLite change overrides the still-warm cache")
	var version string
	require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
	t.Logf("SQLite %s; max connections: %d; production WAL/immediate DSN", version, sqlDB.Stats().MaxOpenConnections)
}
