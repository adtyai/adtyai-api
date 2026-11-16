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
	require.EqualValues(t, 50_000_000, result.Occupancy.CompletedQuota)
	require.EqualValues(t, 51_250_000, (result.Refund.DeductedPrincipalQuota + result.Refund.BonusDeductedQuota))

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

func TestAdjustUserQuotaWithLedgerDoesNotFirePrincipalSpentHook(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "adjust-quiet-hook", 0)

	_, err := AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "add", 1_000_000, "grant", "adjust-quiet-add")
	require.NoError(t, err)

	prev := AdtyAfterPrincipalSpentTx
	called := 0
	AdtyAfterPrincipalSpentTx = func(tx *gorm.DB, userID int, operationKey string, principalQuota int64) error {
		called++
		t.Errorf("manual adjust fired principal-spent hook user=%d quota=%d", userID, principalQuota)
		return nil
	}
	t.Cleanup(func() { AdtyAfterPrincipalSpentTx = prev })

	_, err = AdjustUserQuotaWithLedger(user.Id, 1, common.RoleRootUser, "subtract", 400_000, "trim", "adjust-quiet-sub")
	require.NoError(t, err)
	require.Equal(t, 0, called)
	_, _, remaining := sumAdtyFundRemaining(t, db, user.Id)
	require.EqualValues(t, 600_000, remaining)
	require.EqualValues(t, 600_000, userQuota(t, db, user.Id))
	requireWalletMatchesBatches(t, db, user.Id)
}
