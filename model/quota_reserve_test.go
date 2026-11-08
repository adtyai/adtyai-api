package model

import (
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWalletReservationsSQLiteMultipleConnections(t *testing.T) {
	for _, subject := range []string{"user", "token"} {
		t.Run(subject, func(t *testing.T) {
			previousDB, previousLogDB := DB, LOG_DB
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			initCol()
			dsn := filepath.Join(t.TempDir(), "wallet.db") + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
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
			resetBatchUpdateTestState(t)
			useUserCacheMiniRedis(t)
			common.BatchUpdateEnabled = true
			require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			table := "users"
			if subject == "token" {
				table = "tokens"
			}
			firstAtWrite, proceed := make(chan struct{}), make(chan struct{})
			var first atomic.Bool
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(proceed) }) })
			callback := "test:sqlite-wallet-write"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table && first.CompareAndSwap(false, true) {
					close(firstAtWrite)
					select {
					case <-proceed:
					case <-t.Context().Done():
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(callback)) })
			start := make(chan struct{})
			accepted, errorsByRequest := make([]bool, 2), make([]error, 2)
			var wg sync.WaitGroup
			for i := range accepted {
				wg.Go(func() {
					<-start
					if subject == "user" {
						accepted[i], errorsByRequest[i] = TryReserveUserQuota(user.Id, 80)
					} else {
						accepted[i], errorsByRequest[i] = TryReserveTokenQuota(token.Id, token.Key, 80, false)
					}
				})
			}
			close(start)
			select {
			case <-firstAtWrite:
			case <-time.After(5 * time.Second):
				t.Fatal("the first wallet writer did not reach its locked update")
			}
			waiting := assert.Eventually(t, func() bool { return sqlDB.Stats().InUse >= 2 }, 5*time.Second, 10*time.Millisecond,
				"competing reservations must use separate SQLite connections")
			release.Do(func() { close(proceed) })
			wg.Wait()
			require.True(t, waiting)
			for _, err := range errorsByRequest {
				require.NoError(t, err)
			}
			require.NotEqual(t, accepted[0], accepted[1], "only one request may spend the same 100-unit balance")
			require.NoError(t, FlushBatchUpdates())
			require.NoError(t, FlushBatchUpdates())
			if subject == "user" {
				require.Equal(t, 20, getUserQuotaFromDB(t, user.Id))
			} else {
				current := getTokenFromDB(t, token.Id)
				require.Equal(t, 20, current.RemainQuota)
				require.Equal(t, 80, current.UsedQuota)
				require.Zero(t, current.QuotaPending)
			}
			var version string
			require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
			t.Logf("SQLite %s, WAL/immediate, pool=%d; competing reservations serialized", version, sqlDB.Stats().MaxOpenConnections)
		})
	}
}

func createReserveTestUser(t *testing.T, quota int) User {
	t.Helper()
	user := User{
		Username:    "reserve-user-" + common.GetRandomString(6),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       quota,
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func createReserveTestToken(t *testing.T, remainQuota int) Token {
	t.Helper()
	token := Token{
		UserId:      1,
		Key:         "reserve-token-" + common.GetRandomString(8),
		Name:        "reserve-test",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: -1,
		RemainQuota: remainQuota,
	}
	require.NoError(t, token.Insert())
	return token
}

func getUserQuotaFromDB(t *testing.T, id int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").First(&user, id).Error)
	return user.Quota
}

func getTokenFromDB(t *testing.T, id int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.First(&token, id).Error)
	return token
}

func resetBatchUpdateTestState(t *testing.T) {
	t.Helper()
	oldBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchEnabled
		for i := range BatchUpdateTypeCount {
			batchUpdateLocks[i].Lock()
			batchUpdateStores[i] = make(map[int]int)
			batchUpdateLocks[i].Unlock()
		}
	})
}

func TestTryReserveQuotaWithoutRedis(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)

	user := createReserveTestUser(t, 100)
	reserved, err := TryReserveUserQuota(user.Id, 60)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 41)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 80)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 25, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 55, reloaded.RemainQuota)
	assert.Equal(t, 25, reloaded.UsedQuota)

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 56, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 55, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestRedisBatchReserveNeverFallsBackToStaleDatabaseBalance(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id), "batch delta is not flushed yet")

	reserved, err = TryReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.False(t, reserved, "stale DB balance must not authorize a second spend")
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, cachedUser.Quota)

	token := createReserveTestToken(t, 9)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 3, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 9, getTokenFromDB(t, token.Id).RemainQuota)

	batchUpdate()
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))
	reloadedToken := getTokenFromDB(t, token.Id)
	assert.Equal(t, 2, reloadedToken.RemainQuota)
	assert.Equal(t, 7, reloadedToken.UsedQuota)
}

func TestBatchReservationSurvivesCacheAndLocalQueueLoss(t *testing.T) {
	for _, failure := range []string{"cache deleted", "cache expired", "redis unavailable"} {
		t.Run(failure, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			server := useUserCacheMiniRedis(t)
			common.BatchUpdateEnabled = true
			user := createReserveTestUser(t, 100)

			reserved, err := TryReserveUserQuota(user.Id, 80)
			require.NoError(t, err)
			require.True(t, reserved)
			require.Equal(t, 100, getUserQuotaFromDB(t, user.Id), "the batch is not flushed yet")

			// Another process, or a restart, cannot see this process's queue.
			batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
			batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
			batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
			switch failure {
			case "cache deleted":
				server.Del(getUserCacheKey(user.Id))
			case "cache expired":
				server.FastForward(time.Hour)
			case "redis unavailable":
				server.Close()
			}

			reserved, err = TryReserveUserQuota(user.Id, 80)
			require.NoError(t, err)
			require.False(t, reserved, "an unflushed reservation must remain authoritative without Redis or the local queue")
			charged, err := ChargeUserQuotaUpTo(user.Id, 80)
			require.NoError(t, err)
			require.Equal(t, 20, charged, "only the unreserved balance remains spendable")

			FlushBatchUpdates()
			require.Equal(t, 0, getUserQuotaFromDB(t, user.Id), "flushing must recover all durable deltas without the old queue")
		})
	}
}

func TestBatchConsumptionImmediatelyReducesGiftProtection(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("checkin_quota", 50).Error)
	reserved, err := TryReserveUserQuota(user.Id, 80)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, 20, user.CheckinQuota, "spent gifts cannot remain protected until a later credit or flush")
}

func TestTokenBatchReservationSurvivesCacheAndLocalQueueLoss(t *testing.T) {
	for _, failure := range []string{"deleted", "expired", "unavailable", "restored stale amount"} {
		t.Run(failure, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			server := useUserCacheMiniRedis(t)
			common.BatchUpdateEnabled = true
			token := createReserveTestToken(t, 100)
			reserved, err := TryReserveTokenQuota(token.Id, token.Key, 80, false)
			require.NoError(t, err)
			require.True(t, reserved)
			require.Equal(t, 100, getTokenFromDB(t, token.Id).RemainQuota)
			batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
			batchUpdateStores[BatchUpdateTypeTokenQuota] = make(map[int]int)
			batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
			switch failure {
			case "deleted":
				server.Del(getTokenCacheKey(token.Key))
			case "expired":
				server.FastForward(time.Duration(tokenCacheTTLSeconds()+1) * time.Second)
			case "unavailable":
				require.NoError(t, common.RDB.Close())
			case "restored stale amount":
				require.NoError(t, common.RDB.HSet(t.Context(), getTokenCacheKey(token.Key), "RemainQuota", 100, "UsedQuota", 0).Err())
			}
			reserved, err = TryReserveTokenQuota(token.Id, token.Key, 80, false)
			require.NoError(t, err)
			require.False(t, reserved, "a lost cache or queue cannot restore spent token quota")
			current, err := GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			require.Equal(t, 20, current.RemainQuota)
			require.Equal(t, 80, current.UsedQuota)
			reserved, err = TryReserveTokenQuota(token.Id, token.Key, 20, false)
			require.NoError(t, err)
			require.True(t, reserved)
			FlushBatchUpdates()
			stored := getTokenFromDB(t, token.Id)
			require.Zero(t, stored.RemainQuota)
			require.Equal(t, 100, stored.UsedQuota)
			FlushBatchUpdates()
			require.Equal(t, 100, getTokenFromDB(t, token.Id).UsedQuota)
		})
	}
}

func TestTokenMetadataMutationDoesNotRestorePendingSpend(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	token := createReserveTestToken(t, 100)
	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 80, false)
	require.NoError(t, err)
	require.True(t, reserved)
	token.Name = "renamed while pending"
	require.NoError(t, token.Update())
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 80, false)
	require.NoError(t, err)
	require.False(t, reserved)
}

func TestReplacingTokenLimitCommitsPendingUsageOnce(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	token := createReserveTestToken(t, 100)
	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 80, false)
	require.NoError(t, err)
	require.True(t, reserved)
	current, err := GetTokenByIds(token.Id, token.UserId)
	require.NoError(t, err)
	current.RemainQuota = 50
	require.NoError(t, current.UpdateReplacingRemainQuota())
	FlushBatchUpdates()
	stored := getTokenFromDB(t, token.Id)
	require.Equal(t, 50, stored.RemainQuota, "the old pending spend cannot reduce a replacement limit twice")
	require.Equal(t, 80, stored.UsedQuota)
	require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 20))
	current, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.Equal(t, 70, current.RemainQuota)
	require.Equal(t, 60, current.UsedQuota)
	FlushBatchUpdates()
	stored = getTokenFromDB(t, token.Id)
	require.Equal(t, 70, stored.RemainQuota)
	require.Equal(t, 60, stored.UsedQuota)
}

func TestDurableQuotaMigrationAndRestartRecovery(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	// A representative prior-version schema has every existing field but no
	// durable pending columns. Upgrading it must preserve rows and set zero.
	require.NoError(t, db.Migrator().DropColumn(&User{}, "QuotaPending"))
	require.NoError(t, db.Migrator().DropColumn(&Token{}, "QuotaPending"))
	for range 2 {
		require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	}
	var unchanged User
	require.NoError(t, db.First(&unchanged, user.Id).Error)
	require.Equal(t, 100, unchanged.Quota)
	require.Zero(t, unchanged.QuotaPending)
	storedToken := getTokenFromDB(t, token.Id)
	require.Equal(t, 100, storedToken.RemainQuota)
	require.Zero(t, storedToken.QuotaPending)
	require.NoError(t, DecreaseUserQuota(user.Id, 80, false))
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 70))
	// Repeated migration cannot clear amounts already committed by a new node.
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, db.First(&unchanged, user.Id).Error)
	require.Equal(t, -80, unchanged.QuotaPending)
	storedToken = getTokenFromDB(t, token.Id)
	require.Equal(t, -70, storedToken.QuotaPending)
	// Account/token soft deletion does not erase a committed delta. Simulate a
	// restart with no process-local hints, then flush the soft-deleted rows.
	require.NoError(t, db.Delete(&user).Error)
	require.NoError(t, db.Delete(&token).Error)
	for _, kind := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
		batchUpdateLocks[kind].Lock()
		batchUpdateStores[kind] = make(map[int]int)
		batchUpdateLocks[kind].Unlock()
	}
	require.NoError(t, FlushBatchUpdates())
	require.NoError(t, db.Unscoped().First(&unchanged, user.Id).Error)
	require.Equal(t, 20, unchanged.Quota)
	require.Zero(t, unchanged.QuotaPending)
	require.NoError(t, db.Unscoped().First(&storedToken, token.Id).Error)
	require.Equal(t, 30, storedToken.RemainQuota)
	require.Equal(t, 70, storedToken.UsedQuota)
	require.Zero(t, storedToken.QuotaPending)
	require.NoError(t, FlushBatchUpdates())
}

func TestBatchCreditRetainsNegativeRawRowUntilFlush(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 0)

	require.NoError(t, IncreaseUserQuota(user.Id, 100, false))
	require.NoError(t, DecreaseUserQuota(user.Id, 80, true))
	require.Equal(t, -80, getUserQuotaFromDB(t, user.Id), "a direct debit may precede an unflushed credit")

	charged, err := ChargeUserQuotaUpTo(user.Id, 50)
	require.NoError(t, err)
	require.Equal(t, 20, charged, "the logical balance includes the durable positive credit")
	require.Equal(t, -80, getUserQuotaFromDB(t, user.Id), "do not clamp the raw row or prematurely flush the credit")
	FlushBatchUpdates()
	require.Equal(t, 0, getUserQuotaFromDB(t, user.Id))
}

func TestBatchUpdateAccumulatesTwoMaximumRequestCharges(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, common.MaxQuota*2+100)
	require.NoError(t, DecreaseUserQuota(user.Id, common.MaxQuota, false))
	require.NoError(t, DecreaseUserQuota(user.Id, common.MaxQuota, false))

	batchUpdate()
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
}

func TestBatchUpdateAccumulatorSaturatesOverflow(t *testing.T) {
	resetBatchUpdateTestState(t)

	addNewRecord(BatchUpdateTypeUserQuota, 1, math.MaxInt)
	addNewRecord(BatchUpdateTypeUserQuota, 1, 1)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	assert.Equal(t, math.MaxInt, batchUpdateStores[BatchUpdateTypeUserQuota][1])
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	addNewRecord(BatchUpdateTypeUserQuota, 1, math.MinInt)
	addNewRecord(BatchUpdateTypeUserQuota, 1, -1)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	assert.Equal(t, math.MinInt, batchUpdateStores[BatchUpdateTypeUserQuota][1])
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
}

func TestReserveFallsBackToDatabaseWhenRedisIsUnavailable(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 20)
	require.NoError(t, populateUserCache(user))
	server.Close()

	// Redis 故障时降级为数据库条件更新：服务保持可用且不会超扣。
	reserved, err := TryReserveUserQuota(user.Id, 5)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 16)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))
}

func TestSynchronousReserveCompensatesCacheWhenPersistenceFails(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, DB.Delete(&user).Error)

	reserved, err := TryReserveUserQuota(user.Id, 6)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 10, cached.Quota)

	token := createReserveTestToken(t, 12)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, DB.Delete(&token).Error)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	cachedToken, cacheErr := cacheGetTokenByKey(token.Key)
	require.NoError(t, cacheErr)
	assert.Equal(t, 12, cachedToken.RemainQuota)
	assert.Zero(t, cachedToken.UsedQuota)
}

// 余额下限：结算最多扣到 0，不透支。同一组用例在 SQLite（上游测试库）和
// PostgreSQL（adtyTestDB）上各跑一遍，覆盖 Redis 开关和 BatchUpdateEnabled 开关。
func runChargeUserQuotaUpToCases(t *testing.T, prepare func(t *testing.T)) {
	cases := []struct {
		name        string
		balance     int
		charge      int
		wantCharged int
		wantBalance int
	}{
		{name: "扣足", balance: 100, charge: 60, wantCharged: 60, wantBalance: 40},
		{name: "刚好扣完", balance: 60, charge: 60, wantCharged: 60, wantBalance: 0},
		{name: "不够时扣到 0", balance: 10, charge: 225, wantCharged: 10, wantBalance: 0},
		{name: "余额已为 0", balance: 0, charge: 5, wantCharged: 0, wantBalance: 0},
		{name: "余额已为负", balance: -7, charge: 5, wantCharged: 0, wantBalance: -7},
	}
	modes := []struct {
		name  string
		redis bool
		batch bool
	}{
		{name: "直写", redis: false, batch: false},
		{name: "批量无Redis", redis: false, batch: true},
		{name: "Redis直写", redis: true, batch: false},
		{name: "Redis批量", redis: true, batch: true},
	}
	for _, mode := range modes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				prepare(t)
				resetBatchUpdateTestState(t)
				if mode.redis {
					useUserCacheMiniRedis(t)
				}
				common.BatchUpdateEnabled = mode.batch

				user := createReserveTestUser(t, tc.balance)
				charged, err := ChargeUserQuotaUpTo(user.Id, tc.charge)
				require.NoError(t, err)
				assert.Equal(t, tc.wantCharged, charged)

				if mode.redis {
					cached, err := GetUserCache(user.Id)
					require.NoError(t, err)
					assert.Equal(t, tc.wantBalance, cached.Quota, "缓存余额")
				}
				if mode.redis && mode.batch {
					// 缓存侧扣减走批量队列，刷新前数据库不变。
					assert.Equal(t, tc.balance, getUserQuotaFromDB(t, user.Id))
					batchUpdate()
				}
				// Redis 关闭时即使开了批量也直接条件更新数据库，不进队列。
				assert.Equal(t, tc.wantBalance, getUserQuotaFromDB(t, user.Id), "数据库余额")
			})
		}
	}
}

func TestChargeUserQuotaUpToSQLite(t *testing.T) {
	runChargeUserQuotaUpToCases(t, truncateTables)
}

func TestChargeUserQuotaUpToPostgres(t *testing.T) {
	runChargeUserQuotaUpToCases(t, func(t *testing.T) {
		db := adtyTestDB(t)
		require.NoError(t, db.AutoMigrate(&User{}))
	})
}

func TestChargeUserQuotaUpToRejectsNegativeAmount(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	user := createReserveTestUser(t, 10)
	charged, err := ChargeUserQuotaUpTo(user.Id, -1)
	assert.Error(t, err)
	assert.Zero(t, charged)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
}

// 并发结算：总实扣等于原余额，余额停在 0（PostgreSQL 行锁下验证）。
func TestChargeUserQuotaUpToConcurrentNeverOverdraws(t *testing.T) {
	for _, redisOn := range []bool{false, true} {
		t.Run(map[bool]string{false: "无Redis", true: "Redis"}[redisOn], func(t *testing.T) {
			db := adtyTestDB(t)
			require.NoError(t, db.AutoMigrate(&User{}))
			resetBatchUpdateTestState(t)
			if redisOn {
				useUserCacheMiniRedis(t)
			}
			user := createReserveTestUser(t, 5)

			var wg sync.WaitGroup
			var mu sync.Mutex
			total := 0
			for range 20 {
				wg.Go(func() {
					charged, err := ChargeUserQuotaUpTo(user.Id, 2)
					assert.NoError(t, err)
					mu.Lock()
					total += charged
					mu.Unlock()
				})
			}
			wg.Wait()
			assert.Equal(t, 5, total)
			assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))
		})
	}
}

func TestTokenCacheInitPreservesLiveQuotaAndFenceBlocksStaleSnapshot(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	token := createReserveTestToken(t, 100)
	loaded, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	stale := *loaded

	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 70))

	// A stale cache snapshot cannot replace the durable amount used by readers.
	code, err := cacheInitToken(stale)
	require.NoError(t, err)
	assert.Equal(t, 2, code)
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 30, cached.RemainQuota)

	// 变更期间：fence 删除缓存并拦截并发读者手中的过期快照。
	require.NoError(t, invalidateTokenCacheForMutation(token.Key))
	code, err = cacheInitToken(stale)
	require.NoError(t, err)
	assert.Zero(t, code, "the pre-mutation snapshot must not be published while fenced")
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err)

	// Metadata hydration still uses the committed amount.
	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	fresh, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 30, fresh.RemainQuota)
	cached, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 30, cached.RemainQuota)
}
