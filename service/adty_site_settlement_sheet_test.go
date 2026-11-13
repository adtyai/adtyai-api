package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDailySettlementSheetSumsRecordedFactsAndDoesNotCreditWallets(t *testing.T) {
	db := prepareSiteDB(t)
	t.Cleanup(func() { _ = db.Delete(&model.Option{Key: adtyBusinessTimezoneKey}).Error })
	require.NoError(t, db.Delete(&model.Option{Key: adtyBusinessTimezoneKey}).Error)
	_, site := openActiveSite(t, db, "sheet"+strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10))
	_, other := openActiveSite(t, db, "other"+strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10))
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(10), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "sheet-earn-" + strconv.Itoa(site.Id),
	})
	require.NoError(t, err)

	now := time.Date(2026, 3, 15, 0, 30, 0, 0, time.UTC)
	start := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC).Unix()
	end := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC).Unix()
	stampSheetFacts(t, db, site, other, start)

	var ledgersBefore int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Count(&ledgersBefore).Error)
	var earning model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earning).Error)

	count, err := GenerateAdtySiteSettlementSheets(now)
	require.NoError(t, err)
	require.GreaterOrEqual(t, count, 1)

	var sheet model.AdtySiteSettlementSheet
	require.NoError(t, db.Where("site_id = ? AND business_day = ?", site.Id, "2026-03-14").First(&sheet).Error)
	require.Equal(t, "UTC", sheet.Timezone)
	require.Equal(t, start, sheet.DayStart)
	require.Equal(t, end, sheet.DayEnd)
	require.Equal(t, "site-settlement:"+strconv.Itoa(site.Id)+":2026-03-14", sheet.BatchId)
	require.Equal(t, start+5000, sheet.EventWatermark)
	require.True(t, sheet.TopupFaceUSD.Equal(decimal.NewFromInt(30)))
	require.True(t, sheet.PaidUSD.Equal(decimal.NewFromInt(20)))
	var paid map[string]string
	require.NoError(t, common.UnmarshalJsonStr(sheet.PaidByCurrency, &paid))
	require.Equal(t, "70", paid["CNY"])
	require.NotContains(t, paid, "USD")
	require.True(t, sheet.ConsumptionUSD.Equal(decimal.NewFromInt(8)))
	require.True(t, sheet.SupplyCostUSD.Equal(decimal.NewFromInt(5)))
	require.True(t, sheet.SpreadUSD.Equal(decimal.NewFromInt(3)))
	require.True(t, sheet.GiftFundingUSD.Equal(decimal.NewFromInt(2)))
	require.True(t, sheet.GiftReleaseUSD.Equal(decimal.RequireFromString("0.5")))
	require.True(t, sheet.GiftReserveUSD.Equal(decimal.RequireFromString("1.5")))
	require.True(t, sheet.TrialExpenseUSD.Equal(decimal.RequireFromString("1.25")))
	require.True(t, sheet.CommissionUSD.Equal(decimal.RequireFromString("1.25")))
	require.True(t, sheet.PendingCoverageUSD.Equal(decimal.RequireFromString("2")))
	require.True(t, sheet.MarginDeltaUSD.Equal(decimal.NewFromInt(6)))
	require.True(t, sheet.EarningsDeltaUSD.Equal(decimal.RequireFromString("-0.5")))
	require.True(t, sheet.EarningsAvailableDeltaUSD.Equal(decimal.RequireFromString("-0.5")))
	require.True(t, sheet.EarningsFrozenDeltaUSD.Equal(decimal.NewFromInt(3)))
	require.True(t, sheet.WithdrawalUSD.Equal(decimal.NewFromInt(3)))
	require.True(t, sheet.WithdrawalFeeUSD.Equal(decimal.RequireFromString("0.2")))
	require.True(t, sheet.AdjustmentUSD.Equal(decimal.NewFromInt(-1)))
	require.Equal(t, int64(2), sheet.AnomalyCount)

	var ledgersAfter int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Count(&ledgersAfter).Error)
	require.Equal(t, ledgersBefore, ledgersAfter)
	var earningAfter model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletEarnings).First(&earningAfter).Error)
	require.True(t, earningAfter.Balance.Equal(earning.Balance))

	require.NoError(t, db.Create(&model.AdtySiteEarningDaily{
		SiteId: site.Id, UserId: 3, Day: "2026-03-14", Kind: model.AdtySiteEarningUsage,
		Revenue: decimal.NewFromInt(1), Platform: decimal.Zero, Earning: decimal.NewFromInt(1), Status: model.AdtySiteEarningPending,
	}).Error)
	again, err := GenerateAdtySiteSettlementSheets(now)
	require.NoError(t, err)
	require.GreaterOrEqual(t, again, 1)
	var refreshed model.AdtySiteSettlementSheet
	require.NoError(t, db.Where("site_id = ? AND business_day = ?", site.Id, "2026-03-14").First(&refreshed).Error)
	require.Equal(t, sheet.BatchId, refreshed.BatchId)
	require.Equal(t, sheet.Id, refreshed.Id)
	require.True(t, refreshed.SpreadUSD.Equal(decimal.NewFromInt(4)))
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Count(&ledgersAfter).Error)
	require.Equal(t, ledgersBefore, ledgersAfter)

	require.NoError(t, SaveAdtyBusinessTimezone("Asia/Shanghai"))
	_, err = GenerateAdtySiteSettlementSheets(now)
	require.NoError(t, err)
	require.NoError(t, db.Where("site_id = ? AND business_day = ?", site.Id, "2026-03-14").First(&refreshed).Error)
	require.Equal(t, start, refreshed.DayStart)
	require.Equal(t, "UTC", refreshed.Timezone)

	require.Error(t, db.Model(&model.AdtySiteSettlementSheet{}).Where("id = ?", sheet.Id).Update("batch_id", "rewritten").Error)
	require.Error(t, db.Delete(&model.AdtySiteSettlementSheet{}, sheet.Id).Error)

	own, err := ListAdtySiteSettlementSheets(site.Id)
	require.NoError(t, err)
	require.Len(t, own, 1)
	require.Equal(t, site.Id, own[0].SiteId)
	all, err := ListAllAdtySiteSettlementSheets()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all), 2)
}

func TestDailySettlementSheetUsesConfiguredTimezoneOrUTC(t *testing.T) {
	db := prepareSiteDB(t)
	t.Cleanup(func() { _ = db.Delete(&model.Option{Key: adtyBusinessTimezoneKey}).Error })
	_, site := openActiveSite(t, db, "tz"+strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10))
	require.Error(t, SaveAdtyBusinessTimezone("Not/AZone"))
	require.NoError(t, SaveAdtyBusinessTimezone("Asia/Shanghai"))

	now := time.Date(2026, 6, 1, 16, 30, 0, 0, time.UTC) // 2026-06-02 00:30 in Shanghai
	inside := time.Date(2026, 6, 1, 15, 59, 0, 0, time.UTC).Unix()
	outside := time.Date(2026, 6, 1, 16, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, db.Create(&model.TopUp{TradeNo: "TZ-IN-" + strconv.Itoa(site.Id), UserId: 1, Status: common.TopUpStatusSuccess, CompleteTime: inside}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: "TZ-IN-" + strconv.Itoa(site.Id), UserId: 1, SiteId: site.Id, FaceUSD: decimal.NewFromInt(4), PaidAmount: decimal.NewFromInt(4), PaidCurrency: "USD"}).Error)
	require.NoError(t, db.Create(&model.TopUp{TradeNo: "TZ-OUT-" + strconv.Itoa(site.Id), UserId: 1, Status: common.TopUpStatusSuccess, CompleteTime: outside}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: "TZ-OUT-" + strconv.Itoa(site.Id), UserId: 1, SiteId: site.Id, FaceUSD: decimal.NewFromInt(9), PaidAmount: decimal.NewFromInt(9), PaidCurrency: "USD"}).Error)

	_, err := GenerateAdtySiteSettlementSheets(now)
	require.NoError(t, err)
	var sheet model.AdtySiteSettlementSheet
	require.NoError(t, db.Where("site_id = ? AND business_day = ?", site.Id, "2026-06-01").First(&sheet).Error)
	require.Equal(t, "Asia/Shanghai", sheet.Timezone)
	require.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, mustLoc(t, "Asia/Shanghai")).Unix(), sheet.DayStart)
	require.True(t, sheet.TopupFaceUSD.Equal(decimal.NewFromInt(4)))

	require.NoError(t, SaveAdtyBusinessTimezone(""))
	_, err = GenerateAdtySiteSettlementSheets(time.Date(2026, 6, 3, 0, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, db.Where("site_id = ? AND business_day = ?", site.Id, "2026-06-01").First(&sheet).Error)
	require.Equal(t, "Asia/Shanghai", sheet.Timezone, "a later timezone change does not rewrite the saved sheet")
	var utcSheet model.AdtySiteSettlementSheet
	require.NoError(t, db.Where("site_id = ? AND business_day = ?", site.Id, "2026-06-02").First(&utcSheet).Error)
	require.Equal(t, "UTC", utcSheet.Timezone)
	require.Equal(t, time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC).Unix(), utcSheet.DayStart)
}

func stampSheetFacts(t *testing.T, db *gorm.DB, site, other *model.AdtySite, start int64) {
	t.Helper()
	require.NoError(t, db.Create(&[]model.TopUp{
		{TradeNo: "SH-USD-" + strconv.Itoa(site.Id), UserId: 1, Status: common.TopUpStatusSuccess, CompleteTime: start + 10},
		{TradeNo: "SH-CNY-" + strconv.Itoa(site.Id), UserId: 1, Status: common.TopUpStatusSuccess, CompleteTime: start + 20},
		{TradeNo: "SH-PEND-" + strconv.Itoa(site.Id), UserId: 1, Status: common.TopUpStatusPending, CompleteTime: start + 30},
		{TradeNo: "SH-OTHER-" + strconv.Itoa(other.Id), UserId: 1, Status: common.TopUpStatusSuccess, CompleteTime: start + 40},
	}).Error)
	require.NoError(t, db.Create(&[]model.AdtyTopupOrder{
		{TradeNo: "SH-USD-" + strconv.Itoa(site.Id), SiteId: site.Id, UserId: 1, FaceUSD: decimal.NewFromInt(20), PaidAmount: decimal.NewFromInt(20), PaidCurrency: "USD"},
		{TradeNo: "SH-CNY-" + strconv.Itoa(site.Id), SiteId: site.Id, UserId: 1, FaceUSD: decimal.NewFromInt(10), PaidAmount: decimal.NewFromInt(70), PaidCurrency: "CNY"},
		{TradeNo: "SH-PEND-" + strconv.Itoa(site.Id), SiteId: site.Id, UserId: 1, FaceUSD: decimal.NewFromInt(99), PaidAmount: decimal.NewFromInt(99), PaidCurrency: "USD"},
		{TradeNo: "SH-OTHER-" + strconv.Itoa(other.Id), SiteId: other.Id, UserId: 1, FaceUSD: decimal.NewFromInt(500), PaidAmount: decimal.NewFromInt(500), PaidCurrency: "USD"},
	}).Error)
	require.NoError(t, db.Create(&[]model.AdtySiteEarningDaily{
		{SiteId: site.Id, UserId: 1, Day: "2026-03-14", Kind: model.AdtySiteEarningUsage, Revenue: decimal.NewFromInt(8), Platform: decimal.NewFromInt(5), Earning: decimal.NewFromInt(3), Status: model.AdtySiteEarningPending},
		{SiteId: site.Id, UserId: 1, Day: "2026-03-13", Kind: model.AdtySiteEarningUsage, Revenue: decimal.NewFromInt(100), Platform: decimal.NewFromInt(100), Earning: decimal.NewFromInt(100), Status: model.AdtySiteEarningPending},
		{SiteId: site.Id, UserId: 1, Day: "2026-03-14+1", Kind: model.AdtySiteEarningSkipped, Earning: decimal.NewFromInt(9), Status: model.AdtySiteEarningPending, Reason: model.AdtySiteRiskSelfUse},
		{SiteId: site.Id, UserId: 2, Day: "2026-03-14", Kind: model.AdtySiteEarningBonus, Earning: decimal.NewFromInt(-2), Status: model.AdtySiteEarningPending},
		{SiteId: site.Id, UserId: 2, Day: "2026-03-14", Kind: model.AdtySiteEarningBonusReversal, Earning: decimal.RequireFromString("0.5"), Status: model.AdtySiteEarningPending},
	}).Error)
	require.NoError(t, db.Create(&model.AdtySiteTrialGrant{SiteId: site.Id, UserId: 1, AmountUSD: decimal.RequireFromString("1.25"), Day: "2026-03-14", IdempotencyKey: "trial-" + strconv.Itoa(site.Id)}).Error)
	rows := []model.AdtySiteWalletLedger{
		{SiteId: site.Id, WalletKind: model.AdtySiteWalletEarnings, Delta: decimal.NewFromInt(4), Type: "site_earning", IdempotencyKey: "sheet-earn-row-" + strconv.Itoa(site.Id)},
		{SiteId: site.Id, WalletKind: model.AdtySiteWalletEarnings, Delta: decimal.RequireFromString("-1.5"), Type: "gift_reserve", IdempotencyKey: "sheet-reserve-" + strconv.Itoa(site.Id)},
		{SiteId: site.Id, WalletKind: model.AdtySiteWalletEarnings, Delta: decimal.NewFromInt(-3), Type: "earning_withdraw_freeze", IdempotencyKey: "sheet-freeze-" + strconv.Itoa(site.Id)},
		{SiteId: site.Id, WalletKind: model.AdtySiteWalletMargin, Delta: decimal.NewFromInt(6), Type: "margin_cycle_open", IdempotencyKey: "sheet-margin-" + strconv.Itoa(site.Id)},
		{SiteId: site.Id, WalletKind: model.AdtySiteWalletOperating, Delta: decimal.NewFromInt(-1), Type: "adjustment", IdempotencyKey: "sheet-adjust-" + strconv.Itoa(site.Id)},
	}
	require.NoError(t, db.Create(&rows).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&model.AdtySiteWalletLedger{}).Where("idempotency_key = ?", "sheet-earn-row-"+strconv.Itoa(site.Id)).Update("created_at", start+10).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&model.AdtySiteWalletLedger{}).Where("idempotency_key = ?", "sheet-reserve-"+strconv.Itoa(site.Id)).Update("created_at", start+20).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&model.AdtySiteWalletLedger{}).Where("idempotency_key = ?", "sheet-freeze-"+strconv.Itoa(site.Id)).Update("created_at", start+30).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&model.AdtySiteWalletLedger{}).Where("idempotency_key = ?", "sheet-margin-"+strconv.Itoa(site.Id)).Update("created_at", start+40).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&model.AdtySiteWalletLedger{}).Where("idempotency_key = ?", "sheet-adjust-"+strconv.Itoa(site.Id)).Update("created_at", start+5000).Error)
	require.NoError(t, db.Create(&[]model.AdtySiteInviteCommission{
		{SiteId: site.Id, TradeNo: "SH-COMM-" + strconv.Itoa(site.Id), AmountUSD: decimal.RequireFromString("1.25"), Status: model.AdtySiteCommissionPendingCoverage},
		{SiteId: site.Id, TradeNo: "SH-OLD-" + strconv.Itoa(site.Id), AmountUSD: decimal.RequireFromString("0.75"), Status: model.AdtySiteCommissionPendingCoverage},
		{SiteId: site.Id, TradeNo: "SH-NEW-" + strconv.Itoa(site.Id), AmountUSD: decimal.NewFromInt(9), Status: model.AdtySiteCommissionPendingCoverage},
	}).Error)
	require.NoError(t, db.Model(&model.AdtySiteInviteCommission{}).Where("trade_no = ?", "SH-COMM-"+strconv.Itoa(site.Id)).Update("created_at", start+50).Error)
	require.NoError(t, db.Model(&model.AdtySiteInviteCommission{}).Where("trade_no = ?", "SH-OLD-"+strconv.Itoa(site.Id)).Update("created_at", start-10).Error)
	require.NoError(t, db.Model(&model.AdtySiteInviteCommission{}).Where("trade_no = ?", "SH-NEW-"+strconv.Itoa(site.Id)).Update("created_at", start+86400).Error)
	require.NoError(t, db.Create(&model.AdtySiteWithdrawal{
		SiteId: site.Id, AmountUSD: decimal.NewFromInt(3), FeeUSD: decimal.RequireFromString("0.2"), NetUSD: decimal.RequireFromString("2.8"),
		Method: "alipay", Status: model.AdtySiteWithdrawPending, IdempotencyKey: "sheet-wd-" + strconv.Itoa(site.Id),
	}).Error)
	require.NoError(t, db.Model(&model.AdtySiteWithdrawal{}).Where("idempotency_key = ?", "sheet-wd-"+strconv.Itoa(site.Id)).Update("created_at", start+60).Error)
	require.NoError(t, db.Create(&[]model.AdtyAnomalyDisposition{
		{Kind: "balance_gap", Ref: strconv.Itoa(site.Id), Status: "open"},
		{Kind: "duplicate_credit", Ref: "SH-USD-" + strconv.Itoa(site.Id), Status: "open"},
		{Kind: "balance_gap", Ref: strconv.Itoa(other.Id), Status: "open"},
		{Kind: "unknown", Ref: "no-site", Status: "open"},
	}).Error)
	require.NoError(t, db.Model(&model.AdtyAnomalyDisposition{}).Where("ref = ?", strconv.Itoa(site.Id)).Update("created_at", start+70).Error)
	require.NoError(t, db.Model(&model.AdtyAnomalyDisposition{}).Where("ref = ?", "SH-USD-"+strconv.Itoa(site.Id)).Update("created_at", start+80).Error)
	require.NoError(t, db.Model(&model.AdtyAnomalyDisposition{}).Where("ref = ?", strconv.Itoa(other.Id)).Update("created_at", start+90).Error)
	require.NoError(t, db.Model(&model.AdtyAnomalyDisposition{}).Where("ref = ?", "no-site").Update("created_at", start+100).Error)
}

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}
