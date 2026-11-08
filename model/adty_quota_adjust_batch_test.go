package model

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdjustRejectsSubtractHiddenByPendingBatch(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := &User{
		Username: "batch-floor", Password: "x", Group: "default",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000,
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DecreaseUserQuota(user.Id, 900, false))

	_, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 500, "pending floor", "pending-floor-1")
	require.ErrorIs(t, err, ErrAdtyQuotaFloor)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, 1000, stored.Quota)
	var ledgers int64
	require.NoError(t, DB.Model(&AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&ledgers).Error)
	require.Zero(t, ledgers)
}

func TestAdjustRejectsOverrideAfterLocalPendingHintIsLost(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := &User{
		Username: "batch-floor-detached", Password: "x", Group: "default",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000,
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DecreaseUserQuota(user.Id, 900, false))
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	_, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "override", 500, "pending floor", "detached-floor-1")
	require.ErrorIs(t, err, ErrAdtyQuotaFloor)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, 1000, stored.Quota)
}

func TestAdjustRejectsSubtractDuringDetachedFlush(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 100)
	reserved, err := TryReserveUserQuota(user.Id, 80)
	require.NoError(t, err)
	require.True(t, reserved)
	// The database must enforce the floor even without a live cache.
	common.RedisEnabled = false

	entered, release := make(chan struct{}), make(chan struct{})
	const callback = "audit_detached_wallet_flush"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if !ok {
			return
		}
		if _, ok := updates["request_count"]; !ok {
			return
		}
		close(entered)
		<-release
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	flushed := make(chan struct{})
	go func() {
		FlushBatchUpdates()
		close(flushed)
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("flush did not reach the detached database write")
	}
	adjusted := make(chan error, 1)
	go func() {
		_, adjustErr := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 50, "detached floor", "detached-real-1")
		adjusted <- adjustErr
	}()
	var adjustErr error
	select {
	case adjustErr = <-adjusted:
		close(release)
	case <-time.After(100 * time.Millisecond):
		close(release)
		adjustErr = <-adjusted
	}
	<-flushed
	require.ErrorIs(t, adjustErr, ErrAdtyQuotaFloor)
	require.Equal(t, 20, getUserQuotaFromDB(t, user.Id))
	var count int64
	require.NoError(t, db.Model(&AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&count).Error)
	require.Zero(t, count)
}

func TestAdjustRollbackKeepsDurableBalanceAndCacheUnchanged(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	useUserCacheMiniRedis(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 1000)
	require.NoError(t, DecreaseUserQuota(user.Id, 900, false))
	before, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, 100, before.Quota)
	failure := errors.New("synthetic ledger write failure")
	const callback = "audit_adjust_ledger_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "adty_balance_ledgers" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })
	_, err = AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "add", 50, "rollback", "rollback-adjust")
	require.ErrorIs(t, err, failure)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 1000, stored.Quota)
	require.Equal(t, -900, stored.QuotaPending)
	after, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, 100, after.Quota)

	var count int64
	require.NoError(t, db.Model(&AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&count).Error)
	require.Zero(t, count)
}

func TestAdjustCommitsOnceWhenMetadataCacheIsUnavailable(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	server := useUserCacheMiniRedis(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 1000)
	require.NoError(t, DecreaseUserQuota(user.Id, 900, false))
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)
	server.SetError("READONLY")
	first, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "add", 50, "cache outage", "cache-adjust")
	require.NoError(t, err, "cache failures cannot turn a committed amount into a retryable failure")
	require.Equal(t, 1050, first.After)
	again, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "add", 50, "cache outage", "cache-adjust")
	require.NoError(t, err)
	require.True(t, again.Replayed)
	server.SetError("")
	current, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, 150, current.Quota, "a stale metadata hash cannot hide or duplicate the committed credit")
}

func TestAdjustUsesLogicalBoundsForTransientRawRows(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	for _, withLedger := range []bool{false, true} {
		for _, tc := range []struct {
			name, mode                          string
			raw, pending, value, after, logical int
		}{
			{name: "positive pending permits subtraction", mode: "subtract", raw: 10, pending: 100, value: 50, after: -40, logical: 60},
			{name: "raw above ceiling with spent pending", mode: "subtract", raw: common.MaxWalletQuota + 50, pending: -100, value: 25, after: common.MaxWalletQuota + 25, logical: common.MaxWalletQuota - 75},
			{name: "add can reduce an existing debt", mode: "add", raw: 10, pending: -100, value: 50, after: 60, logical: -40},
		} {
			t.Run(fmt.Sprintf("ledger=%t/%s", withLedger, tc.name), func(t *testing.T) {
				user := createReserveTestUser(t, tc.raw)
				require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota_pending", tc.pending).Error)
				if withLedger {
					adjusted, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, tc.mode, tc.value, "logical bound", fmt.Sprintf("logical-%d", user.Id))
					require.NoError(t, err)
					require.Equal(t, tc.raw, adjusted.Before)
					require.Equal(t, tc.after, adjusted.After)
				} else {
					adjusted, err := AdjustUserQuota(user.Id, common.RoleRootUser, tc.mode, tc.value)
					require.NoError(t, err)
					require.Equal(t, tc.raw, adjusted.Before)
					require.Equal(t, tc.after, adjusted.After)
				}
				balance, err := GetUserQuota(user.Id, false)
				require.NoError(t, err)
				require.Equal(t, tc.logical, balance)
			})
		}
	}
}

func TestOverrideRejectsUnrepresentableAuditDelta(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	for _, withLedger := range []bool{false, true} {
		user := createReserveTestUser(t, math.MinInt)
		var err error
		if withLedger {
			_, err = AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "override", 0, "extreme legacy debt", fmt.Sprintf("extreme-%d", user.Id))
		} else {
			_, err = AdjustUserQuota(user.Id, common.RoleRootUser, "override", 0)
		}
		require.ErrorIs(t, err, ErrWalletQuotaLimitExceeded)
		require.Equal(t, math.MinInt, getUserQuotaFromDB(t, user.Id))
	}
}

func TestAdminAdjustmentNonnegativeBoundaryWithPendingBalance(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	for _, withLedger := range []bool{false, true} {
		for _, tc := range []struct {
			name, mode                string
			raw, pending, value, want int
			rejected                  bool
		}{
			{name: "subtract exactly live balance", mode: "subtract", raw: 100, pending: -80, value: 20, want: 0},
			{name: "subtract exceeds live balance", mode: "subtract", raw: 100, pending: -80, value: 21, rejected: true},
			{name: "override zero", mode: "override", raw: 100, value: 0, want: 0},
			{name: "negative override with positive pending", mode: "override", raw: 100, pending: 80, value: -1, rejected: true},
		} {
			t.Run(fmt.Sprintf("ledger=%t/%s", withLedger, tc.name), func(t *testing.T) {
				user := createReserveTestUser(t, tc.raw)
				require.NoError(t, db.Model(user).Update("quota_pending", tc.pending).Error)
				var err error
				if withLedger {
					_, err = AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, tc.mode, tc.value, "admin floor boundary", fmt.Sprintf("boundary-%d", user.Id))
				} else {
					_, err = AdjustUserQuota(user.Id, common.RoleRootUser, tc.mode, tc.value)
				}
				if tc.rejected {
					require.ErrorIs(t, err, ErrAdtyQuotaFloor)
					require.Equal(t, tc.raw, getUserQuotaFromDB(t, user.Id))
				} else {
					require.NoError(t, err)
					balance, readErr := GetUserQuota(user.Id, false)
					require.NoError(t, readErr)
					require.Equal(t, tc.want, balance)
				}
			})
		}
	}
}
