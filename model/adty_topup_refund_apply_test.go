package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRefundAdtyTopUpAppliesWithoutCompleting(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "apply-quota", 0)
	seedCreditedOrder(t, db, user.Id, "RF-apply-quota", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	before := userQuota(t, db, user.Id)

	applied, err := RefundAdtyTopUp("RF-apply-quota", "customer request", 1, "apply-quota-1", 25_000_000)
	require.NoError(t, err)
	require.Equal(t, AdtyManualRefundStatusSubmitted, applied.Refund.Status)
	require.EqualValues(t, 25_000_000, applied.Refund.RequestQuota)
	require.EqualValues(t, 25_000_000, applied.Occupancy.InProgressQuota)
	require.Zero(t, applied.Occupancy.CompletedQuota)
	require.Equal(t, before, userQuota(t, db, user.Id), "apply occupies H but does not debit")
	require.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "RF-apply-quota"))

	ext, err := GetAdtyTopupOrder("RF-apply-quota")
	require.NoError(t, err)
	require.Zero(t, ext.RefundQuota, "completed S is not written at apply")

	var ledgers int64
	require.NoError(t, db.Model(&AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&ledgers).Error)
	require.Zero(t, ledgers, "after-sales hooks and wallet ledgers wait for later steps")

	again, err := RefundAdtyTopUp("RF-apply-quota", "customer request", 1, "apply-quota-1", 25_000_000)
	require.NoError(t, err)
	require.Equal(t, applied.Refund.RefundId, again.Refund.RefundId)
	require.Equal(t, AdtyManualRefundStatusSubmitted, again.Refund.Status)
	require.Zero(t, again.Occupancy.CompletedQuota)
	require.Equal(t, before, userQuota(t, db, user.Id))

	_, err = RefundAdtyTopUp("RF-apply-quota", "customer request", 1, "apply-quota-over", 30_000_000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "remaining principal")

	second, err := RefundAdtyTopUp("RF-apply-quota", "customer request", 1, "apply-quota-2", 25_000_000)
	require.NoError(t, err)
	require.Equal(t, AdtyManualRefundStatusSubmitted, second.Refund.Status)
	require.EqualValues(t, 50_000_000, second.Occupancy.InProgressQuota)
	require.Zero(t, second.Occupancy.CompletedQuota)
	require.Zero(t, second.Occupancy.RemainingQuota)
	require.Equal(t, before, userQuota(t, db, user.Id))
}
