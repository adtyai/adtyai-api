package model

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdjustRejectsSubtractHiddenByPendingBatch(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := &User{
		Username: "batch-floor", Password: "x", Group: "default",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000,
	}
	require.NoError(t, DB.Create(user).Error)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -900)

	_, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 500, "pending floor", "pending-floor-1")
	require.ErrorIs(t, err, ErrAdtyQuotaFloor)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, 1000, stored.Quota)
	var ledgers int64
	require.NoError(t, DB.Model(&AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&ledgers).Error)
	require.Zero(t, ledgers)
}

func TestAdjustRejectsSubtractWhileBatchFlushHasDetachedPendingDelta(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &AdtyBalanceLedger{}))
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := &User{
		Username: "batch-floor-detached", Password: "x", Group: "default",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000,
	}
	require.NoError(t, DB.Create(user).Error)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -900)

	flushStarted := make(chan struct{})
	flushDone := make(chan struct{})
	callbackName := fmt.Sprintf("test:batch_floor_detached_%d", time.Now().UnixNano())
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" {
			return
		}
		select {
		case <-flushStarted:
			return
		default:
			close(flushStarted)
		}
		go func() {
			batchUpdate()
			close(flushDone)
		}()
		// The broken ordering lets batchUpdate swap the queue while the
		// transaction still owns the user row. The fixed ordering keeps the
		// queue lock until after this query, so do not wait indefinitely.
		deadline := time.NewTimer(200 * time.Millisecond)
		defer deadline.Stop()
		for {
			if !batchUpdateLocks[BatchUpdateTypeUserQuota].TryLock() {
				select {
				case <-deadline.C:
					return
				case <-time.After(time.Millisecond):
					continue
				}
			}
			detached := len(batchUpdateStores[BatchUpdateTypeUserQuota]) == 0
			batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
			if detached {
				return
			}
			select {
			case <-deadline.C:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	resultCh := make(chan error, 1)
	go func() {
		_, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 500, "pending floor", "detached-floor-1")
		resultCh <- err
	}()
	err := <-resultCh
	select {
	case <-flushDone:
	case <-time.After(2 * time.Second):
		t.Fatal("batch flush did not finish")
	}
	if !errors.Is(err, ErrAdtyQuotaFloor) {
		t.Fatalf("expected detached pending delta to reject adjustment, got %v", err)
	}

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, 100, stored.Quota)
}

func TestAdjustCacheCompensateFailureIsReviewed(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&PaymentOrphanEvent{}))
	previous := compensateManualQuotaDelta
	compensateManualQuotaDelta = func(int, int64) (cacheQuotaResult, error) {
		return cacheQuotaMiss, ErrAdtyQuotaFloor
	}
	t.Cleanup(func() { compensateManualQuotaDelta = previous })

	compensateManualAdjustCache(41, 100)
	var count int64
	require.NoError(t, db.Model(&PaymentOrphanEvent{}).Where("event_type = ? AND reference_id = ?", "adjust_cache_compensate", "41").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
