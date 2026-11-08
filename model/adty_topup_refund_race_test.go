package model

import (
	"testing"

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
	require.NoError(t, DecreaseUserQuota(user.Id, int(q("30")), false))
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	// The live balance is 80 after the queued 30-unit spend, so the refund
	// should deduct 80 and return 70 principal quota (the 10-unit bonus stays).
	result, err := RefundAdtyTopUp("RF-detached", "chargeback", 1, "detached-refund-1", q("70"))
	require.NoError(t, err)
	require.Equal(t, q("80"), result.DeductedQuota)
	require.Equal(t, q("70"), result.RefundQuota)
	// The queued spend is still unflushed, so the row keeps that 30 until the
	// next batch flush. The refund itself has already removed the live 80.
	require.Equal(t, int(q("30")), userQuota(t, db, user.Id))
	FlushBatchUpdates()
	require.Zero(t, userQuota(t, db, user.Id))
}

func TestSpentCheckinGiftDoesNotProtectLaterTopUp(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	useUserCacheMiniRedis(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true
	user := seedAdtyTopUpUser(t, db, "gift-spent-topup", 100)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("checkin_quota", 50).Error)
	reserved, err := TryReserveUserQuota(user.Id, 80)
	require.NoError(t, err)
	require.True(t, reserved)
	seedCreditedOrder(t, db, user.Id, "RF-gift-spent", PaymentProviderAdtyBEpusdt, "1", "0", "0", "1")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return creditTopUpQuota(tx, user.Id, 500000, nil)
	}))
	preview, err := PreviewAdtyTopUpRefund("RF-gift-spent")
	require.NoError(t, err)
	require.EqualValues(t, 500000, preview.RefundQuota, "later principal cannot revive the spent part of a gift")
	result, err := RefundAdtyTopUp("RF-gift-spent", "synthetic refund", 1, "gift-spent-refund", 500000)
	require.NoError(t, err)
	require.EqualValues(t, 500000, result.DeductedQuota)
	FlushBatchUpdates()
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 20, stored.Quota)
	require.Equal(t, 20, stored.CheckinQuota)
}
