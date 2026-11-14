package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReserveSiteGiftUsesEarningThenAuthorizedMargin(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "gift-freeze")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("4"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "earn-4",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "gift-cycle"))
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	t.Cleanup(func() { adty_setting.AuthorizeSiteBonusMarginFill(site.Id, false) })
	ext := &model.AdtyTopupOrder{TradeNo: "GIFT-1", SiteId: site.Id, BonusUSD: decimal.RequireFromString("10"), Method: "usdt.trc20", BonusMethod: "usdt", BonusExpiresAt: 0}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReserveAdtySiteGiftTx(tx, ext)
	}))
	require.Equal(t, "earning+margin", ext.GiftReserveSource)
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("4")))
	require.True(t, ext.GiftReserveMargin.Equal(decimal.RequireFromString("6")))
	require.EqualValues(t, 0, ext.BonusExpiresAt)
	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("4")), "reserve must not spend earning balance")
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("20")), "reserve must not spend margin balance")
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("6")))
	assertGiftLedger(t, db, site.Id, "gift_reserve", "GIFT-1", model.AdtySiteWalletEarnings, "0", "4")
	assertGiftLedger(t, db, site.Id, "gift_reserve", "GIFT-1", model.AdtySiteWalletMargin, "0", "6")
}

func TestReserveSiteGiftIncludesOwnerPromoBonus(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "gift-promo-owner")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("4"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "promo-owner-earn-4",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "promo-owner-cycle"))
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	t.Cleanup(func() { adty_setting.AuthorizeSiteBonusMarginFill(site.Id, false) })
	ext := &model.AdtyTopupOrder{
		TradeNo: "GIFT-PROMO-OWNER", SiteId: site.Id,
		BonusUSD: decimal.RequireFromString("10"), PromoBonusUSD: decimal.RequireFromString("5"),
		PromoFunder: model.AdtyPromoFunderSiteOwner, Method: "usdt.trc20", BonusMethod: "usdt",
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReserveAdtySiteGiftTx(tx, ext)
	}))
	require.Equal(t, "earning+margin", ext.GiftReserveSource)
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("4")))
	require.True(t, ext.GiftReserveMargin.Equal(decimal.RequireFromString("11")))
	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("11")))
	assertGiftLedger(t, db, site.Id, "gift_reserve", "GIFT-PROMO-OWNER", model.AdtySiteWalletEarnings, "0", "4")
	assertGiftLedger(t, db, site.Id, "gift_reserve", "GIFT-PROMO-OWNER", model.AdtySiteWalletMargin, "0", "11")
}

func TestReserveSiteGiftCoversOwnerPromoBonusAlone(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "gift-promo-only")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("10"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "promo-only-earn-10",
	})
	require.NoError(t, err)
	ext := &model.AdtyTopupOrder{
		TradeNo: "GIFT-PROMO-ONLY", SiteId: site.Id,
		PromoBonusUSD: decimal.RequireFromString("5"), PromoFunder: model.AdtyPromoFunderSiteOwner,
		Method: "usdt.trc20",
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReserveAdtySiteGiftTx(tx, ext)
	}))
	require.Equal(t, "earning", ext.GiftReserveSource)
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("5")))
	require.True(t, ext.GiftReserveMargin.IsZero())
	earning, _ := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("5")))
	assertGiftLedger(t, db, site.Id, "gift_reserve", "GIFT-PROMO-ONLY", model.AdtySiteWalletEarnings, "0", "5")
}

func TestReserveSiteGiftIgnoresPlatformPromoBonus(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "gift-promo-platform")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("20"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "promo-platform-earn-20",
	})
	require.NoError(t, err)
	ext := &model.AdtyTopupOrder{
		TradeNo: "GIFT-PROMO-PLATFORM", SiteId: site.Id,
		BonusUSD: decimal.RequireFromString("10"), PromoBonusUSD: decimal.RequireFromString("5"),
		PromoFunder: model.AdtyPromoFunderPlatform, Method: "usdt.trc20", BonusMethod: "usdt",
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReserveAdtySiteGiftTx(tx, ext)
	}))
	require.Equal(t, "earning", ext.GiftReserveSource)
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("10")))
	require.True(t, ext.GiftReserveMargin.IsZero())
	earning, _ := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("10")))
	assertGiftLedger(t, db, site.Id, "gift_reserve", "GIFT-PROMO-PLATFORM", model.AdtySiteWalletEarnings, "0", "10")
}

func TestCancelSiteGiftReleasesFreezeIdempotently(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-cancel", "10")
	earningBefore, marginBefore := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningBefore.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, marginBefore.FrozenUSD.Equal(decimal.RequireFromString("6")))

	canceled, err := model.CancelAdtyTopUp(ext.TradeNo, customer.Id)
	require.NoError(t, err)
	require.Equal(t, ext.TradeNo, canceled.TradeNo)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.Balance.Equal(earningBefore.Balance))
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, margin.Balance.Equal(marginBefore.Balance))
	require.True(t, margin.FrozenUSD.IsZero())
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings, "0", "-4")
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin, "0", "-6")

	_, err = model.CancelAdtyTopUp(ext.TradeNo, customer.Id)
	require.ErrorIs(t, err, model.ErrAdtyTopUpNotCancel)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReleaseAdtySiteGiftTx(tx, ext)
	}))
	earningAgain, marginAgain := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningAgain.Balance.Equal(earning.Balance))
	require.True(t, earningAgain.FrozenUSD.IsZero())
	require.True(t, marginAgain.Balance.Equal(margin.Balance))
	require.True(t, marginAgain.FrozenUSD.IsZero())
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings))
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin))
}

func TestReplaceSiteGiftReleasesFreezeIdempotently(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-replace", "10")
	earningBefore, marginBefore := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningBefore.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, marginBefore.FrozenUSD.Equal(decimal.RequireFromString("6")))

	replaced, err := model.ReplaceAdtyTopUpAfterLock(ext.TradeNo, customer.Id, nil)
	require.NoError(t, err)
	require.Equal(t, ext.TradeNo, replaced.TradeNo)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.Balance.Equal(earningBefore.Balance))
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, margin.Balance.Equal(marginBefore.Balance))
	require.True(t, margin.FrozenUSD.IsZero())
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings, "0", "-4")
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin, "0", "-6")

	_, err = model.ReplaceAdtyTopUpAfterLock(ext.TradeNo, customer.Id, nil)
	require.ErrorIs(t, err, model.ErrAdtyTopUpNotCancel)
	_, err = model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.ErrorIs(t, err, model.ErrTopUpStatusInvalid)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReleaseAdtySiteGiftTx(tx, ext)
	}))
	earningAgain, marginAgain := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningAgain.Balance.Equal(earning.Balance))
	require.True(t, earningAgain.FrozenUSD.IsZero())
	require.True(t, marginAgain.Balance.Equal(margin.Balance))
	require.True(t, marginAgain.FrozenUSD.IsZero())
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings))
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin))
}

func TestExpireSiteGiftReleasesFreezeIdempotently(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, _, ext := seedReservedSiteGift(t, db, "gift-expire", "10")
	require.NoError(t, db.Model(ext).Update("expire_at", 100).Error)
	earningBefore, marginBefore := mustSiteGiftWallets(t, db, site.Id)

	expired, err := model.ExpireAdtyTopUps(common.GetTimestamp(), 50)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	require.Equal(t, ext.TradeNo, expired[0].TradeNo)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.Balance.Equal(earningBefore.Balance))
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, margin.Balance.Equal(marginBefore.Balance))
	require.True(t, margin.FrozenUSD.IsZero())
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings, "0", "-4")
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin, "0", "-6")

	again, err := model.ExpireAdtyTopUps(common.GetTimestamp(), 50)
	require.NoError(t, err)
	require.Empty(t, again)
	earningAgain, marginAgain := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningAgain.FrozenUSD.IsZero())
	require.True(t, marginAgain.FrozenUSD.IsZero())
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings))
}

func TestPayAndSettleSiteGiftDoesNotDoubleSpend(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site, _, ext := seedReservedSiteGift(t, db, "gift-pay", "10")
	earningBefore, marginBefore := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningBefore.Balance.Equal(decimal.RequireFromString("4")))
	require.True(t, earningBefore.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, marginBefore.Balance.Equal(decimal.RequireFromString("20")))
	require.True(t, marginBefore.FrozenUSD.Equal(decimal.RequireFromString("6")))

	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)

	earningPaid, marginPaid := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningPaid.Balance.Equal(earningBefore.Balance), "payment converts reserve to a guarantee hold, not a spend")
	require.True(t, earningPaid.FrozenUSD.Equal(earningBefore.FrozenUSD))
	require.True(t, marginPaid.Balance.Equal(marginBefore.Balance))
	require.True(t, marginPaid.FrozenUSD.Equal(marginBefore.FrozenUSD))

	settled, err := RunAdtySiteSettlementOnce(time.Now().AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Equal(t, 1, settled)

	earningSettled, marginSettled := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningSettled.Balance.Equal(earningBefore.Balance), "settlement must not debit the reserved gift again")
	require.True(t, earningSettled.FrozenUSD.Equal(earningBefore.FrozenUSD))
	require.True(t, marginSettled.Balance.Equal(marginBefore.Balance))
	require.True(t, marginSettled.FrozenUSD.Equal(marginBefore.FrozenUSD))
	require.EqualValues(t, 0, countLedgersByType(t, db, site.Id, "site_earning_settlement"))

	var bonus model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningBonus).First(&bonus).Error)
	require.True(t, bonus.Earning.Equal(decimal.RequireFromString("-10")))
	require.Equal(t, model.AdtySiteEarningAvailable, bonus.Status)
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	require.InDelta(t, 0, account(t, db, affiliate.Id).AvailableAmount, 1e-9, "gift settlement must not reduce the commission book")
}

func TestLatePayAfterCancelReacquiresGiftGuarantee(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-late-reacq", "10")
	_, err := model.CancelAdtyTopUp(ext.TradeNo, customer.Id)
	require.NoError(t, err)
	earningOpen, marginOpen := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningOpen.FrozenUSD.IsZero())
	require.True(t, marginOpen.FrozenUSD.IsZero())

	_, err = model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.Balance.Equal(earningOpen.Balance))
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, margin.Balance.Equal(marginOpen.Balance))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("6")))
	require.EqualValues(t, 55_000_000, userQuotaOf(t, db, customer.Id))
	require.EqualValues(t, 0, countGiftFulfillments(t, db, ext.TradeNo))
	assertGiftLedger(t, db, site.Id, "gift_reserve", ext.TradeNo, model.AdtySiteWalletEarnings, "0", "4")
	// The reacquire row is a second reserve with the same hold, distinct key.
	require.EqualValues(t, 2, countGiftLedgers(t, db, site.Id, "gift_reserve", ext.TradeNo, model.AdtySiteWalletEarnings))
}

func TestLatePayAfterExpireReacquiresGiftGuarantee(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-late-exp", "10")
	require.NoError(t, db.Model(ext).Update("expire_at", 100).Error)
	expired, err := model.ExpireAdtyTopUps(common.GetTimestamp(), 50)
	require.NoError(t, err)
	require.Len(t, expired, 1)

	_, err = model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("6")))
	require.EqualValues(t, 55_000_000, userQuotaOf(t, db, customer.Id))
	require.EqualValues(t, 0, countGiftFulfillments(t, db, ext.TradeNo))
}

func TestLatePayAfterCancelRecordsPendingGiftWhenUncovered(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-late-pend", "10")
	_, err := model.CancelAdtyTopUp(ext.TradeNo, customer.Id)
	require.NoError(t, err)
	require.NoError(t, drainSiteGiftWallets(t, db, site.Id, "gift-late-pend"))

	notifies := 0
	old := adtyNotifyGiftFulfillmentPending
	adtyNotifyGiftFulfillmentPending = func(*model.AdtyTopupOrder) { notifies++ }
	t.Cleanup(func() { adtyNotifyGiftFulfillmentPending = old })

	_, err = model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, margin.FrozenUSD.IsZero())
	require.EqualValues(t, 50_000_000, userQuotaOf(t, db, customer.Id))
	require.EqualValues(t, 1, notifies)
	var row model.AdtyGiftFulfillment
	require.NoError(t, db.Where("trade_no = ?", ext.TradeNo).First(&row).Error)
	require.Equal(t, model.AdtyGiftFulfillmentPending, row.Status)
	require.True(t, row.BonusUSD.Equal(decimal.RequireFromString("10")))
	require.EqualValues(t, 5_000_000, row.BonusQuota)
	reloaded, err := model.GetAdtyTopupOrder(ext.TradeNo)
	require.NoError(t, err)
	require.Equal(t, model.AdtyGiftFulfillmentPending, reloaded.GiftFulfillmentStatus)

	already, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	require.True(t, already)
	require.EqualValues(t, 50_000_000, userQuotaOf(t, db, customer.Id))
	require.EqualValues(t, 1, countGiftFulfillments(t, db, ext.TradeNo))
	require.EqualValues(t, 1, notifies)
}

func TestSiteGiftBonusQuotaUsesAdtyBonusUSDForSite(t *testing.T) {
	ownerQuota, err := siteGiftBonusQuota(&model.AdtyTopupOrder{
		SiteId: 1, BonusUSD: decimal.RequireFromString("10"), PromoBonusUSD: decimal.RequireFromString("5"),
		PromoFunder: model.AdtyPromoFunderSiteOwner,
	})
	require.NoError(t, err)
	require.EqualValues(t, 7_500_000, ownerQuota)

	platformQuota, err := siteGiftBonusQuota(&model.AdtyTopupOrder{
		SiteId: 1, BonusUSD: decimal.RequireFromString("10"), PromoBonusUSD: decimal.RequireFromString("5"),
		PromoFunder: model.AdtyPromoFunderPlatform,
	})
	require.NoError(t, err)
	require.EqualValues(t, 5_000_000, platformQuota)

	promoOnly, err := siteGiftBonusQuota(&model.AdtyTopupOrder{
		SiteId: 1, PromoBonusUSD: decimal.RequireFromString("5"), PromoFunder: model.AdtyPromoFunderSiteOwner,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2_500_000, promoOnly)

	plain, err := siteGiftBonusQuota(&model.AdtyTopupOrder{
		SiteId: 1, BonusUSD: decimal.RequireFromString("10"),
	})
	require.NoError(t, err)
	require.EqualValues(t, 5_000_000, plain)
}

func TestLatePayAfterCancelReacquiresOwnerPromoBonus(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGiftWithPromo(t, db, "gift-late-owner", "10", "5", model.AdtyPromoFunderSiteOwner)
	require.Equal(t, "earning+margin", ext.GiftReserveSource)
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("4")))
	require.True(t, ext.GiftReserveMargin.Equal(decimal.RequireFromString("11")))
	_, err := model.CancelAdtyTopUp(ext.TradeNo, customer.Id)
	require.NoError(t, err)
	earningOpen, marginOpen := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningOpen.FrozenUSD.IsZero())
	require.True(t, marginOpen.FrozenUSD.IsZero())

	_, err = model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.Balance.Equal(earningOpen.Balance))
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, margin.Balance.Equal(marginOpen.Balance))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("11")))
	require.EqualValues(t, 57_500_000, userQuotaOf(t, db, customer.Id))
	require.EqualValues(t, 0, countGiftFulfillments(t, db, ext.TradeNo))
	require.EqualValues(t, 2, countGiftLedgers(t, db, site.Id, "gift_reserve", ext.TradeNo, model.AdtySiteWalletEarnings))
	require.EqualValues(t, 2, countGiftLedgers(t, db, site.Id, "gift_reserve", ext.TradeNo, model.AdtySiteWalletMargin))
}

func TestLatePayAfterCancelIgnoresPlatformPromoBonus(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGiftWithPromo(t, db, "gift-late-plat", "10", "5", model.AdtyPromoFunderPlatform)
	require.Equal(t, "earning+margin", ext.GiftReserveSource)
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("4")))
	require.True(t, ext.GiftReserveMargin.Equal(decimal.RequireFromString("6")))
	_, err := model.CancelAdtyTopUp(ext.TradeNo, customer.Id)
	require.NoError(t, err)

	_, err = model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("6")))
	require.EqualValues(t, 57_500_000, userQuotaOf(t, db, customer.Id))
	require.EqualValues(t, 0, countGiftFulfillments(t, db, ext.TradeNo))
}

func TestPrepareTopUpGiftReacquiresOwnerPromoShortfallWhenHeldEqualsBonusUSD(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "gift-late-short")
	customer := siteCustomer(t, db, site, "gift-late-short-customer")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("20"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "gift-late-short-earn-20",
	})
	require.NoError(t, err)
	face := decimal.NewFromInt(100)
	bonusUSD := decimal.RequireFromString("10")
	promoUSD := decimal.RequireFromString("5")
	credit := face.Add(bonusUSD).Add(promoUSD).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart()
	tradeNo := "EPU-gift-late-short"
	ext := &model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: customer.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		FaceUSD: face, BonusRate: bonusUSD.Div(face), BonusUSD: bonusUSD, PromoBonusUSD: promoUSD,
		PromoFunder: model.AdtyPromoFunderSiteOwner, CreditQuota: credit,
		PayAmount: face, PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1),
		ExpireAt: time.Now().Unix() + 900, SiteId: site.Id,
	}
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: customer.Id, Amount: 100, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		ext,
	))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := applyAdtySiteWalletChangeTx(tx, site.Id, model.AdtySiteWalletEarnings, decimal.Zero, AdtySiteWalletChange{
			Type: "gift_reserve", RefType: "topup", RefID: tradeNo, IdempotencyKey: "gift:" + tradeNo + ":earn",
			HoldDelta: bonusUSD,
		})
		return err
	}))
	earningBefore, _ := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningBefore.FrozenUSD.Equal(bonusUSD))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		granted, err := prepareTopUpGiftOnCreditTx(tx, ext, common.TopUpStatusExpired)
		if err != nil {
			return err
		}
		require.EqualValues(t, 7_500_000, granted)
		return nil
	}))
	earning, _ := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("15")))
	require.True(t, ext.GiftReserveEarning.Equal(decimal.RequireFromString("5")))
	require.True(t, ext.GiftReserveMargin.IsZero())
	require.Equal(t, "earning", ext.GiftReserveSource)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		held, err := netGiftHoldTx(tx, site.Id, tradeNo)
		if err != nil {
			return err
		}
		require.True(t, held.Equal(decimal.RequireFromString("15")))
		return nil
	}))
}

func TestRefundPaidSiteGiftReleasesGuaranteeIdempotently(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site, _, ext := seedReservedSiteGift(t, db, "gift-refund", "10")
	earningBefore, marginBefore := mustSiteGiftWallets(t, db, site.Id)
	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	earningPaid, marginPaid := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningPaid.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, marginPaid.FrozenUSD.Equal(decimal.RequireFromString("6")))

	preview, err := model.PreviewAdtyTopUpRefund(ext.TradeNo)
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, preview.RefundQuota)
	result, err := refundAdtyTopUp(t, ext.TradeNo, "unused principal", owner.Id, "gift-refund-1")
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, result.Occupancy.CompletedQuota)

	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.Balance.Equal(earningBefore.Balance))
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, margin.Balance.Equal(marginBefore.Balance))
	require.True(t, margin.FrozenUSD.IsZero())
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings, "0", "-4")
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin, "0", "-6")
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings))

	requestUSD, err := model.RefundQuotaToUSD(preview.RefundQuota)
	require.NoError(t, err)
	again, err := completeManualRefundForTest(t, ext.TradeNo, "unused principal", owner.Id, "gift-refund-1", requestUSD)
	require.NoError(t, err)
	require.EqualValues(t, result.Occupancy.CompletedQuota, again.Occupancy.CompletedQuota)
	earningAgain, marginAgain := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earningAgain.FrozenUSD.IsZero())
	require.True(t, marginAgain.FrozenUSD.IsZero())
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings))
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin))
}

func TestRefundAfterPendingFulfillmentReturnsFullPrincipal(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site, customer, ext := seedReservedSiteGift(t, db, "gift-rf-pend", "10")
	_, err := model.CancelAdtyTopUp(ext.TradeNo, customer.Id)
	require.NoError(t, err)
	require.NoError(t, drainSiteGiftWallets(t, db, site.Id, "gift-rf-pend"))
	_, err = model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, userQuotaOf(t, db, customer.Id))

	preview, err := model.PreviewAdtyTopUpRefund(ext.TradeNo)
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, preview.RefundQuota)
	result, err := refundAdtyTopUp(t, ext.TradeNo, "pending gift refund", owner.Id, "gift-rf-pend-1")
	require.NoError(t, err)
	require.EqualValues(t, 50_000_000, result.Occupancy.CompletedQuota)
	require.EqualValues(t, 0, userQuotaOf(t, db, customer.Id))
	var row model.AdtyGiftFulfillment
	require.NoError(t, db.Where("trade_no = ?", ext.TradeNo).First(&row).Error)
	require.Equal(t, model.AdtyGiftFulfillmentCanceled, row.Status)
}

func TestConsumingSiteGiftDebitsGuaranteeOnce(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-spend", "10")
	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, customer.Id, 50_000_000, 55_000_000)
	}))
	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("4")))
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("4")))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("6")))
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("20")))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, customer.Id, 2_500_000, 5_000_000)
	}))
	earning, margin = mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("2")))
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("2")))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("3")))
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("17")))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, customer.Id, 2_500_000, 2_500_000)
	}))
	earning, margin = mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, earning.Balance.IsZero())
	require.True(t, margin.FrozenUSD.IsZero())
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("14")))
	require.EqualValues(t, 2, countGiftLedgers(t, db, site.Id, "gift_spend", ext.TradeNo, model.AdtySiteWalletEarnings))
	require.EqualValues(t, 2, countGiftLedgers(t, db, site.Id, "gift_spend", ext.TradeNo, model.AdtySiteWalletMargin))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		held, err := netGiftHoldTx(tx, site.Id, ext.TradeNo)
		if err != nil {
			return err
		}
		require.True(t, held.IsZero(), "exhausting the gift batch must not leave or re-open the original reserve")
		return nil
	}))
}

func TestNetGiftHoldIncludesGiftSpend(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-net-hold", "10")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		held, err := netGiftHoldTx(tx, site.Id, ext.TradeNo)
		if err != nil {
			return err
		}
		require.True(t, held.Equal(decimal.RequireFromString("10")))
		earning, margin, err := netGiftHoldByKindTx(tx, site.Id, ext.TradeNo)
		if err != nil {
			return err
		}
		require.True(t, earning.Equal(decimal.RequireFromString("4")))
		require.True(t, margin.Equal(decimal.RequireFromString("6")))
		return nil
	}))

	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, customer.Id, 50_000_000, 55_000_000)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, customer.Id, 2_500_000, 5_000_000)
	}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		held, err := netGiftHoldTx(tx, site.Id, ext.TradeNo)
		if err != nil {
			return err
		}
		require.True(t, held.Equal(decimal.RequireFromString("5")), "net hold must include negative gift_spend HoldDelta")
		earning, margin, err := netGiftHoldByKindTx(tx, site.Id, ext.TradeNo)
		if err != nil {
			return err
		}
		require.True(t, earning.Equal(decimal.RequireFromString("2")))
		require.True(t, margin.Equal(decimal.RequireFromString("3")))
		return nil
	}))
}

func TestRefundReleaseUsesRemainingNetHoldAfterGiftSpend(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-rf-spend", "10")
	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, customer.Id, 50_000_000, 55_000_000)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, customer.Id, 2_500_000, 5_000_000)
	}))
	earning, margin := mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("2")))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("3")))
	earnBalance, marginBalance := earning.Balance, margin.Balance

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var row model.AdtyTopupOrder
		if err := tx.Where("trade_no = ?", ext.TradeNo).First(&row).Error; err != nil {
			return err
		}
		row.RefundBonusQuota = 5_000_000
		row.RefundDeductedQuota = 5_000_000
		return releaseUnusedSiteGiftGuaranteeTx(tx, &row)
	}))

	earning, margin = mustSiteGiftWallets(t, db, site.Id)
	require.True(t, earning.FrozenUSD.IsZero())
	require.True(t, margin.FrozenUSD.IsZero())
	require.True(t, earning.Balance.Equal(earnBalance), "refund release must not re-debit already spent guarantee")
	require.True(t, margin.Balance.Equal(marginBalance), "refund release must not re-debit already spent guarantee")
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings, "0", "-2")
	assertGiftLedger(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin, "0", "-3")
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletEarnings))
	require.EqualValues(t, 1, countGiftLedgers(t, db, site.Id, "gift_release", ext.TradeNo, model.AdtySiteWalletMargin))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		held, err := netGiftHoldTx(tx, site.Id, ext.TradeNo)
		if err != nil {
			return err
		}
		require.True(t, held.IsZero())
		return nil
	}))
}

func TestQuietBonusDebitSpendsSiteGiftGuarantee(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-quiet", "10")
	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	operatingBefore := mustSiteOperatingWallet(t, db, site.Id)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesQuietTx(tx, customer.Id, 52_500_000, 55_000_000)
	}))
	requirePartialTopupGiftSpendWithoutOperatingDebit(t, db, site.Id, ext.TradeNo, operatingBefore)
}

func TestAdjustUserQuotaReduceReleasesSiteGiftGuarantee(t *testing.T) {
	db := prepareSiteDB(t)
	_, site, customer, ext := seedReservedSiteGift(t, db, "gift-adjust-sub", "10")
	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	operatingBefore := mustSiteOperatingWallet(t, db, site.Id)

	adjusted, err := model.AdjustUserQuotaWithLedger(customer.Id, 1, common.RoleRootUser, "subtract", 52_500_000, "trim bonus-backed balance", "gift-adjust-sub-1")
	require.NoError(t, err)
	require.False(t, adjusted.Replayed)
	require.Equal(t, 55_000_000, adjusted.Before)
	require.Equal(t, 2_500_000, adjusted.After)
	require.EqualValues(t, 2_500_000, userQuotaOf(t, db, customer.Id))
	requirePartialTopupGiftSpendWithoutOperatingDebit(t, db, site.Id, ext.TradeNo, operatingBefore)
}

func TestApprovedStaffDecreaseReleasesSiteGiftGuarantee(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site, customer, ext := seedReservedSiteGift(t, db, "gift-staff-dec", "10")
	staffUser := referralUser(t, db, "gift-staff-dec-staff")
	require.NoError(t, db.Create(&model.AdtySiteStaff{
		SiteId: site.Id, OwnerUserId: owner.Id, UserId: staffUser.Id, Status: "enabled", Role: "support",
	}).Error)
	_, err := model.CompleteAdtyTopUp(ext.TradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	operatingBefore := mustSiteOperatingWallet(t, db, site.Id)

	request, err := SubmitAdtyStaffBalanceRequest(staffUser.Id, customer.Id, model.AdtyStaffBalanceDecrease, decimal.RequireFromString("105"), "gift-staff-dec-1")
	require.NoError(t, err)
	require.NoError(t, ReviewAdtyStaffBalanceRequest(owner.Id, "owner", request.Id, true))
	require.EqualValues(t, 2_500_000, userQuotaOf(t, db, customer.Id))
	requirePartialTopupGiftSpendWithoutOperatingDebit(t, db, site.Id, ext.TradeNo, operatingBefore)
}

func requirePartialTopupGiftSpendWithoutOperatingDebit(t *testing.T, db *gorm.DB, siteID int, tradeNo string, operatingBefore model.AdtySiteWallet) {
	t.Helper()
	earning, margin := mustSiteGiftWallets(t, db, siteID)
	require.True(t, earning.FrozenUSD.Equal(decimal.RequireFromString("2")))
	require.True(t, earning.Balance.Equal(decimal.RequireFromString("2")))
	require.True(t, margin.FrozenUSD.Equal(decimal.RequireFromString("3")))
	require.True(t, margin.Balance.Equal(decimal.RequireFromString("17")))
	require.EqualValues(t, 1, countGiftLedgers(t, db, siteID, "gift_spend", tradeNo, model.AdtySiteWalletEarnings))
	require.EqualValues(t, 1, countGiftLedgers(t, db, siteID, "gift_spend", tradeNo, model.AdtySiteWalletMargin))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		held, err := netGiftHoldTx(tx, siteID, tradeNo)
		if err != nil {
			return err
		}
		require.True(t, held.Equal(decimal.RequireFromString("5")))
		return nil
	}))
	operating := mustSiteOperatingWallet(t, db, siteID)
	require.True(t, operating.Balance.Equal(operatingBefore.Balance), "manual or quiet reduction must not debit the operating wallet as paid usage")
	require.EqualValues(t, 0, countLedgersByType(t, db, siteID, "principal_consume"))
}

func mustSiteOperatingWallet(t *testing.T, db *gorm.DB, siteID int) model.AdtySiteWallet {
	t.Helper()
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletOperating).First(&wallet).Error)
	return wallet
}

func drainSiteGiftWallets(t *testing.T, db *gorm.DB, siteID int, prefix string) error {
	t.Helper()
	earning, margin := mustSiteGiftWallets(t, db, siteID)
	if earning.Balance.IsPositive() {
		if _, err := ApplyAdtySiteWalletChange(siteID, model.AdtySiteWalletEarnings, earning.Balance.Neg(), AdtySiteWalletChange{
			Type: "earning_settlement", IdempotencyKey: prefix + "-drain-earn",
		}); err != nil {
			return err
		}
	}
	if margin.Balance.IsPositive() {
		if _, err := ApplyAdtySiteWalletChange(siteID, model.AdtySiteWalletMargin, margin.Balance.Neg(), AdtySiteWalletChange{
			Type: "margin_return", IdempotencyKey: prefix + "-drain-margin", AllowOverdraft: true,
		}); err != nil {
			return err
		}
	}
	return nil
}

func userQuotaOf(t *testing.T, db *gorm.DB, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	return user.Quota
}

func countGiftFulfillments(t *testing.T, db *gorm.DB, tradeNo string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtyGiftFulfillment{}).Where("trade_no = ?", tradeNo).Count(&n).Error)
	return n
}

func seedReservedSiteGiftWithPromo(t *testing.T, db *gorm.DB, prefix, bonus, promoBonus, funder string) (*model.User, *model.AdtySite, *model.User, *model.AdtyTopupOrder) {
	t.Helper()
	owner, site := openActiveSite(t, db, prefix)
	customer := siteCustomer(t, db, site, prefix+"-customer")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("4"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: prefix + "-earn-4",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), prefix+"-cycle"))
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	t.Cleanup(func() { adty_setting.AuthorizeSiteBonusMarginFill(site.Id, false) })
	bonusUSD := decimal.RequireFromString(bonus)
	promoUSD := decimal.Zero
	if promoBonus != "" {
		promoUSD = decimal.RequireFromString(promoBonus)
	}
	face := decimal.NewFromInt(100)
	credit := face.Add(bonusUSD).Add(promoUSD).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart()
	tradeNo := "EPU-" + prefix
	ext := &model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: customer.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		FaceUSD: face, BonusRate: bonusUSD.Div(face), BonusUSD: bonusUSD, PromoBonusUSD: promoUSD, PromoFunder: funder,
		CreditQuota: credit, PayAmount: face, PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1),
		ExpireAt: time.Now().Unix() + 900, SiteId: site.Id,
	}
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: customer.Id, Amount: 100, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		ext,
	))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := ReserveAdtySiteGiftTx(tx, ext); err != nil {
			return err
		}
		return tx.Model(ext).Updates(map[string]any{
			"gift_reserve_source":  ext.GiftReserveSource,
			"gift_reserve_earning": ext.GiftReserveEarning,
			"gift_reserve_margin":  ext.GiftReserveMargin,
		}).Error
	}))
	return owner, site, customer, ext
}

func seedReservedSiteGift(t *testing.T, db *gorm.DB, prefix, bonus string) (*model.User, *model.AdtySite, *model.User, *model.AdtyTopupOrder) {
	t.Helper()
	owner, site := openActiveSite(t, db, prefix)
	customer := siteCustomer(t, db, site, prefix+"-customer")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("4"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: prefix + "-earn-4",
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), prefix+"-cycle"))
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	t.Cleanup(func() { adty_setting.AuthorizeSiteBonusMarginFill(site.Id, false) })
	bonusUSD := decimal.RequireFromString(bonus)
	face := decimal.NewFromInt(100)
	credit := face.Add(bonusUSD).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart()
	tradeNo := "EPU-" + prefix
	ext := &model.AdtyTopupOrder{
		TradeNo: tradeNo, UserId: customer.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		FaceUSD: face, BonusRate: bonusUSD.Div(face), BonusUSD: bonusUSD, CreditQuota: credit,
		PayAmount: face, PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1),
		ExpireAt: time.Now().Unix() + 900, SiteId: site.Id,
	}
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: customer.Id, Amount: 100, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		ext,
	))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := ReserveAdtySiteGiftTx(tx, ext); err != nil {
			return err
		}
		return tx.Model(ext).Updates(map[string]any{
			"gift_reserve_source":  ext.GiftReserveSource,
			"gift_reserve_earning": ext.GiftReserveEarning,
			"gift_reserve_margin":  ext.GiftReserveMargin,
		}).Error
	}))
	require.Equal(t, "earning+margin", ext.GiftReserveSource)
	return owner, site, customer, ext
}

func mustSiteGiftWallets(t *testing.T, db *gorm.DB, siteID int) (model.AdtySiteWallet, model.AdtySiteWallet) {
	t.Helper()
	var earning, margin model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletEarnings).First(&earning).Error)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).First(&margin).Error)
	return earning, margin
}

func assertGiftLedger(t *testing.T, db *gorm.DB, siteID int, typ, tradeNo, kind, delta, hold string) {
	t.Helper()
	var row model.AdtySiteWalletLedger
	require.NoError(t, db.Where("site_id = ? AND type = ? AND ref_id = ? AND wallet_kind = ?", siteID, typ, tradeNo, kind).First(&row).Error)
	require.True(t, row.Delta.Equal(decimal.RequireFromString(delta)), "delta %s want %s", row.Delta, delta)
	require.True(t, row.HoldDelta.Equal(decimal.RequireFromString(hold)), "hold %s want %s", row.HoldDelta, hold)
}

func countGiftLedgers(t *testing.T, db *gorm.DB, siteID int, typ, tradeNo, kind string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ? AND type = ? AND ref_id = ? AND wallet_kind = ?", siteID, typ, tradeNo, kind).Count(&n).Error)
	return n
}

func countLedgersByType(t *testing.T, db *gorm.DB, siteID int, typ string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtySiteWalletLedger{}).Where("site_id = ? AND type = ?", siteID, typ).Count(&n).Error)
	return n
}
