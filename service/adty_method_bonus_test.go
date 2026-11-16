package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestQuoteUsesPaymentMethodRateNotAmountTier(t *testing.T) {
	setAdtyQuoteDefaults(t)
	setAdtyBonusRates(t, map[int]float64{100: 50})
	SetAdtyMethodBonusForTest(t, "usdt", true, "0.025")

	quote, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	require.Equal(t, "2.5", quote.BonusUSD.String())
	require.EqualValues(t, 51_250_000, quote.CreditQuota)
	require.True(t, quote.BonusRate.Equal(decimal.RequireFromString("0.025")))
}

func TestAmountTierDoesNotCreateBonus(t *testing.T) {
	setAdtyQuoteDefaults(t)
	setAdtyBonusRates(t, map[int]float64{10: 8})
	ClearAdtyMethodBonusForTest(t)

	quote, err := AdtyQuoteFor("default", 100, AdtyMethodAlipay, AdtyPromoInput{})
	require.NoError(t, err)
	require.True(t, quote.BonusUSD.IsZero())
	require.EqualValues(t, 50_000_000, quote.CreditQuota)
}

func TestSiteGiftQuoteRequiresEarningReserve(t *testing.T) {
	db := prepareSiteDB(t)
	setAdtyQuoteDefaults(t)
	// 主站 100 档 8% + max_site_bonus_extra 2 = 上限 10%，让 10% 这一档不撞策略上限，
	// 用例断言的是「收益不够垫这笔加赠」，不是上限。
	setAdtyBonusRates(t, map[int]float64{100: 8})
	_, site := openActiveSite(t, db, "gift-reserve")
	restoreSiteMethodSettings(t)
	setting := adty_setting.GetTopupSetting()
	setting.SiteMethodAuth = map[int][]string{site.Id: {"usdt"}}
	setting.SiteMethodBonus = map[int]map[string]adty_setting.MethodBonusRule{
		site.Id: {"usdt": {Enabled: true, Rate: "0.10"}},
	}
	_, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not covered", "上限不是这条用例要挡的东西")
}

func TestSameMethodSharesBonusAcrossChains(t *testing.T) {
	setAdtyQuoteDefaults(t)
	setMainBonusForTest(t, "usdt", true, "0.02")
	t.Cleanup(func() { setMainBonusForTest(t, "usdt", false, "0") })
	trc, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	bep, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTBEP20, AdtyPromoInput{})
	require.NoError(t, err)
	require.True(t, trc.BonusUSD.Equal(bep.BonusUSD))
	require.Equal(t, "2", trc.BonusUSD.String())
}

func TestSiteGiftCanUseAuthorizedMarginFill(t *testing.T) {
	db := prepareSiteDB(t)
	setAdtyQuoteDefaults(t)
	// 上限 10%（主站 100 档 8% + extra 2），用例断言的是保证金补位，不是上限。
	setAdtyBonusRates(t, map[int]float64{100: 8})
	restoreSiteMethodSettings(t)
	_, site := openActiveSite(t, db, "gift-margin")
	authorizeSiteBonusForTest(t, site.Id, "usdt", true)
	setSiteBonusForTest(t, site.Id, "usdt", true, "0.10")
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "gift-margin-cycle"))
	quote, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	require.Equal(t, "10", quote.BonusUSD.String())
}

// 上限是「面额 ×（主站同档比例 + max_site_bonus_extra）÷ 100」。prepareSiteDB 把
// max_site_bonus_extra 设成 2，所以主站 100 档 2.5% 时 $100 的分站订单最多给 $4.5 加赠。
func TestSiteBonusAboveCapIsRefusedAtQuote(t *testing.T) {
	db := prepareSiteDB(t)
	setAdtyQuoteDefaults(t)
	restoreSiteMethodSettings(t)
	setAdtyBonusRates(t, map[int]float64{100: 2.5})
	_, site := openActiveSite(t, db, "gift-over-cap")
	authorizeSiteBonusForTest(t, site.Id, "usdt", true)
	setSiteBonusForTest(t, site.Id, "usdt", true, "0.05") // $100 × 5% = $5 > $4.5

	_, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "4.5", "报错要点出这一单的上限，运维才知道改哪一档")
}

func TestSiteBonusAtCapIsQuoted(t *testing.T) {
	db := prepareSiteDB(t)
	setAdtyQuoteDefaults(t)
	restoreSiteMethodSettings(t)
	setAdtyBonusRates(t, map[int]float64{100: 2.5})
	_, site := openActiveSite(t, db, "gift-at-cap")
	authorizeSiteBonusForTest(t, site.Id, "usdt", true)
	setSiteBonusForTest(t, site.Id, "usdt", true, "0.045") // 恰好等于 $4.5，不算超
	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "gift-at-cap-cycle"))

	quote, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	require.Equal(t, "4.5", quote.BonusUSD.String())
}

func TestPromoCodeIsRetiredOnQuote(t *testing.T) {
	setAdtyQuoteDefaults(t)
	_, err := AdtyQuoteFor("default", 100, AdtyMethodAlipay, AdtyPromoInput{Code: "SAVE10"})
	require.Error(t, err)
}

// 加赠配置现在只有一条写入口：以事务内加锁读到的当前值为基准合并，再由配置层
// 同步进内存。测试预置配置也走这两步，避免出现只改内存的旁路；这里的基准取内存
// 里的现值，相当于测试自己扮演那个「当前值」。
func currentBonusJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func setMainBonusForTest(t *testing.T, method string, enabled bool, rate string) {
	t.Helper()
	base := currentBonusJSON(t, adty_setting.GetTopupSetting().MethodBonus)
	value, err := adty_setting.MergeMainMethodBonus(base, method, enabled, rate)
	require.NoError(t, err)
	require.NoError(t, adty_setting.ApplyMethodBonusOption(adty_setting.TopupSettingKeyMethodBonus, value))
}

func authorizeSiteBonusForTest(t *testing.T, siteID int, method string, allowed bool) {
	t.Helper()
	base := currentBonusJSON(t, adty_setting.GetTopupSetting().SiteMethodAuth)
	value, err := adty_setting.MergeSiteMethodAuth(base, siteID, method, allowed)
	require.NoError(t, err)
	require.NoError(t, adty_setting.ApplyMethodBonusOption(adty_setting.TopupSettingKeySiteMethodAuth, value))
}

// restoreSiteMethodSettings 把站点的授权/加赠规则在用例结束时还原：这两张表是进程级
// 全局配置，而每个用例的站点 id 都从 1 开始，不还原就会串到下一个用例。
func restoreSiteMethodSettings(t *testing.T) {
	t.Helper()
	setting := adty_setting.GetTopupSetting()
	oldAuth, oldRules := setting.SiteMethodAuth, setting.SiteMethodBonus
	t.Cleanup(func() {
		setting.SiteMethodAuth, setting.SiteMethodBonus = oldAuth, oldRules
	})
}

func setSiteBonusForTest(t *testing.T, siteID int, method string, enabled bool, rate string) {
	t.Helper()
	base := currentBonusJSON(t, adty_setting.GetTopupSetting().SiteMethodBonus)
	value, err := adty_setting.MergeSiteMethodBonus(base, siteID, method, enabled, rate)
	require.NoError(t, err)
	require.NoError(t, adty_setting.ApplyMethodBonusOption(adty_setting.TopupSettingKeySiteMethodBonus, value))
}
