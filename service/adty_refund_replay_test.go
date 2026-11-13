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
	first, err := model.RefundAdtyTopUpManualUSD("EPU-replay", "duplicate", 1, "refund-replay-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	second, err := model.RefundAdtyTopUpManualUSD("EPU-replay", "duplicate", 1, "refund-replay-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	require.Equal(t, first.DeductedQuota, second.DeductedQuota)
	require.Equal(t, first.RefundQuota, second.RefundQuota)
	var user model.User
	require.NoError(t, db.First(&user, customer.Id).Error)
	require.EqualValues(t, 50_000_000-first.DeductedQuota, user.Quota)
	require.NoError(t, model.RecordAdtyRefundProof("EPU-replay", "bank-slip-88"))
	var order model.AdtyTopupOrder
	require.NoError(t, db.Where("trade_no = ?", "EPU-replay").First(&order).Error)
	require.Equal(t, "bank-slip-88", order.RefundProof)
}
