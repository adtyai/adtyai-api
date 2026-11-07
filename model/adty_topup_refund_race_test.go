package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefundUsesPendingBatchDeltaWhenFlushHasDetachedQueue(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	useUserCacheMiniRedis(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	q := func(usd string) int64 {
		return decimal.RequireFromString(usd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	user := seedAdtyTopUpUser(t, db, "refund-detached", int(q("110")))
	seedCreditedOrder(t, db, user.Id, "RF-detached", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, int(-q("30")))

	flushDone := make(chan struct{})
	var flushOnce sync.Once
	callbackName := fmt.Sprintf("test:refund_detached_%d", time.Now().UnixNano())
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" {
			return
		}
		flushOnce.Do(func() {
			go func() {
				batchUpdate()
				close(flushDone)
			}()
		})
		// Wait for the broken ordering to detach the queue. With the fix,
		// the caller owns the queue lock and this times out before returning.
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

	// The live balance is 80 after the queued 30-unit spend, so the refund
	// should deduct 80 and return 70 principal quota (the 10-unit bonus stays).
	result, err := RefundAdtyTopUp("RF-detached", "chargeback", 1, "detached-refund-1", q("70"))
	require.NoError(t, err)
	select {
	case <-flushDone:
	case <-time.After(2 * time.Second):
		t.Fatal("batch flush did not finish")
	}
	require.Equal(t, q("80"), result.DeductedQuota)
	require.Equal(t, q("70"), result.RefundQuota)
	require.Equal(t, int64(0), int64(userQuota(t, db, user.Id)))
}
