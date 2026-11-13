package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func completeManualRefundForTest(t *testing.T, tradeNo, reason string, operator int, refundID string, requestUSD decimal.Decimal) (*AdtyManualRefundResult, error) {
	t.Helper()
	if _, err := ApplyAdtyTopUpManualRefund(tradeNo, reason, operator, refundID, requestUSD); err != nil {
		return nil, err
	}
	if _, err := ApproveAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":approve", ""); err != nil {
		return nil, err
	}
	if _, err := ReclaimAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":reclaim"); err != nil {
		return nil, err
	}
	if _, err := PlanAdtyTopUpManualRefundBonus(tradeNo, refundID, operator, refundID+":bonus"); err != nil {
		return nil, err
	}
	if _, err := PlanAdtyTopUpManualRefundCommission(tradeNo, refundID, operator, refundID+":commission"); err != nil {
		return nil, err
	}
	if _, err := RecordAdtyTopUpManualRefundPayout(tradeNo, refundID, operator, refundID+":payout", AdtyManualRefundPayout{
		Status:   AdtyManualRefundPayoutSuccess,
		Currency: "USDT",
		Amount:   requestUSD,
		PaidAt:   1,
		Voucher:  "voucher-" + refundID,
	}); err != nil {
		return nil, err
	}
	return CompleteAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":complete")
}

func seedManualRefundOrder(t *testing.T, name, tradeNo string) *User {
	t.Helper()
	user := seedAdtyTopUpUser(t, DB, name, 0)
	seedAdtyOrder(t, user.Id, tradeNo, PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := CompleteAdtyTopUp(tradeNo, usdtFacts(tradeNo, "50.01"), "")
	require.NoError(t, err)
	return user
}

func TestManualRefundCapUsesCompletedInProgressAndUnknown(t *testing.T) {
	prepareAdtyTopUpDB(t)
	user := seedManualRefundOrder(t, "cap-shu", "EPU-CAP-SHU")
	_ = user

	first, err := ApplyAdtyTopUpManualRefund("EPU-CAP-SHU", "partial", 1, "refund-h-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	require.EqualValues(t, 20_000_000, first.Occupancy.InProgressQuota)
	require.Zero(t, first.Occupancy.CompletedQuota)
	require.EqualValues(t, 30_000_000, first.Occupancy.RemainingQuota)

	second, err := ApplyAdtyTopUpManualRefund("EPU-CAP-SHU", "partial", 1, "refund-u-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	require.EqualValues(t, 40_000_000, second.Occupancy.InProgressQuota)
	require.EqualValues(t, 10_000_000, second.Occupancy.RemainingQuota)

	_, err = ApplyAdtyTopUpManualRefund("EPU-CAP-SHU", "partial", 1, "refund-over", decimal.RequireFromString("30"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "remaining principal")

	_, err = ApproveAdtyTopUpManualRefund("EPU-CAP-SHU", "refund-u-1", 1, "", "")
	require.NoError(t, err)
	unknown, err := RecordAdtyTopUpManualRefundPayout("EPU-CAP-SHU", "refund-u-1", 1, "", AdtyManualRefundPayout{
		Status: AdtyManualRefundPayoutUnknown, Currency: "USDT", Amount: decimal.RequireFromString("40"), PaidAt: 1, Voucher: "pending-wire",
	})
	require.NoError(t, err)
	require.EqualValues(t, 20_000_000, unknown.Occupancy.InProgressQuota)
	require.EqualValues(t, 20_000_000, unknown.Occupancy.UnknownQuota)
	require.EqualValues(t, 10_000_000, unknown.Occupancy.RemainingQuota)

	exact, err := ApplyAdtyTopUpManualRefund("EPU-CAP-SHU", "partial", 1, "refund-exact", decimal.RequireFromString("20"))
	require.NoError(t, err)
	require.Zero(t, exact.Occupancy.RemainingQuota)
	_, err = ApplyAdtyTopUpManualRefund("EPU-CAP-SHU", "partial", 1, "refund-over-2", decimal.RequireFromString("0.01"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "remaining principal")
}

func TestManualRefundAllowsAmountAboveSpendableWithoutNegativeBalance(t *testing.T) {
	prepareAdtyTopUpDB(t)
	user := seedManualRefundOrder(t, "shortfall-user", "EPU-SHORTFALL")
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 5_000_000).Error)
	require.NoError(t, DB.Model(&AdtyFundBatch{}).Where("user_id = ? AND source = ? AND kind = ?", user.Id, "EPU-SHORTFALL", "principal").Update("remaining", 5_000_000).Error)
	require.NoError(t, DB.Model(&AdtyFundBatch{}).Where("user_id = ? AND source = ? AND kind = ?", user.Id, "EPU-SHORTFALL", "bonus").Update("remaining", 0).Error)

	applied, err := ApplyAdtyTopUpManualRefund("EPU-SHORTFALL", "chargeback", 1, "refund-short-1", decimal.RequireFromString("100"))
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, applied.Refund.RequestQuota)
	require.EqualValues(t, 50_000_000, applied.Occupancy.InProgressQuota)
	require.Zero(t, applied.Occupancy.CompletedQuota)

	_, err = ApproveAdtyTopUpManualRefund("EPU-SHORTFALL", "refund-short-1", 1, "", "")
	require.NoError(t, err)
	reclaimed, err := ReclaimAdtyTopUpManualRefund("EPU-SHORTFALL", "refund-short-1", 1, "")
	require.NoError(t, err)
	require.EqualValues(t, 5_000_000, reclaimed.Refund.DeductedPrincipalQuota)
	require.EqualValues(t, 45_000_000, reclaimed.Refund.ShortfallQuota)
	require.True(t, reclaimed.Refund.ShortfallUSD.Equal(decimal.RequireFromString("90")))
	require.EqualValues(t, 50_000_000, reclaimed.Refund.RequestQuota, "authorized r is not shrunk by the shortfall")
	require.EqualValues(t, 0, userQuota(t, DB, user.Id))
	require.GreaterOrEqual(t, userQuota(t, DB, user.Id), 0)
}

func TestManualRefundInternalReclaimAndPayoutAreIndependent(t *testing.T) {
	prepareAdtyTopUpDB(t)
	seedManualRefundOrder(t, "independent-user", "EPU-INDEP")
	_, err := ApplyAdtyTopUpManualRefund("EPU-INDEP", "chargeback", 1, "refund-indep-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	_, err = ApproveAdtyTopUpManualRefund("EPU-INDEP", "refund-indep-1", 1, "", "")
	require.NoError(t, err)

	reclaimed, err := ReclaimAdtyTopUpManualRefund("EPU-INDEP", "refund-indep-1", 1, "")
	require.NoError(t, err)
	require.Greater(t, reclaimed.Refund.ReclaimedAt, int64(0))
	require.Zero(t, reclaimed.Refund.PayoutRecordedAt)
	require.NotEqual(t, AdtyManualRefundStatusCompleted, reclaimed.Refund.Status)

	_, err = CompleteAdtyTopUpManualRefund("EPU-INDEP", "refund-indep-1", 1, "")
	require.Error(t, err)

	paid, err := RecordAdtyTopUpManualRefundPayout("EPU-INDEP", "refund-indep-1", 1, "", AdtyManualRefundPayout{
		Status: AdtyManualRefundPayoutSuccess, Currency: "USDT", Amount: decimal.RequireFromString("40"), PaidAt: 11, Voucher: "wire-1",
	})
	require.NoError(t, err)
	require.Equal(t, AdtyManualRefundPayoutSuccess, paid.Refund.PayoutStatus)
	require.NotEqual(t, AdtyManualRefundStatusCompleted, paid.Refund.Status)
	require.EqualValues(t, 20_000_000, paid.Occupancy.InProgressQuota)
	require.Zero(t, paid.Occupancy.CompletedQuota)

	other, err := ApplyAdtyTopUpManualRefund("EPU-INDEP-2", "other", 1, "refund-indep-payout-only", decimal.RequireFromString("10"))
	_ = other
	_ = err

	seedManualRefundOrder(t, "payout-first-user", "EPU-PAYFIRST")
	_, err = ApplyAdtyTopUpManualRefund("EPU-PAYFIRST", "chargeback", 1, "refund-pay-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	_, err = ApproveAdtyTopUpManualRefund("EPU-PAYFIRST", "refund-pay-1", 1, "", "")
	require.NoError(t, err)
	payoutOnly, err := RecordAdtyTopUpManualRefundPayout("EPU-PAYFIRST", "refund-pay-1", 1, "", AdtyManualRefundPayout{
		Status: AdtyManualRefundPayoutSuccess, Currency: "USDT", Amount: decimal.RequireFromString("40"), PaidAt: 12, Voucher: "wire-2",
	})
	require.NoError(t, err)
	require.Zero(t, payoutOnly.Refund.ReclaimedAt)
	require.Greater(t, payoutOnly.Refund.PayoutRecordedAt, int64(0))
	_, err = CompleteAdtyTopUpManualRefund("EPU-PAYFIRST", "refund-pay-1", 1, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Internal principal reclaim")
}

func TestManualRefundCompletesOnlyAfterRequiredSubsteps(t *testing.T) {
	prepareAdtyTopUpDB(t)
	user := seedManualRefundOrder(t, "complete-user", "EPU-COMPLETE")
	_, err := ApplyAdtyTopUpManualRefund("EPU-COMPLETE", "chargeback", 1, "refund-complete-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	_, err = CompleteAdtyTopUpManualRefund("EPU-COMPLETE", "refund-complete-1", 1, "")
	require.Error(t, err)

	_, err = ApproveAdtyTopUpManualRefund("EPU-COMPLETE", "refund-complete-1", 1, "", "")
	require.NoError(t, err)
	_, err = ReclaimAdtyTopUpManualRefund("EPU-COMPLETE", "refund-complete-1", 1, "")
	require.NoError(t, err)
	_, err = PlanAdtyTopUpManualRefundBonus("EPU-COMPLETE", "refund-complete-1", 1, "")
	require.NoError(t, err)
	_, err = PlanAdtyTopUpManualRefundCommission("EPU-COMPLETE", "refund-complete-1", 1, "")
	require.NoError(t, err)
	_, err = CompleteAdtyTopUpManualRefund("EPU-COMPLETE", "refund-complete-1", 1, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Offline payout")

	_, err = RecordAdtyTopUpManualRefundPayout("EPU-COMPLETE", "refund-complete-1", 1, "", AdtyManualRefundPayout{
		Status: AdtyManualRefundPayoutSuccess, Currency: "USDT", Amount: decimal.RequireFromString("40"), PaidAt: 13, Voucher: "wire-complete",
	})
	require.NoError(t, err)
	done, err := CompleteAdtyTopUpManualRefund("EPU-COMPLETE", "refund-complete-1", 1, "")
	require.NoError(t, err)
	require.Equal(t, AdtyManualRefundStatusCompleted, done.Refund.Status)
	require.EqualValues(t, 20_000_000, done.Occupancy.CompletedQuota)
	require.Zero(t, done.Occupancy.InProgressQuota)
	require.EqualValues(t, 30_750_000, userQuota(t, DB, user.Id))
}

func TestManualRefundSameRefundIDIsIdempotent(t *testing.T) {
	prepareAdtyTopUpDB(t)
	user := seedManualRefundOrder(t, "idem-user", "EPU-IDEM")
	first, err := ApplyAdtyTopUpManualRefund("EPU-IDEM", "chargeback", 1, "refund-idem-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	second, err := ApplyAdtyTopUpManualRefund("EPU-IDEM", "different-reason", 1, "refund-idem-1", decimal.RequireFromString("50"))
	require.NoError(t, err)
	require.Equal(t, first.Refund.Id, second.Refund.Id)
	require.True(t, first.Refund.RequestUSD.Equal(decimal.RequireFromString("40")))
	require.EqualValues(t, 20_000_000, second.Occupancy.InProgressQuota)

	_, err = ApproveAdtyTopUpManualRefund("EPU-IDEM", "refund-idem-1", 1, "", "")
	require.NoError(t, err)
	reclaim1, err := ReclaimAdtyTopUpManualRefund("EPU-IDEM", "refund-idem-1", 1, "")
	require.NoError(t, err)
	reclaim2, err := ReclaimAdtyTopUpManualRefund("EPU-IDEM", "refund-idem-1", 1, "")
	require.NoError(t, err)
	require.Equal(t, reclaim1.Ledger.Id, reclaim2.Ledger.Id)
	require.EqualValues(t, 51_250_000-reclaim1.Refund.DeductedPrincipalQuota, userQuota(t, DB, user.Id))

	_, err = PlanAdtyTopUpManualRefundBonus("EPU-IDEM", "refund-idem-1", 1, "")
	require.NoError(t, err)
	_, err = PlanAdtyTopUpManualRefundCommission("EPU-IDEM", "refund-idem-1", 1, "")
	require.NoError(t, err)
	_, err = RecordAdtyTopUpManualRefundPayout("EPU-IDEM", "refund-idem-1", 1, "", AdtyManualRefundPayout{
		Status: AdtyManualRefundPayoutSuccess, Currency: "USDT", Amount: decimal.RequireFromString("40"), PaidAt: 14, Voucher: "wire-idem",
	})
	require.NoError(t, err)
	done1, err := CompleteAdtyTopUpManualRefund("EPU-IDEM", "refund-idem-1", 1, "")
	require.NoError(t, err)
	done2, err := CompleteAdtyTopUpManualRefund("EPU-IDEM", "refund-idem-1", 1, "")
	require.NoError(t, err)
	require.Equal(t, done1.Refund.CompletedAt, done2.Refund.CompletedAt)
	require.Equal(t, done1.Ledger.Id, done2.Ledger.Id)
	require.EqualValues(t, 20_000_000, done2.Occupancy.CompletedQuota)
}

func TestManualRefundRatiosUseCompletedSOnlyAndIgnoreInProgress(t *testing.T) {
	prepareAdtyTopUpDB(t)
	seedManualRefundOrder(t, "ratio2-user", "EPU-RATIO2")
	_, err := ApplyAdtyTopUpManualRefund("EPU-RATIO2", "hold", 1, "refund-ratio2-h", decimal.RequireFromString("40"))
	require.NoError(t, err)

	done, err := completeManualRefundForTest(t, "EPU-RATIO2", "done", 1, "refund-ratio2-s", decimal.RequireFromString("20"))
	require.NoError(t, err)
	require.EqualValues(t, 10_000_000, done.Occupancy.CompletedQuota)
	require.EqualValues(t, 20_000_000, done.Occupancy.InProgressQuota)

	var ext AdtyTopupOrder
	require.NoError(t, DB.Where("trade_no = ?", "EPU-RATIO2").First(&ext).Error)
	require.EqualValues(t, 10_000_000, ext.RefundQuota)
	require.True(t, ext.RefundRatio().Equal(decimal.NewFromInt(10_000_000).Div(decimal.NewFromInt(ext.FaceQuota()))))
	reclaim := CumulativeGiftReclaim(ext.BonusUSD, ext.FaceUSD, decimal.Zero, decimal.RequireFromString("20"))
	require.True(t, reclaim.Equal(decimal.RequireFromString("0.5")))
	facts, err := LoadAdtyRefundPageFacts("EPU-RATIO2", decimal.Zero)
	require.NoError(t, err)
	require.EqualValues(t, 10_000_000, facts.RefundCompletedQuota)
	require.EqualValues(t, 20_000_000, facts.RefundInProgressQuota)
	require.EqualValues(t, 30_000_000, facts.RefundOccupiedQuota)
}

func TestManualRefundHistoricalCompletedCountsInS(t *testing.T) {
	prepareAdtyTopUpDB(t)
	user := seedManualRefundOrder(t, "hist-user", "EPU-HIST")
	require.NoError(t, DB.Model(&AdtyTopupOrder{}).Where("trade_no = ?", "EPU-HIST").Updates(map[string]any{
		"refund_quota":  25_000_000,
		"refund_amount": decimal.NewFromInt(50),
		"refunded_at":   99,
		"refund_reason": "historical one-shot",
	}).Error)
	var ext AdtyTopupOrder
	require.NoError(t, DB.Where("trade_no = ?", "EPU-HIST").First(&ext).Error)
	require.EqualValues(t, 25_000_000, ext.RefundQuota)

	facts, err := LoadAdtyRefundPageFacts("EPU-HIST", decimal.Zero)
	require.NoError(t, err)
	require.EqualValues(t, 25_000_000, facts.RefundCompletedQuota)
	require.EqualValues(t, 25_000_000, facts.RefundOccupiedQuota)
	require.EqualValues(t, 25_000_000, facts.RefundRemainingQuota)
	require.Empty(t, facts.ManualRefunds)

	_, err = ApplyAdtyTopUpManualRefund("EPU-HIST", "second", 1, "refund-hist-2", decimal.RequireFromString("60"))
	require.Error(t, err)
	applied, err := ApplyAdtyTopUpManualRefund("EPU-HIST", "second", 1, "refund-hist-2", decimal.RequireFromString("50"))
	require.NoError(t, err)
	require.EqualValues(t, 25_000_000, applied.Occupancy.CompletedQuota)
	require.EqualValues(t, 25_000_000, applied.Occupancy.InProgressQuota)
	require.Zero(t, applied.Occupancy.RemainingQuota)
	require.EqualValues(t, 25_000_000, applied.Refund.RequestQuota)

	var stored AdtyTopupOrder
	require.NoError(t, DB.Where("trade_no = ?", "EPU-HIST").First(&stored).Error)
	require.EqualValues(t, 25_000_000, stored.RefundQuota, "historical completed S is not rewritten at apply")
	_ = user
}

func TestManualRefundTableMissingFailsClosed(t *testing.T) {
	prepareAdtyTopUpDB(t)
	seedManualRefundOrder(t, "missing-user", "EPU-MISSING")
	require.NoError(t, DB.Migrator().DropTable(&AdtyManualRefund{}))
	_, err := ApplyAdtyTopUpManualRefund("EPU-MISSING", "chargeback", 1, "refund-missing-1", decimal.RequireFromString("10"))
	require.ErrorIs(t, err, ErrAdtyManualRefundTableMissing)
	_, err = LoadAdtyRefundPageFacts("EPU-MISSING", decimal.Zero)
	require.ErrorIs(t, err, ErrAdtyManualRefundTableMissing)
}
