package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
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

	// The live balance is 80 after the queued 30-unit spend, so the refund
	// should deduct 80 and return 70 principal quota (the 10-unit bonus stays).
	result, err := RefundAdtyTopUp("RF-detached", "chargeback", 1, "detached-refund-1", q("70"))
	require.NoError(t, err)
	require.Equal(t, q("80"), result.DeductedQuota)
	require.Equal(t, q("70"), result.RefundQuota)
	// The queued spend is still unflushed, so the row keeps that 30 until the
	// next batch flush. The refund itself has already removed the live 80.
	require.Equal(t, int(q("30")), userQuota(t, db, user.Id))
}
