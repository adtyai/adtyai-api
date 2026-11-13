package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func adtySiteTierUSD(amount string) decimal.Decimal {
	return decimal.RequireFromString(amount)
}

func adtySiteTierQuota(usd string) int64 {
	quota, err := common.WalletQuotaFromDecimalStrict(decimal.RequireFromString(usd).Mul(common.QuotaPerUnitDecimal()))
	if err != nil {
		panic(err)
	}
	return int64(quota)
}

func insertAdtySiteTierTopUp(db *gorm.DB, siteID, userID int, trade, provider string, faceUSD decimal.Decimal, teamID int64, status string) error {
	return insertAdtySiteTierTopUpExtra(db, siteID, userID, trade, provider, "", faceUSD, decimal.Zero, 0, teamID, status)
}

func insertAdtySiteTierTopUpExtra(db *gorm.DB, siteID, userID int, trade, provider, method string, faceUSD, bonusUSD decimal.Decimal, refundQuota int64, teamID int64, status string) error {
	if method == "" && provider == model.AdtyVoucherProvider {
		method = model.AdtyVoucherPurposeSiteCustomer
	}
	if method == "" {
		method = "usdt.trc20"
	}
	credit := faceUSD.Mul(common.QuotaPerUnitDecimal()).IntPart()
	if !bonusUSD.IsZero() {
		credit += bonusUSD.Mul(common.QuotaPerUnitDecimal()).IntPart()
	}
	if err := db.Create(&model.TopUp{
		UserId: userID, Amount: faceUSD.IntPart(), Money: faceUSD.InexactFloat64(),
		TradeNo: trade, PaymentMethod: method, PaymentProvider: provider, Status: status,
		CreateTime: time.Now().Unix(), CompleteTime: time.Now().Unix(),
	}).Error; err != nil {
		return err
	}
	return db.Create(&model.AdtyTopupOrder{
		TradeNo: trade, UserId: userID, Provider: provider, Method: method,
		FaceUSD: faceUSD, BonusUSD: bonusUSD, CreditQuota: credit,
		PayAmount: faceUSD, PayCurrency: "USD", PaidAmount: faceUSD, PaidCurrency: "USD",
		TopupGroupRatio: decimal.NewFromInt(1), TeamId: teamID, SiteId: siteID,
		RefundQuota: refundQuota,
	}).Error
}

func insertAdtySiteTierBilling(db *gorm.DB, siteID, userID int, key, state, kind string, usd int64) error {
	quota := usd * int64(common.QuotaPerUnit)
	return db.Create(&model.BillingOperation{
		OperationKey: key, UserId: userID, Kind: kind, State: state,
		SiteId: siteID, ActualQuota: int(quota), AppliedQuota: int(quota),
	}).Error
}

func saveFixtureAdtySiteTiers(t *testing.T) (low, mid, high model.AdtySiteTier) {
	t.Helper()
	low = model.AdtySiteTier{
		Code: "eval-low", Name: "Eval Low", MinEffectiveUsers: 1,
		MinPrincipalUSD: adtySiteTierUSD("10"), MinConsumptionUSD: adtySiteTierUSD("5"),
		CostFactor: dec("0.9"), MaxMarkup: dec("1.2"), DepositUSD: adtySiteTierUSD("15"),
		Enabled: true, SortOrder: 1,
	}
	mid = model.AdtySiteTier{
		Code: "eval-mid", Name: "Eval Mid", MinEffectiveUsers: 3,
		MinPrincipalUSD: adtySiteTierUSD("80"), MinConsumptionUSD: adtySiteTierUSD("40"),
		CostFactor: dec("0.8"), MaxMarkup: dec("1.5"), DepositUSD: adtySiteTierUSD("30"),
		Enabled: true, SortOrder: 2,
	}
	high = model.AdtySiteTier{
		Code: "eval-high", Name: "Eval High", MinEffectiveUsers: 5,
		MinPrincipalUSD: adtySiteTierUSD("150"), MinConsumptionUSD: adtySiteTierUSD("90"),
		CostFactor: dec("0.7"), MaxMarkup: dec("2"), DepositUSD: adtySiteTierUSD("50"),
		Enabled: true, SortOrder: 3,
	}
	require.NoError(t, SaveAdtySiteTiers([]model.AdtySiteTier{low, mid, high}))
	return low, mid, high
}

func TestAdtySiteTierEffectiveUsersAreDistinctInGoodStanding(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "tier-users")
	otherOwner, other := openActiveSite(t, db, "tier-users-other")
	_ = otherOwner

	alice := siteCustomer(t, db, site, "tier-user-alice")
	bob := siteCustomer(t, db, site, "tier-user-bob")
	_ = siteCustomer(t, db, site, "tier-user-disabled")
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "tier-user-disabled").Update("status", common.UserStatusDisabled).Error)
	cancelled := siteCustomer(t, db, site, "tier-user-cancelled")
	require.NoError(t, db.Delete(cancelled).Error)
	_ = siteCustomer(t, db, other, "tier-user-other-site")
	staff := referralUser(t, db, "tier-user-staff")
	require.NoError(t, db.Model(staff).Updates(map[string]any{"site_id": site.Id, "staff_site_id": site.Id}).Error)

	// A team relationship must not count the same personal user twice.
	require.NoError(t, db.Create(&model.AdtyTeam{Id: 71012, SiteId: site.Id, LeaderUserId: alice.Id, Name: "tier-team", Status: model.AdtyTeamStatusActive, Discount: decimal.NewFromInt(1)}).Error)
	require.NoError(t, db.Create(&model.AdtyTeamMember{TeamId: 71012, UserId: alice.Id, Role: model.AdtyTeamRoleLeader, Status: model.AdtyTeamMemberActive}).Error)
	require.NoError(t, db.Create(&model.AdtyTeamMember{TeamId: 71012, UserId: bob.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}).Error)

	stats, err := ComputeAdtySiteTierStats(site.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.EffectiveUsers, "only distinct in-good-standing personal users on this site count")
}

func TestAdtySiteTierPrincipalIncludesExternalAndCustomerVouchers(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "tier-principal")
	_, other := openActiveSite(t, db, "tier-principal-other")
	customer := siteCustomer(t, db, site, "tier-prin-cust")
	teammate := siteCustomer(t, db, site, "tier-prin-team")
	outsider := siteCustomer(t, db, other, "tier-prin-out")

	require.NoError(t, insertAdtySiteTierTopUpExtra(db, site.Id, customer.Id, "prin-cash", model.PaymentProviderAdtyBEpusdt, "usdt.trc20",
		adtySiteTierUSD("40"), adtySiteTierUSD("8"), 0, 0, common.TopUpStatusSuccess))
	require.NoError(t, insertAdtySiteTierTopUp(db, site.Id, teammate.Id, "prin-team", model.PaymentProviderAdtyEpay, adtySiteTierUSD("25"), 88, common.TopUpStatusSuccess))
	require.NoError(t, insertAdtySiteTierTopUpExtra(db, site.Id, customer.Id, "prin-voucher", model.AdtyVoucherProvider, model.AdtyVoucherPurposeSiteCustomer,
		adtySiteTierUSD("15"), decimal.Zero, 0, 0, common.TopUpStatusSuccess))
	require.NoError(t, insertAdtySiteTierTopUpExtra(db, site.Id, customer.Id, "prin-refunded", model.PaymentProviderAdtyBEpusdt, "usdt.trc20",
		adtySiteTierUSD("20"), decimal.Zero, adtySiteTierQuota("12"), 0, model.TopUpStatusRefunded))
	require.NoError(t, insertAdtySiteTierTopUpExtra(db, site.Id, site.OwnerUserId, "prin-margin-voucher", model.AdtyVoucherProvider, model.AdtyVoucherPurposeSiteMargin,
		adtySiteTierUSD("50"), decimal.Zero, 0, 0, common.TopUpStatusSuccess))
	require.NoError(t, insertAdtySiteTierTopUpExtra(db, site.Id, site.OwnerUserId, "prin-owner-voucher", model.AdtyVoucherProvider, model.AdtyVoucherPurposeSiteOwner,
		adtySiteTierUSD("30"), decimal.Zero, 0, 0, common.TopUpStatusSuccess))
	require.NoError(t, insertAdtySiteTierTopUp(db, other.Id, outsider.Id, "prin-other-site", model.PaymentProviderAdtyBEpusdt, adtySiteTierUSD("99"), 0, common.TopUpStatusSuccess))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletMargin, adtySiteTierUSD("18"), AdtySiteWalletChange{
		Type: "margin_payment", IdempotencyKey: "prin-margin-topup",
	})
	require.NoError(t, err)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, adtySiteTierUSD("12"), AdtySiteWalletChange{
		Type: "earning_credit", IdempotencyKey: "prin-earning-credit",
	})
	require.NoError(t, err)
	require.NoError(t, TransferAdtyEarningToMargin(site.Id, site.OwnerUserId, adtySiteTierUSD("12"), adtySiteTierUSD("15"), "prin-earn-to-margin"))

	stats, err := ComputeAdtySiteTierStats(site.Id)
	require.NoError(t, err)
	// 40 cash + 25 team + 15 customer voucher + (20-12) retained refunded principal.
	assert.True(t, stats.PrincipalUSD.Equal(adtySiteTierUSD("88")), "got %s", stats.PrincipalUSD)
}

func TestAdtySiteTierConsumptionCountsOnlySettledBilledAmounts(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "tier-consume")
	_, other := openActiveSite(t, db, "tier-consume-other")
	customer := siteCustomer(t, db, site, "tier-cons-cust")
	outsider := siteCustomer(t, db, other, "tier-cons-out")

	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, customer.Id, "cons-settled-request", model.BillingSettled, "request", 7))
	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, customer.Id, "cons-settled-task", model.BillingSettled, "task", 3))
	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, customer.Id, "cons-hold", model.BillingReserved, "request", 11))
	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, customer.Id, "cons-pending", model.BillingSettlePending, "request", 9))
	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, customer.Id, "cons-failed", model.BillingCanceled, "request", 6))
	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, customer.Id, "cons-refunded", model.BillingRefunded, "request", 4))
	require.NoError(t, insertAdtySiteTierBilling(db, other.Id, outsider.Id, "cons-other-site", model.BillingSettled, "request", 20))

	stats, err := ComputeAdtySiteTierStats(site.Id)
	require.NoError(t, err)
	assert.True(t, stats.ConsumptionUSD.Equal(adtySiteTierUSD("10")), "got %s", stats.ConsumptionUSD)
}

func TestAdtySiteTierPromotionDemotionPinAndIdempotency(t *testing.T) {
	db := prepareSiteDB(t)
	saveFixtureAdtySiteTiers(t)
	_, site := openActiveSite(t, db, "tier-eval")

	users := make([]*model.User, 0, 5)
	for _, name := range []string{"eval-u1", "eval-u2", "eval-u3", "eval-u4", "eval-u5"} {
		users = append(users, siteCustomer(t, db, site, name))
	}
	lead := users[0]

	// Two of the mid-grade thresholds are met (users and principal); consumption stays on low.
	require.NoError(t, insertAdtySiteTierTopUp(db, site.Id, lead.Id, "eval-prin-1", model.PaymentProviderAdtyBEpusdt, adtySiteTierUSD("80"), 0, common.TopUpStatusSuccess))
	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, lead.Id, "eval-cons-low", model.BillingSettled, "request", 5))

	stats, err := ComputeAdtySiteTierStats(site.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 5, stats.EffectiveUsers)
	assert.True(t, stats.PrincipalUSD.Equal(adtySiteTierUSD("80")), stats.PrincipalUSD)
	assert.True(t, stats.ConsumptionUSD.Equal(adtySiteTierUSD("5")), stats.ConsumptionUSD)

	first, err := EvaluateAdtySiteTierAt(site.Id, 1_800_000_000, "eval-period-low")
	require.NoError(t, err)
	assert.Equal(t, "eval-low", first.TierCode)
	assert.Equal(t, AdtySiteTierModeAuto, first.TierMode)
	require.NotNil(t, first.TierCostFactor)
	assert.Equal(t, "0.9", first.TierCostFactor.String())
	var margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.RequiredBalance.Equal(adtySiteTierUSD("15")))
	assert.True(t, margin.CycleTierUSD.IsZero(), "grade assignment must not open a margin cycle")

	// Meeting all three mid and high thresholds jumps across mid to high in one evaluation.
	require.NoError(t, insertAdtySiteTierTopUp(db, site.Id, lead.Id, "eval-prin-2", model.PaymentProviderAdtyBEpusdt, adtySiteTierUSD("70"), 0, common.TopUpStatusSuccess))
	require.NoError(t, insertAdtySiteTierBilling(db, site.Id, lead.Id, "eval-cons-high", model.BillingSettled, "task", 85))
	jumped, err := EvaluateAdtySiteTierAt(site.Id, 1_800_000_100, "eval-period-jump")
	require.NoError(t, err)
	assert.Equal(t, "eval-high", jumped.TierCode)
	require.NotNil(t, jumped.TierCostFactor)
	assert.Equal(t, "0.7", jumped.TierCostFactor.String())
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.RequiredBalance.Equal(adtySiteTierUSD("50")))
	cycleBefore := margin.CycleTierUSD

	// Root pin must not be overwritten, even if totals still qualify for high.
	pinned, err := SetAdtySiteTier(site.Id, "eval-mid", AdtySiteTierModeManual, 1_800_000_200)
	require.NoError(t, err)
	assert.Equal(t, "eval-mid", pinned.TierCode)
	assert.Equal(t, AdtySiteTierModeManual, pinned.TierMode)
	held, err := EvaluateAdtySiteTierAt(site.Id, 1_800_000_300, "eval-period-pinned")
	require.NoError(t, err)
	assert.Equal(t, "eval-mid", held.TierCode)
	assert.Equal(t, AdtySiteTierModeManual, held.TierMode)
	var pinRows int64
	require.NoError(t, db.Model(&model.AdtySiteTierEvaluation{}).Where("site_id = ? AND period_id = ?", site.Id, "eval-period-pinned").Count(&pinRows).Error)
	assert.EqualValues(t, 0, pinRows, "a manual pin must not consume an auto-evaluation period")

	// After unpin, the next evaluation uses current lifetime totals (still high).
	unpinned, err := SetAdtySiteTier(site.Id, "eval-mid", AdtySiteTierModeAuto, 1_800_000_400)
	require.NoError(t, err)
	assert.Equal(t, AdtySiteTierModeAuto, unpinned.TierMode)
	restored, err := EvaluateAdtySiteTierAt(site.Id, 1_800_000_500, "eval-period-unpinned")
	require.NoError(t, err)
	assert.Equal(t, "eval-high", restored.TierCode)
	assert.Equal(t, AdtySiteTierModeAuto, restored.TierMode)

	// Demote by disabling users and refunding principal so only the low grade still qualifies.
	for _, user := range users[1:] {
		require.NoError(t, db.Model(user).Update("status", common.UserStatusDisabled).Error)
	}
	require.NoError(t, db.Model(&model.AdtyTopupOrder{}).Where("trade_no = ?", "eval-prin-2").Update("refund_quota", adtySiteTierQuota("70")).Error)
	require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", "eval-prin-2").Update("status", model.TopUpStatusRefunded).Error)
	require.NoError(t, db.Model(&model.BillingOperation{}).Where("operation_key = ?", "eval-cons-high").Update("state", model.BillingRefunded).Error)

	var warnings int
	oldNotify := adtyNotifyRootAdmins
	adtyNotifyRootAdmins = func(func(string) (string, string)) { warnings++ }
	t.Cleanup(func() { adtyNotifyRootAdmins = oldNotify })

	demoted, err := EvaluateAdtySiteTierAt(site.Id, 1_800_000_600, "eval-period-demote")
	require.NoError(t, err)
	assert.Equal(t, "eval-low", demoted.TierCode)
	assert.Equal(t, model.AdtySiteStatusActive, demoted.Status)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.RequiredBalance.Equal(adtySiteTierUSD("15")))
	assert.True(t, margin.CycleTierUSD.Equal(cycleBefore), "a grade change must not rewrite the current margin cycle cap")
	assert.Equal(t, 0, warnings)

	// Falling below every configured grade warns Root and leaves the site open.
	require.NoError(t, db.Model(&model.BillingOperation{}).Where("operation_key = ?", "eval-cons-low").Update("state", model.BillingCanceled).Error)
	failed, err := EvaluateAdtySiteTierAt(site.Id, 1_800_000_700, "eval-period-warn")
	require.NoError(t, err)
	assert.Empty(t, failed.TierCode)
	assert.Equal(t, model.AdtySiteStatusActive, failed.Status)
	assert.Equal(t, 1, warnings)
	var warningRows int64
	require.NoError(t, db.Model(&model.AdtySiteTierEvaluation{}).Where("site_id = ? AND period_id = ? AND warned = ?", site.Id, "eval-period-warn", true).Count(&warningRows).Error)
	assert.EqualValues(t, 1, warningRows)

	updatedAt := failed.TierUpdatedAt
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	requiredBefore := margin.RequiredBalance
	replay, err := EvaluateAdtySiteTierAt(site.Id, 1_800_000_800, "eval-period-warn")
	require.NoError(t, err)
	assert.Empty(t, replay.TierCode)
	assert.Equal(t, updatedAt, replay.TierUpdatedAt)
	assert.Equal(t, 1, warnings, "re-running the same period must not notify again")
	require.NoError(t, db.Model(&model.AdtySiteTierEvaluation{}).Where("site_id = ? AND period_id = ?", site.Id, "eval-period-warn").Count(&warningRows).Error)
	assert.EqualValues(t, 1, warningRows)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).First(&margin).Error)
	assert.True(t, margin.RequiredBalance.Equal(requiredBefore))
	var ledgerCount int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ? AND wallet_kind = ?", site.Id, model.AdtySiteWalletMargin).Count(&ledgerCount).Error)
	assert.EqualValues(t, 0, ledgerCount, "evaluation must not debit or credit the margin wallet")
}

func TestSelectAdtySiteTierIgnoresMonthlySpendAndPartialMatches(t *testing.T) {
	low := model.AdtySiteTier{Code: "sel-low", Name: "Sel Low", Enabled: true, SortOrder: 1}
	mid := model.AdtySiteTier{Code: "sel-mid", Name: "Sel Mid", MinEffectiveUsers: 4, MinPrincipalUSD: adtySiteTierUSD("20"), MinConsumptionUSD: adtySiteTierUSD("10"), Enabled: true, SortOrder: 2, MinMonthlyUSD: adtySiteTierUSD("10000")}
	high := model.AdtySiteTier{Code: "sel-high", Name: "Sel High", MinEffectiveUsers: 8, MinPrincipalUSD: adtySiteTierUSD("40"), MinConsumptionUSD: adtySiteTierUSD("30"), Enabled: true, SortOrder: 3}
	disabled := model.AdtySiteTier{Code: "sel-off", Name: "Sel Off", Enabled: false, SortOrder: 9}
	tiers := []model.AdtySiteTier{low, mid, high, disabled}

	// Two of three mid thresholds are met, so the site stays on low.
	selected := SelectAdtySiteTier(tiers, AdtySiteTierStats{EffectiveUsers: 4, PrincipalUSD: adtySiteTierUSD("20"), ConsumptionUSD: adtySiteTierUSD("9")})
	require.NotNil(t, selected)
	assert.Equal(t, "sel-low", selected.Code)

	// All mid thresholds are met; monthly spend is irrelevant.
	selected = SelectAdtySiteTier(tiers, AdtySiteTierStats{EffectiveUsers: 4, PrincipalUSD: adtySiteTierUSD("20"), ConsumptionUSD: adtySiteTierUSD("10")})
	require.NotNil(t, selected)
	assert.Equal(t, "sel-mid", selected.Code)

	// All high thresholds are met, so the selection jumps over mid.
	selected = SelectAdtySiteTier(tiers, AdtySiteTierStats{EffectiveUsers: 8, PrincipalUSD: adtySiteTierUSD("40"), ConsumptionUSD: adtySiteTierUSD("30")})
	require.NotNil(t, selected)
	assert.Equal(t, "sel-high", selected.Code)

	selected = SelectAdtySiteTier([]model.AdtySiteTier{mid, high, disabled}, AdtySiteTierStats{})
	assert.Nil(t, selected)
}

func TestValidateAdtySiteTierRejectsIllegalLifetimeThresholds(t *testing.T) {
	assert.ErrorIs(t, ValidateAdtySiteTier(model.AdtySiteTier{Code: "bad", Name: "Bad", MinEffectiveUsers: -1}), ErrAdtySiteTierInvalid)
	assert.ErrorIs(t, ValidateAdtySiteTier(model.AdtySiteTier{Code: "bad", Name: "Bad", MinPrincipalUSD: decimal.RequireFromString("-0.01")}), ErrAdtySiteTierInvalid)
	assert.ErrorIs(t, ValidateAdtySiteTier(model.AdtySiteTier{Code: "bad", Name: "Bad", MinConsumptionUSD: decimal.RequireFromString("0.000000001")}), ErrAdtySiteTierInvalid)
	assert.NoError(t, ValidateAdtySiteTier(model.AdtySiteTier{Code: "ok", Name: "Ok", MinEffectiveUsers: 0, MinPrincipalUSD: decimal.Zero, MinConsumptionUSD: decimal.Zero}))
}
