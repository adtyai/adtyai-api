package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestManualRefundReplayDoesNotDebitTwice(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "refund-replay")
	customer := siteCustomer(t, db, site, "refund-replay-user")
	siteTopUp(t, customer, site, "EPU-replay", 100, "0")
	first, err := model.ApplyAdtyTopUpManualRefund("EPU-replay", "duplicate", 1, "refund-replay-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	second, err := model.ApplyAdtyTopUpManualRefund("EPU-replay", "duplicate", 1, "refund-replay-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	require.Equal(t, first.Refund.Id, second.Refund.Id)
	require.Equal(t, first.Refund.RequestQuota, second.Refund.RequestQuota)
	var user model.User
	require.NoError(t, db.First(&user, customer.Id).Error)
	require.EqualValues(t, 50_000_000, user.Quota, "apply occupies H but does not debit")
	_, err = model.ApproveAdtyTopUpManualRefund("EPU-replay", "refund-replay-1", 1, "", "")
	require.NoError(t, err)
	reclaim, err := model.ReclaimAdtyTopUpManualRefund("EPU-replay", "refund-replay-1", 1, "")
	require.NoError(t, err)
	again, err := model.ReclaimAdtyTopUpManualRefund("EPU-replay", "refund-replay-1", 1, "")
	require.NoError(t, err)
	require.Equal(t, reclaim.Ledger.Id, again.Ledger.Id)
	require.NoError(t, db.First(&user, customer.Id).Error)
	require.EqualValues(t, 50_000_000-reclaim.Refund.DeductedPrincipalQuota, user.Quota)
	_, err = model.RecordAdtyTopUpManualRefundPayout("EPU-replay", "refund-replay-1", 1, "", model.AdtyManualRefundPayout{
		Status: model.AdtyManualRefundPayoutSuccess, Currency: "USDT", Amount: decimal.RequireFromString("40"), PaidAt: 20, Voucher: "bank-slip-88",
	})
	require.NoError(t, err)
	var order model.AdtyTopupOrder
	require.NoError(t, db.Where("trade_no = ?", "EPU-replay").First(&order).Error)
	require.Equal(t, "bank-slip-88", order.RefundProof)
	require.Zero(t, order.RefundQuota, "payout success does not complete S")
}
