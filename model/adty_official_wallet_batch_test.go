package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func sumAdtyFundRemaining(t *testing.T, db *gorm.DB, userID int) (principal, bonus, total int64) {
	t.Helper()
	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", userID).Find(&rows).Error)
	for _, row := range rows {
		switch row.Kind {
		case "principal":
			principal += row.Remaining
		case "bonus":
			bonus += row.Remaining
		}
		total += row.Remaining
	}
	return principal, bonus, total
}

func adtyFundRemainingBySource(t *testing.T, db *gorm.DB, userID int, source string) (principal, bonus int64) {
	t.Helper()
	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND source = ?", userID, source).Find(&rows).Error)
	for _, row := range rows {
		switch row.Kind {
		case "principal":
			principal += row.Remaining
		case "bonus":
			bonus += row.Remaining
		}
	}
	return principal, bonus
}

func requireWalletMatchesBatches(t *testing.T, db *gorm.DB, userID int) {
	t.Helper()
	_, _, remaining := sumAdtyFundRemaining(t, db, userID)
	require.EqualValues(t, userQuota(t, db, userID), remaining)
}

func seedHostedPendingTopUp(t *testing.T, userID int, tradeNo, provider string, amount int64, money float64) {
	t.Helper()
	require.NoError(t, DB.Create(&TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   provider,
		PaymentProvider: provider,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}).Error)
}

func TestOfficialHostedCreditsWritePrincipalBatches(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	quota := int64(2 * int(common.QuotaPerUnit))

	cases := []struct {
		name, provider, tradeNo string
		amount                  int64
		money                   float64
		credit                  func(*testing.T, string)
	}{
		{
			name: "epay", provider: PaymentProviderEpay, tradeNo: "EPAY-BATCH-1", amount: 2, money: 10,
			credit: func(t *testing.T, tradeNo string) {
				done, err := RechargeEpay(tradeNo, "alipay", decimal.NewFromInt(10), "127.0.0.1")
				require.NoError(t, err)
				require.False(t, done)
			},
		},
		{
			name: "stripe", provider: PaymentProviderStripe, tradeNo: "STRIPE-BATCH-1", amount: 2, money: 10,
			credit: func(t *testing.T, tradeNo string) {
				require.NoError(t, Recharge(tradeNo, "cus_test", "127.0.0.1"))
			},
		},
		{
			name: "creem", provider: PaymentProviderCreem, tradeNo: "CREEM-BATCH-1", amount: quota, money: 10,
			credit: func(t *testing.T, tradeNo string) {
				require.NoError(t, RechargeCreem(tradeNo, "", "", "127.0.0.1"))
			},
		},
		{
			name: "waffo", provider: PaymentProviderWaffo, tradeNo: "WAFFO-BATCH-1", amount: 2, money: 10,
			credit: func(t *testing.T, tradeNo string) {
				require.NoError(t, RechargeWaffo(tradeNo, "127.0.0.1"))
			},
		},
		{
			name: "waffo-pancake", provider: PaymentProviderWaffoPancake, tradeNo: "PAN-BATCH-1", amount: 2, money: 10,
			credit: func(t *testing.T, tradeNo string) {
				require.NoError(t, RechargeWaffoPancake(tradeNo))
			},
		},
		{
			name: "manual-epay", provider: PaymentProviderEpay, tradeNo: "MANUAL-BATCH-1", amount: 2, money: 10,
			credit: func(t *testing.T, tradeNo string) {
				require.NoError(t, ManualCompleteTopUp(tradeNo, "127.0.0.1"))
			},
		},
		{
			name: "manual-creem", provider: PaymentProviderCreem, tradeNo: "MANUAL-CREEM-BATCH-1", amount: quota, money: 10,
			credit: func(t *testing.T, tradeNo string) {
				require.NoError(t, ManualCompleteTopUp(tradeNo, "127.0.0.1"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := seedAdtyTopUpUser(t, db, "credit-"+tc.name, 0)
			seedHostedPendingTopUp(t, user.Id, tc.tradeNo, tc.provider, tc.amount, tc.money)
			tc.credit(t, tc.tradeNo)

			principal, bonus := adtyFundRemainingBySource(t, db, user.Id, tc.tradeNo)
			require.EqualValues(t, quota, principal)
			require.EqualValues(t, 0, bonus)
			require.EqualValues(t, quota, userQuota(t, db, user.Id))
			requireWalletMatchesBatches(t, db, user.Id)

			var lotCount int64
			require.NoError(t, db.Model(&AdtyFundBatch{}).Where("user_id = ? AND source = ?", user.Id, tc.tradeNo).Count(&lotCount).Error)
			require.EqualValues(t, 1, lotCount)
		})
	}
}

func TestOfficialHostedCreditReplayDoesNotDuplicateBatches(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "epay-replay", 0)
	seedHostedPendingTopUp(t, user.Id, "EPAY-REPLAY-BATCH", PaymentProviderEpay, 2, 10)
	done, err := RechargeEpay("EPAY-REPLAY-BATCH", "alipay", decimal.NewFromInt(10), "127.0.0.1")
	require.NoError(t, err)
	require.False(t, done)
	done, err = RechargeEpay("EPAY-REPLAY-BATCH", "alipay", decimal.NewFromInt(10), "127.0.0.1")
	require.NoError(t, err)
	require.True(t, done)
	principal, bonus := adtyFundRemainingBySource(t, db, user.Id, "EPAY-REPLAY-BATCH")
	require.EqualValues(t, 2*int(common.QuotaPerUnit), principal)
	require.EqualValues(t, 0, bonus)
	requireWalletMatchesBatches(t, db, user.Id)
}

func TestRefundWaffoConsumesOrderBatches(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "waffo-refund-batch", 0)
	seedHostedPendingTopUp(t, user.Id, "WAFFO-REFUND-BATCH", PaymentProviderWaffo, 2, 10)
	require.NoError(t, RechargeWaffo("WAFFO-REFUND-BATCH", "127.0.0.1"))
	credited := int64(2 * int(common.QuotaPerUnit))
	require.NoError(t, RefundWaffo("WAFFO-REFUND-BATCH", "waffo-refund-1", false, "5"))
	principal, bonus := adtyFundRemainingBySource(t, db, user.Id, "WAFFO-REFUND-BATCH")
	require.EqualValues(t, credited/2, principal)
	require.EqualValues(t, 0, bonus)
	require.EqualValues(t, credited/2, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)

	require.NoError(t, RefundWaffo("WAFFO-REFUND-BATCH", "waffo-refund-1", false, "5"))
	principal, _ = adtyFundRemainingBySource(t, db, user.Id, "WAFFO-REFUND-BATCH")
	require.EqualValues(t, credited/2, principal)
	requireWalletMatchesBatches(t, db, user.Id)

	require.NoError(t, RefundWaffo("WAFFO-REFUND-BATCH", "waffo-refund-2", true, ""))
	principal, _ = adtyFundRemainingBySource(t, db, user.Id, "WAFFO-REFUND-BATCH")
	require.EqualValues(t, 0, principal)
	require.EqualValues(t, 0, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)
}

func TestRefundWaffoPancakeConsumesOrderBatches(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "pancake-refund-batch", 0)
	seedHostedPendingTopUp(t, user.Id, "PAN-REFUND-BATCH", PaymentProviderWaffoPancake, 2, 10)
	require.NoError(t, RechargeWaffoPancake("PAN-REFUND-BATCH"))
	require.NoError(t, RefundWaffoPancake("PAN-REFUND-BATCH"))
	principal, bonus := adtyFundRemainingBySource(t, db, user.Id, "PAN-REFUND-BATCH")
	require.EqualValues(t, 0, principal)
	require.EqualValues(t, 0, bonus)
	require.EqualValues(t, 0, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)
}

func TestManualRefundUpdatesOrderPrincipalAndGiftBatches(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "manual-refund-batch", 0)
	seedAdtyOrder(t, user.Id, "EPU-MANUAL-BATCH", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := CompleteAdtyTopUp("EPU-MANUAL-BATCH", usdtFacts("EPU-MANUAL-BATCH", "50.01"), "")
	require.NoError(t, err)
	requireWalletMatchesBatches(t, db, user.Id)

	result, err := completeManualRefundForTest(t, "EPU-MANUAL-BATCH", "chargeback", 1, "manual-batch-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	require.EqualValues(t, 20_000_000, result.Refund.RequestQuota)
	require.EqualValues(t, 500_000, result.Refund.BonusDeductedQuota)
	require.EqualValues(t, 20_000_000, result.Refund.DeductedPrincipalQuota)
	require.Zero(t, result.Refund.ShortfallQuota)

	principal, bonus := adtyFundRemainingBySource(t, db, user.Id, "EPU-MANUAL-BATCH")
	require.EqualValues(t, 30_000_000, principal)
	require.EqualValues(t, 750_000, bonus)
	require.EqualValues(t, 30_750_000, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)

	again, err := completeManualRefundForTest(t, "EPU-MANUAL-BATCH", "chargeback", 1, "manual-batch-1", decimal.RequireFromString("40"))
	require.NoError(t, err)
	require.Equal(t, result.Refund.Id, again.Refund.Id)
	require.Equal(t, result.Ledger.Id, again.Ledger.Id)
	principal, bonus = adtyFundRemainingBySource(t, db, user.Id, "EPU-MANUAL-BATCH")
	require.EqualValues(t, 30_000_000, principal)
	require.EqualValues(t, 750_000, bonus)
	requireWalletMatchesBatches(t, db, user.Id)
}

func TestRefundAdtyTopUpReclaimsOrderBatches(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "auto-refund-batch", 0)
	seedAdtyOrder(t, user.Id, "EPU-AUTO-BATCH", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := CompleteAdtyTopUp("EPU-AUTO-BATCH", usdtFacts("EPU-AUTO-BATCH", "50.01"), "")
	require.NoError(t, err)

	result, err := refundAdtyTopUp(t, "EPU-AUTO-BATCH", "chargeback", 1, "auto-batch-1")
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, result.RefundQuota)
	require.EqualValues(t, 51_250_000, result.DeductedQuota)

	principal, bonus := adtyFundRemainingBySource(t, db, user.Id, "EPU-AUTO-BATCH")
	require.EqualValues(t, 0, principal)
	require.EqualValues(t, 0, bonus)
	require.EqualValues(t, 0, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)
}

func TestAdjustUserQuotaWithLedgerMaintainsFundBatches(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "adjust-batch", 0)

	added, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "add", 1_000_000, "grant", "adjust-add-1")
	require.NoError(t, err)
	require.False(t, added.Replayed)
	principal, bonus := adtyFundRemainingBySource(t, db, user.Id, adtyAdjustFundSource("adjust-add-1"))
	require.EqualValues(t, 1_000_000, principal)
	require.EqualValues(t, 0, bonus)
	require.EqualValues(t, 1_000_000, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)

	replay, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "add", 1_000_000, "grant", "adjust-add-1")
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	var lotCount int64
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("user_id = ? AND source = ?", user.Id, adtyAdjustFundSource("adjust-add-1")).Count(&lotCount).Error)
	require.EqualValues(t, 1, lotCount)
	requireWalletMatchesBatches(t, db, user.Id)

	_, err = AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 400_000, "trim", "adjust-sub-1")
	require.NoError(t, err)
	_, _, remaining := sumAdtyFundRemaining(t, db, user.Id)
	require.EqualValues(t, 600_000, remaining)
	require.EqualValues(t, 600_000, userQuota(t, db, user.Id))

	_, err = AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "override", 0, "clear", "adjust-over-1")
	require.NoError(t, err)
	require.EqualValues(t, 0, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)
}
