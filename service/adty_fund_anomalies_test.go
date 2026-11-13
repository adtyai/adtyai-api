package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCollectAdtyFundAnomaliesCoversMissingG134Kinds(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "g134-collect")
	customer := siteCustomer(t, db, site, "g134-buyer")

	unbookedTrade := "UNBOOKED-" + strconv.Itoa(site.Id)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: customer.Id, Amount: 10, TradeNo: unbookedTrade,
		PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt,
		Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: unbookedTrade, UserId: customer.Id, SiteId: site.Id,
		Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		FaceUSD: decimal.NewFromInt(10), CreditQuota: 5_000_000,
		PayAmount: decimal.NewFromInt(10), PayCurrency: "USDT",
	}).Error)

	giftTrade := "GIFT-SHORT-" + strconv.Itoa(site.Id)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: customer.Id, Amount: 20, TradeNo: giftTrade,
		PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt,
		Status: common.TopUpStatusPending,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: giftTrade, UserId: customer.Id, SiteId: site.Id,
		Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		FaceUSD: decimal.NewFromInt(20), BonusUSD: decimal.NewFromInt(1),
		PromoBonusUSD: decimal.NewFromInt(2), PromoFunder: model.AdtyPromoFunderSiteOwner,
		CreditQuota: 11_500_000, GiftReserveEarning: decimal.NewFromInt(1),
	}).Error)

	quota := int64(1_000_000)
	k := decimal.RequireFromString("1.25")
	d := decimal.RequireFromString("0.8")
	_, bookedPlatform, bookedEarning := AdtySiteEarningSplit(quota, k, d)
	_, officialPlatform, officialEarning := OfficialAdtySupplySplit(quota, k)
	require.False(t, officialPlatform.Equal(bookedPlatform))
	require.False(t, officialEarning.Equal(bookedEarning))
	earningKey := "request:g134-" + strconv.Itoa(site.Id)
	require.NoError(t, db.Create(&model.AdtySiteEarningEvent{
		OperationKey: earningKey, SiteId: site.Id, UserId: customer.Id,
		Quota: int(quota), Markup: 1.25, CostFactor: 0.8, Day: "2026-11-14",
	}).Error)
	require.NoError(t, db.Create(&model.AdtySiteEarningDaily{
		SiteId: site.Id, UserId: customer.Id, Day: "2026-11-14",
		Kind: model.AdtySiteEarningUsage, Quota: quota,
		Revenue: decimal.NewFromInt(2), Platform: bookedPlatform, Earning: bookedEarning,
		Status: model.AdtySiteEarningPending,
	}).Error)

	refundTrade := "REFUND-MIS-" + strconv.Itoa(site.Id)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: customer.Id, Amount: 10, TradeNo: refundTrade,
		PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt,
		Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: refundTrade, UserId: customer.Id, SiteId: site.Id,
		Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		FaceUSD: decimal.NewFromInt(10), CreditQuota: 5_000_000,
		PayAmount: decimal.NewFromInt(10), PayCurrency: "USDT",
		RefundQuota: 4_000_000,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyBalanceLedger{
		UserId: customer.Id, DeltaQuota: 5_000_000, Type: model.AdtyLedgerTypeTopUp,
		RefType: "topup", RefId: refundTrade,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyManualRefund{
		RefundId: "RF-H-" + strconv.Itoa(site.Id), TradeNo: refundTrade,
		UserId: customer.Id, SiteId: site.Id, Status: model.AdtyManualRefundStatusApproved,
		RequestUSD: decimal.NewFromInt(8), RequestQuota: 4_000_000,
	}).Error)

	walletsBefore := snapshotSiteWallets(t, db, site.Id)
	siteLedgersBefore, userLedgersBefore := countSiteAndUserLedgers(t, db, site.Id, customer.Id)
	quotaBefore := loadUserQuota(t, customer.Id)

	kinds, err := CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	require.Contains(t, kinds, AdtyAnomalyOrderSuccessUnbooked)
	require.Contains(t, kinds, AdtyAnomalyGiftGuaranteeShort)
	require.Contains(t, kinds, AdtyAnomalyEarningSupplyMismatch)
	require.Contains(t, kinds, AdtyAnomalyRefundAmountMismatch)

	rows, err := ListAdtyAnomalyDispositions(site.Id)
	require.NoError(t, err)
	byKind := dispositionsByKindRef(rows)
	unbooked := byKind[AdtyAnomalyOrderSuccessUnbooked+"|"+unbookedTrade]
	require.Equal(t, "open", unbooked.Status)
	require.Equal(t, site.Id, unbooked.SiteId)
	require.Equal(t, customer.Id, unbooked.AccountId)
	require.Equal(t, unbookedTrade, unbooked.OrderRef)
	require.Equal(t, "10", unbooked.Difference)
	require.Contains(t, unbooked.Evidence, "expected_credit_quota")

	gift := byKind[AdtyAnomalyGiftGuaranteeShort+"|"+giftTrade]
	require.Equal(t, site.Id, gift.SiteId)
	require.Equal(t, customer.Id, gift.AccountId)
	require.Equal(t, giftTrade, gift.OrderRef)
	require.Equal(t, "2", gift.Difference)
	require.Contains(t, gift.Evidence, "required_usd")

	earning := byKind[AdtyAnomalyEarningSupplyMismatch+"|"+earningKey]
	require.Equal(t, site.Id, earning.SiteId)
	require.Equal(t, customer.Id, earning.AccountId)
	require.Equal(t, officialEarning.Sub(bookedEarning).Abs().String(), earning.Difference)
	require.Contains(t, earning.Evidence, "official_platform")

	refund := byKind[AdtyAnomalyRefundAmountMismatch+"|"+refundTrade]
	require.Equal(t, site.Id, refund.SiteId)
	require.Equal(t, customer.Id, refund.AccountId)
	require.Equal(t, refundTrade, refund.OrderRef)
	require.Equal(t, "6", refund.Difference)
	require.Contains(t, refund.Evidence, "occupied")

	kindsAgain, err := CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	require.Contains(t, kindsAgain, AdtyAnomalyOrderSuccessUnbooked)
	require.Contains(t, kindsAgain, AdtyAnomalyGiftGuaranteeShort)
	require.Contains(t, kindsAgain, AdtyAnomalyEarningSupplyMismatch)
	require.Contains(t, kindsAgain, AdtyAnomalyRefundAmountMismatch)

	var openCount int64
	require.NoError(t, db.Model(&model.AdtyAnomalyDisposition{}).
		Where("site_id = ? AND status = ? AND kind IN ?", site.Id, "open", []string{
			AdtyAnomalyOrderSuccessUnbooked, AdtyAnomalyGiftGuaranteeShort,
			AdtyAnomalyEarningSupplyMismatch, AdtyAnomalyRefundAmountMismatch,
		}).Count(&openCount).Error)
	require.EqualValues(t, 4, openCount)

	require.Equal(t, walletsBefore, snapshotSiteWallets(t, db, site.Id))
	siteLedgersAfter, userLedgersAfter := countSiteAndUserLedgers(t, db, site.Id, customer.Id)
	require.Equal(t, siteLedgersBefore, siteLedgersAfter)
	require.Equal(t, userLedgersBefore, userLedgersAfter)
	require.Equal(t, quotaBefore, loadUserQuota(t, customer.Id))
}

func TestCollectAdtyFundAnomaliesKeepsExistingKindsIdempotent(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "g134-existing")
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("100"), "g134-cycle"))
	require.NoError(t, db.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).
		Update("cycle_used_usd", "91").Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-credit-g134", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "credited"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-credit-g134", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "credited"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-refund-g134", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "received", EventType: "refund.succeeded"}).Error)
	require.NoError(t, db.Create(&model.AdtyOrderPaymentEvent{TradeNo: "dup-refund-g134", SiteId: site.Id, UserId: 1, Provider: "epay", Source: "callback", Outcome: "received", EventType: "refund.succeeded"}).Error)

	walletsBefore := snapshotSiteWallets(t, db, site.Id)
	kinds, err := CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	require.Contains(t, kinds, AdtyAnomalyMargin)
	require.Contains(t, kinds, AdtyAnomalyDuplicateCredit)
	require.Contains(t, kinds, AdtyAnomalyDuplicateRefund)

	rows, err := ListAdtyAnomalyDispositions(site.Id)
	require.NoError(t, err)
	byKind := dispositionsByKindRef(rows)
	margin := byKind[AdtyAnomalyMargin+"|"+strconv.Itoa(site.Id)]
	require.Equal(t, site.Id, margin.SiteId)
	require.NotEmpty(t, margin.Difference)
	require.Contains(t, margin.Evidence, "cycle_overage_usd")
	require.Contains(t, byKind[AdtyAnomalyDuplicateCredit+"|"+strconv.Itoa(site.Id)].Evidence, "dup-credit-g134")

	_, err = CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	var openCount int64
	require.NoError(t, db.Model(&model.AdtyAnomalyDisposition{}).
		Where("site_id = ? AND status = ? AND kind IN ?", site.Id, "open", []string{
			AdtyAnomalyMargin, AdtyAnomalyDuplicateCredit, AdtyAnomalyDuplicateRefund,
		}).Count(&openCount).Error)
	require.EqualValues(t, 3, openCount)
	require.Equal(t, walletsBefore, snapshotSiteWallets(t, db, site.Id))
}

func TestCollectAdtyFundAnomaliesFlagsPayoutMismatchWithoutMutatingLedgers(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "g134-payout")
	customer := siteCustomer(t, db, site, "g134-payout-buyer")
	trade := "REFUND-PAY-" + strconv.Itoa(site.Id)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: customer.Id, Amount: 10, TradeNo: trade,
		PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt,
		Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: trade, UserId: customer.Id, SiteId: site.Id,
		Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		FaceUSD: decimal.NewFromInt(10), CreditQuota: 5_000_000,
		PayAmount: decimal.NewFromInt(10), PayCurrency: "USDT", RefundQuota: 0,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyBalanceLedger{
		UserId: customer.Id, DeltaQuota: 5_000_000, Type: model.AdtyLedgerTypeTopUp,
		RefType: "topup", RefId: trade,
	}).Error)
	require.NoError(t, db.Create(&model.AdtyManualRefund{
		RefundId: "RF-PAY-" + strconv.Itoa(site.Id), TradeNo: trade,
		UserId: customer.Id, SiteId: site.Id, Status: model.AdtyManualRefundStatusApproved,
		RequestUSD: decimal.NewFromInt(4), RequestQuota: 2_000_000,
		ForeignReference: decimal.NewFromInt(4), PayoutStatus: model.AdtyManualRefundPayoutSuccess,
		PayoutAmount: decimal.NewFromInt(1),
	}).Error)

	siteLedgersBefore, userLedgersBefore := countSiteAndUserLedgers(t, db, site.Id, customer.Id)
	kinds, err := CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	require.Contains(t, kinds, AdtyAnomalyRefundAmountMismatch)
	rows, err := ListAdtyAnomalyDispositions(site.Id)
	require.NoError(t, err)
	item := dispositionsByKindRef(rows)[AdtyAnomalyRefundAmountMismatch+"|"+trade]
	require.Equal(t, "3", item.Difference)
	require.Contains(t, item.Evidence, "paid-out amount")
	siteLedgersAfter, userLedgersAfter := countSiteAndUserLedgers(t, db, site.Id, customer.Id)
	require.Equal(t, siteLedgersBefore, siteLedgersAfter)
	require.Equal(t, userLedgersBefore, userLedgersAfter)
}

func snapshotSiteWallets(t *testing.T, db *gorm.DB, siteID int) []model.AdtySiteWallet {
	t.Helper()
	var wallets []model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ?", siteID).Order("id").Find(&wallets).Error)
	return wallets
}

func countSiteAndUserLedgers(t *testing.T, db *gorm.DB, siteID, userID int) (siteLedgers, userLedgers int64) {
	t.Helper()
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ?", siteID).Count(&siteLedgers).Error)
	require.NoError(t, db.Model(&model.AdtyBalanceLedger{}).Where("user_id = ?", userID).Count(&userLedgers).Error)
	return siteLedgers, userLedgers
}

func loadUserQuota(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").First(&user, userID).Error)
	return user.Quota
}

func dispositionsByKindRef(rows []model.AdtyAnomalyDisposition) map[string]model.AdtyAnomalyDisposition {
	out := map[string]model.AdtyAnomalyDisposition{}
	for _, row := range rows {
		out[row.Kind+"|"+row.Ref] = row
	}
	return out
}
