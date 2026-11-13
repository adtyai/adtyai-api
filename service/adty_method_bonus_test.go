package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestQuoteUsesPaymentMethodRateNotAmountTier(t *testing.T) {
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, map[int]float64{100: 50})
	SetAdtyMethodBonusForTest(t, "usdt", true, "0.025")

	quote, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	require.Equal(t, "2.5", quote.BonusUSD.String())
	require.EqualValues(t, 51_250_000, quote.CreditQuota)
	require.True(t, quote.BonusRate.Equal(decimal.RequireFromString("0.025")))
}

func TestAmountTierDoesNotCreateBonus(t *testing.T) {
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, map[int]float64{10: 8})
	ClearAdtyMethodBonusForTest(t)

	quote, err := AdtyQuoteFor("default", 100, AdtyMethodAlipay, AdtyPromoInput{})
	require.NoError(t, err)
	require.True(t, quote.BonusUSD.IsZero())
	require.EqualValues(t, 50_000_000, quote.CreditQuota)
}

func TestSiteGiftQuoteRequiresEarningReserve(t *testing.T) {
	db := prepareSiteDB(t)
	confirmAdtyCompliance(t)
	_, site := openActiveSite(t, db, "gift-reserve")
	setting := adty_setting.GetTopupSetting()
	oldAuth, oldRules := setting.SiteMethodAuth, setting.SiteMethodBonus
	t.Cleanup(func() {
		setting.SiteMethodAuth, setting.SiteMethodBonus = oldAuth, oldRules
	})
	setting.SiteMethodAuth = map[int][]string{site.Id: {"usdt"}}
	setting.SiteMethodBonus = map[int]map[string]adty_setting.MethodBonusRule{
		site.Id: {"usdt": {Enabled: true, Rate: "0.10"}},
	}
	_, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.Error(t, err)
}

func TestSameMethodSharesBonusAcrossChains(t *testing.T) {
	confirmAdtyCompliance(t)
	require.NoError(t, adty_setting.SetMainMethodBonus("usdt", true, "0.02"))
	t.Cleanup(func() { _ = adty_setting.SetMainMethodBonus("usdt", false, "0") })
	trc, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	bep, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTBEP20, AdtyPromoInput{})
	require.NoError(t, err)
	require.True(t, trc.BonusUSD.Equal(bep.BonusUSD))
	require.Equal(t, "2", trc.BonusUSD.String())
}

func TestSiteGiftCanUseAuthorizedMarginFill(t *testing.T) {
	db := prepareSiteDB(t)
	confirmAdtyCompliance(t)
	_, site := openActiveSite(t, db, "gift-margin")
	require.NoError(t, adty_setting.AuthorizeSiteBonusMethod(site.Id, "usdt", true))
	require.NoError(t, adty_setting.SetSiteMethodBonus(site.Id, "usdt", true, "0.10"))
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "gift-margin-cycle"))
	quote, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	require.Equal(t, "10", quote.BonusUSD.String())
}

func TestPromoCodeIsRetiredOnQuote(t *testing.T) {
	confirmAdtyCompliance(t)
	_, err := AdtyQuoteFor("default", 100, AdtyMethodAlipay, AdtyPromoInput{Code: "SAVE10"})
	require.Error(t, err)
}
