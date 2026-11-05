package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 站长收益：记账、加赠扣减、试用额度、风控、结算（任务 11.5）。

func usd(value string) decimal.Decimal { return decimal.RequireFromString(value) }

func perUnit(amount string) int64 {
	return usd(amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
}

// earningRows 取一个站的收益行，按类型汇总。
func earningTotals(t *testing.T, db *gorm.DB, siteId int) map[string]decimal.Decimal {
	t.Helper()
	var rows []model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ?", siteId).Find(&rows).Error)
	totals := map[string]decimal.Decimal{}
	for _, row := range rows {
		totals[row.Kind] = totals[row.Kind].Add(row.Earning)
	}
	return totals
}

// consumeOnSite 模拟一次扣费：乘 k 之后写消费日志（钩子从 gin 上下文取 k、d）。
func consumeOnSite(t *testing.T, user *model.User, quota int64) {
	t.Helper()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	AdtyApplySiteMarkup(ctx, user.Id, "default", 1)
	model.RecordConsumeLog(ctx, user.Id, model.RecordConsumeLogParams{ModelName: "m", Quota: int(quota), Group: "default"})
}

func TestEarningSplitExact(t *testing.T) {
	prepareSiteDB(t)
	revenue, platform, earning := AdtySiteEarningSplit(perUnit("125"), usd("1.25"), usd("0.8"))
	assert.True(t, revenue.Equal(usd("125")), "revenue %s", revenue)
	assert.True(t, platform.Equal(usd("80")), "platform %s", platform)
	assert.True(t, earning.Equal(usd("45")), "earning %s", earning)
	assert.True(t, platform.Add(earning).Equal(revenue))

	// 除不尽时两项相加仍然等于客户付的钱。
	revenue, platform, earning = AdtySiteEarningSplit(1_234_567, usd("1.3"), usd("0.77"))
	assert.True(t, platform.Add(earning).Equal(revenue))
}

func TestEarningUsesLoggedSnapshot(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "snapshot")
	customer := siteCustomer(t, db, site, "snapshot-customer")
	sitePrice(t, db, site.Id, 0, "", "1.25")
	consumeOnSite(t, customer, perUnit("125"))
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningUsage].Equal(usd("45")))

	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", usd("2")).Error)
	_, err := AdjustAdtySite(site.Id, 1, AdtySiteAdjust{CostFactor: dec("0.5")})
	require.NoError(t, err)
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningUsage].Equal(usd("45")), "already recorded earnings do not change")

	consumeOnSite(t, customer, perUnit("100"))
	// 新的 k 2、d 0.5：$100 × (2 − 0.5) ÷ 2 = $75。
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningUsage].Equal(usd("120")))
}

// siteTask 建一个提交时带分站快照的任务。
func siteTask(t *testing.T, db *gorm.DB, user *model.User, site *model.AdtySite, quota int, k, d float64) *model.Task {
	t.Helper()
	task := &model.Task{TaskID: fmt.Sprintf("site-task-%d-%d", user.Id, time.Now().UnixNano()), UserId: user.Id, Group: "default", Quota: quota, Status: model.TaskStatusFailure}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "earning-task", GroupRatio: k, SiteMarkup: k, SiteCostFactor: d, SiteId: site.Id}
	require.NoError(t, db.Create(task).Error)
	return task
}

func TestMidjourneyRefundUsesSubmitSnapshot(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}, &model.Channel{}))
	_, site := openActiveSite(t, db, "mj-refund")
	customer := siteCustomer(t, db, site, "mj-refund-customer")
	sitePrice(t, db, site.Id, 0, "", "3")
	task := &model.Midjourney{
		UserId: customer.Id, MjId: "mj-snap", Action: "IMAGINE", Status: "FAILURE",
		Quota: int(perUnit("125")), BillingSiteId: site.Id, BillingSiteMarkup: 1.25, BillingSiteCostFactor: 0.8,
	}
	require.NoError(t, db.Create(task).Error)
	require.True(t, RefundMidjourneyQuota(context.Background(), task, "upstream failed"))
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningRefund].Equal(usd("-45")), "refund keeps the submit snapshot, not the current k of 3")
}

func TestTaskRefundReversesEarningWithSnapshot(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Token{}, &model.Channel{}))
	_, site := openActiveSite(t, db, "task-refund")
	customer := siteCustomer(t, db, site, "task-refund-customer")
	sitePrice(t, db, site.Id, 0, "", "3")
	task := siteTask(t, db, customer, site, int(perUnit("125")), 1.25, 0.8)
	require.True(t, RefundTaskQuota(context.Background(), task, "upstream failed"))
	// 快照 k 1.25、d 0.8，不按现在的 k 3：退 $125 → 站长 −$45。
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningRefund].Equal(usd("-45")))
}

func TestNegativeTaskDeltaReversesEarning(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Token{}, &model.Channel{}))
	_, site := openActiveSite(t, db, "task-delta")
	customer := siteCustomer(t, db, site, "task-delta-customer")
	task := siteTask(t, db, customer, site, int(perUnit("125")), 1.25, 0.8)
	RecalculateTaskQuota(context.Background(), task, int(perUnit("100")), common.NewMessage("adjusted"))
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningRefund].Equal(usd("-9")), "$25 back × 0.36 = $9")
	RecalculateTaskQuota(context.Background(), task, int(perUnit("112.5")), common.NewMessage("adjusted"))
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningUsage].Equal(usd("4.5")), "a positive delta earns")
}

func TestEarningRecordedWhenConsumeLogDisabled(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "no-log")
	customer := siteCustomer(t, db, site, "no-log-customer")
	sitePrice(t, db, site.Id, 0, "", "1.25")
	old := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = old })
	consumeOnSite(t, customer, perUnit("125"))
	var logs int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ?", customer.Id).Count(&logs).Error)
	assert.Zero(t, logs)
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningUsage].Equal(usd("45")))
}

func TestChannelTestNotCounted(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "chantest")
	customer := siteCustomer(t, db, site, "chantest-customer")
	sitePrice(t, db, site.Id, 0, "", "1.25")
	// 渠道测试不经过乘 k（relay/helper.HandleGroupRatio 跳过 IsChannelTest），上下文里没有 k。
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/channel/test/1", nil)
	model.RecordConsumeLog(ctx, customer.Id, model.RecordConsumeLogParams{ModelName: "m", Quota: int(perUnit("10")), TokenName: "Model test"})
	assert.Empty(t, earningTotals(t, db, site.Id))
}

func TestMainSiteCustomerNoEarning(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "main-only")
	mainUser := referralUser(t, db, "main-only-customer")
	sitePrice(t, db, site.Id, 0, "", "1.25")
	consumeOnSite(t, mainUser, perUnit("100"))
	var rows int64
	require.NoError(t, db.Model(&model.AdtySiteEarningDaily{}).Count(&rows).Error)
	assert.Zero(t, rows)
}

// siteTopUp 建一笔分站客户的订单并入账，加赠比例 bonusRate%。
func siteTopUp(t *testing.T, user *model.User, site *model.AdtySite, tradeNo string, face int64, bonusRate string) {
	t.Helper()
	bonus := decimal.NewFromInt(face).Mul(usd(bonusRate)).Div(decimal.NewFromInt(100))
	credit := decimal.NewFromInt(face).Add(bonus).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart()
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: user.Id, Amount: face, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: user.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
			FaceUSD: decimal.NewFromInt(face), BonusRate: usd(bonusRate), BonusUSD: bonus, CreditQuota: credit,
			PayAmount: decimal.NewFromInt(face), PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1),
			ExpireAt: time.Now().Unix() + 900, SiteId: site.Id},
	))
	_, err := model.CompleteAdtyTopUp(tradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(face), FiatAmount: decimal.NewFromInt(face),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
}

func TestSiteBonusDeductedFromEarning(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "bonus")
	customer := siteCustomer(t, db, site, "bonus-customer")
	siteTopUp(t, customer, site, "EPU-site-bonus", 100, "5")
	var stored model.User
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.EqualValues(t, perUnit("105"), stored.Quota, "pays $100, gets $105")
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningBonus].Equal(usd("-5")))

	consumeOnSite(t, customer, perUnit("105")) // k 1（没设定价）
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningUsage].Equal(usd("21")), "$105 × (1 − 0.8)")
	assert.True(t, totals[model.AdtySiteEarningUsage].Add(totals[model.AdtySiteEarningBonus]).Equal(usd("16")))
	var platform struct{ Total decimal.Decimal }
	require.NoError(t, db.Model(&model.AdtySiteEarningDaily{}).Select("COALESCE(SUM(platform),0) AS total").Where("site_id = ?", site.Id).Scan(&platform).Error)
	assert.True(t, platform.Total.Equal(usd("84")), "the platform keeps exactly $105 × 0.8")
}

func TestTopupRefundReversesBonusDeduction(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "bonus-refund")
	customer := siteCustomer(t, db, site, "bonus-refund-customer")
	siteTopUp(t, customer, site, "EPU-site-refund", 100, "5")
	_, err := refundAdtyTopUp(t, "EPU-site-refund", "chargeback", 1, "refund-site-1")
	require.NoError(t, err)
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningBonus].Equal(usd("-5")))
	assert.True(t, totals[model.AdtySiteEarningBonusReversal].Equal(usd("5")))
}

// 赠金先花掉一部分再退款：按「先花本金」，收回的赠金是 min(G, R)，站长只拿回这部分。
// 付 $100、站长送 $10（10%），花掉 $105，余额 $5：R = $5，只收回 $5 赠金，还给站长 $5，不是 $10。
func TestTopupRefundReturnsOnlyReclaimedOwnerBonus(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "bonus-spent")
	customer := siteCustomer(t, db, site, "bonus-spent-customer")
	siteTopUp(t, customer, site, "EPU-site-bonus-spent", 100, "10")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", perUnit("5")).Error)

	result, err := refundAdtyTopUp(t, "EPU-site-bonus-spent", "chargeback", 1, "refund-site-bonus-spent")
	require.NoError(t, err)
	assert.EqualValues(t, perUnit("5"), result.DeductedQuota)
	assert.Zero(t, result.RefundQuota)
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningBonus].Equal(usd("-10")))
	assert.True(t, totals[model.AdtySiteEarningBonusReversal].Equal(usd("5")), "only the reclaimed $5 of the owner's $10")
}

// 赠金一部分站长出、一部分平台码出：只按站长出资额乘收回比例还给站长，平台码的赠金不还给任何人。
// 付 $100，站长加赠 $10、平台优惠码 $10（G = $20），余额 $10：收回赠金 $10，比例 1/2，还给站长 $5。
func TestTopupRefundScalesOnlyOwnerFundedBonus(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "bonus-mixed")
	customer := siteCustomer(t, db, site, "bonus-mixed-customer")
	tradeNo := "EPU-site-bonus-mixed"
	credit := perUnit("120")
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: customer.Id, Amount: 100, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: customer.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
			FaceUSD: decimal.NewFromInt(100), BonusRate: usd("10"), BonusUSD: usd("10"), PromoBonusUSD: usd("10"),
			PromoFunder: model.AdtyPromoFunderPlatform, CreditQuota: int64(credit),
			PayAmount: decimal.NewFromInt(100), PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1),
			ExpireAt: time.Now().Unix() + 900, SiteId: site.Id},
	))
	_, err := model.CompleteAdtyTopUp(tradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", perUnit("10")).Error)

	_, err = refundAdtyTopUp(t, tradeNo, "chargeback", 1, "refund-site-bonus-mixed")
	require.NoError(t, err)
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningBonus].Equal(usd("-10")), "the owner paid only the site tier bonus")
	assert.True(t, totals[model.AdtySiteEarningBonusReversal].Equal(usd("5")), "$10 owner bonus × $10 reclaimed ÷ $20 bonus")
}

// 客户花掉一部分后退款：赠金没花到（余额 ≥ 赠金），站长出的加赠全额还给站长，已经花掉部分的消费分成不扣。
func TestPartialTopupRefundKeepsUsageEarning(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "partial-refund")
	customer := siteCustomer(t, db, site, "partial-refund-customer")
	siteTopUp(t, customer, site, "EPU-site-partial", 100, "5")
	consumeOnSite(t, customer, perUnit("30")) // k 1、d 0.8：站长 +$6
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", perUnit("75")).Error)

	result, err := refundAdtyTopUp(t, "EPU-site-partial", "chargeback", 1, "refund-site-partial")
	require.NoError(t, err)
	assert.EqualValues(t, perUnit("75"), result.DeductedQuota)
	assert.EqualValues(t, perUnit("70"), result.RefundQuota, "$75 left − $5 bonus")
	assert.True(t, result.RefundAmount.Equal(usd("70")))
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningBonusReversal].Equal(usd("5")))
	assert.True(t, totals[model.AdtySiteEarningUsage].Equal(usd("6")), "earnings from spent credit stay")
	assert.True(t, totals[model.AdtySiteEarningRefund].IsZero())
}

func TestTopUpStatsKeepUnrefundedFace(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "stats-refund")
	partial := siteCustomer(t, db, site, "stats-partial-customer")
	siteTopUp(t, partial, site, "EPU-stats-partial", 100, "5")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", partial.Id).Update("quota", perUnit("75")).Error)
	partialRefund, err := refundAdtyTopUp(t, "EPU-stats-partial", "chargeback", 1, "refund-stats-partial")
	require.NoError(t, err)
	assert.EqualValues(t, perUnit("70"), partialRefund.RefundQuota)

	today := time.Now().Format("2006-01-02")
	stats, err := GetAdtySiteStats(site.Id, today, today)
	require.NoError(t, err)
	assert.True(t, stats.Total.TopUpUSD.Equal(usd("30")), "partial refund kept %s", stats.Total.TopUpUSD)
	assert.EqualValues(t, 1, stats.Total.PaidCustomers)

	spent := siteCustomer(t, db, site, "stats-zero-customer")
	siteTopUp(t, spent, site, "EPU-stats-zero", 100, "10")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", spent.Id).Update("quota", perUnit("5")).Error)
	zeroRefund, err := refundAdtyTopUp(t, "EPU-stats-zero", "chargeback", 1, "refund-stats-zero")
	require.NoError(t, err)
	assert.Zero(t, zeroRefund.RefundQuota)
	stats, err = GetAdtySiteStats(site.Id, today, today)
	require.NoError(t, err)
	assert.True(t, stats.Total.TopUpUSD.Equal(usd("130")), "zero principal refund kept %s", stats.Total.TopUpUSD)
	assert.EqualValues(t, 2, stats.Total.PaidCustomers)

	full := siteCustomer(t, db, site, "stats-full-customer")
	siteTopUp(t, full, site, "EPU-stats-full", 50, "0")
	fullRefund, err := refundAdtyTopUp(t, "EPU-stats-full", "chargeback", 1, "refund-stats-full")
	require.NoError(t, err)
	assert.EqualValues(t, perUnit("50"), fullRefund.RefundQuota)
	stats, err = GetAdtySiteStats(site.Id, today, today)
	require.NoError(t, err)
	assert.True(t, stats.Total.TopUpUSD.Equal(usd("130")), "full refund kept %s", stats.Total.TopUpUSD)
	assert.EqualValues(t, 2, stats.Total.PaidCustomers)

	month, err := AdtySitesMonthTotals([]int{site.Id}, time.Now())
	require.NoError(t, err)
	assert.True(t, month[site.Id].TopUpUSD.Equal(usd("130")), "month %s", month[site.Id].TopUpUSD)
	overview, err := GetAdtySitesOverview(today, today)
	require.NoError(t, err)
	assert.True(t, overview.SiteTopUpUSD.Equal(usd("130")), "overview %s", overview.SiteTopUpUSD)
}

func TestSiteBonusCappedByMainPlusExtra(t *testing.T) {
	db := prepareSiteDB(t)
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, map[int]float64{100: 2.5, 500: 4})
	_, site := openActiveSite(t, db, "bonus-cap")
	// max_site_bonus_extra 是 2：$100 档最多 4.5%，$300 落在 $100 档，$500 档最多 6%，$50 没有主站档，最多 2%。
	assert.NoError(t, ValidateAdtySiteBonusRates(map[int]float64{100: 4.5, 300: 4.5, 500: 6, 50: 2}))
	assert.Error(t, ValidateAdtySiteBonusRates(map[int]float64{100: 4.6}))
	assert.Error(t, ValidateAdtySiteBonusRates(map[int]float64{50: 2.1}))
	assert.Error(t, ValidateAdtySiteBonusRates(map[int]float64{500: 6.5}))

	require.NoError(t, SaveAdtySiteBonusRates(site.Id, map[int]float64{100: 4}))
	quote, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 200, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	assert.True(t, quote.BonusRate.Equal(usd("4")), "site customers use the site's tiers")
	quote, err = AdtyQuoteFor("default", 200, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	assert.True(t, quote.BonusRate.Equal(usd("2.5")))

	require.NoError(t, SaveAdtySiteBonusRates(site.Id, map[int]float64{100: 4.5}))
	setAdtyBonusRates(t, map[int]float64{100: 1})
	capped, err := AdtyQuoteForSite(AdtySiteById(site.Id), "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{})
	require.NoError(t, err)
	assert.True(t, capped.BonusUSD.Equal(usd("3")), "order time recap uses the current main tier plus extra, not the saved 4.5")
}

func TestTrialGrantLimitsAndLedger(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "trial")
	customer := siteCustomer(t, db, site, "trial-customer")
	stranger := referralUser(t, db, "trial-stranger")

	_, err := GrantAdtySiteTrial(site.Id, customer.Id, usd("5.01"), "k0")
	assert.Error(t, err, "above trial_grant_max_usd")
	_, err = GrantAdtySiteTrial(site.Id, stranger.Id, usd("1"), "k1")
	assert.Error(t, err, "not this site's customer")
	_, err = GrantAdtySiteTrial(site.Id, customer.Id, usd("1"), "broke")
	assert.Error(t, err, "zero withdrawable commission cannot fund a trial")

	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ReferralCommissionAccount{}).Where("affiliate_id = ?", affiliate.Id).Update("available_amount", 100).Error)

	grant, err := GrantAdtySiteTrial(site.Id, customer.Id, usd("5"), "k2")
	require.NoError(t, err)
	again, err := GrantAdtySiteTrial(site.Id, customer.Id, usd("5"), "k2")
	require.NoError(t, err)
	assert.Equal(t, grant.Id, again.Id, "replay is idempotent")
	_, err = GrantAdtySiteTrial(site.Id, customer.Id, usd("4"), "k2")
	assert.ErrorIs(t, err, model.ErrAdtyIdempotencyConflict)
	assert.InDelta(t, 95, account(t, db, affiliate.Id).AvailableAmount, 1e-9)

	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusPaused).Error)
	paused, err := GrantAdtySiteTrial(site.Id, customer.Id, usd("5"), "k2")
	require.NoError(t, err)
	assert.Equal(t, grant.Id, paused.Id, "a paused site still replays the same grant")
	_, err = GrantAdtySiteTrial(site.Id, customer.Id, usd("1"), "while-paused")
	assert.Error(t, err, "a new grant needs an active site")
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusActive).Error)

	var stored model.User
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.EqualValues(t, perUnit("5"), stored.Quota)
	var ledger model.AdtyBalanceLedger
	require.NoError(t, db.Where("user_id = ? AND type = ?", customer.Id, model.AdtyLedgerTypeSiteTrial).First(&ledger).Error)
	assert.EqualValues(t, perUnit("5"), ledger.DeltaQuota)
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningTrial].Equal(usd("-5")))
	var trialRow model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningTrial).First(&trialRow).Error)
	assert.Equal(t, model.AdtySiteEarningAvailable, trialRow.Status)
	assert.NotZero(t, trialRow.SettledAt)

	for i := range 3 {
		_, err := GrantAdtySiteTrial(site.Id, customer.Id, usd("5"), fmt.Sprintf("more-%d", i))
		require.NoError(t, err)
	}
	_, err = GrantAdtySiteTrial(site.Id, customer.Id, usd("0.01"), "over-daily")
	assert.Error(t, err, "trial_grant_daily_usd of $20 is used up")

	resellerSetting().TrialGrantMaxUSD = nil
	_, err = GrantAdtySiteTrial(site.Id, customer.Id, usd("1"), "unset")
	assert.ErrorIs(t, err, ErrAdtySiteTrialUnavailable)
}

func TestTrialGrantConcurrentSameKey(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "trial-race")
	customer := siteCustomer(t, db, site, "trial-race-customer")
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ReferralCommissionAccount{}).Where("affiliate_id = ?", affiliate.Id).Update("available_amount", 20).Error)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := make([]int64, 0, 2)
	errs := make([]error, 0)
	for range 2 {
		wg.Go(func() {
			grant, err := GrantAdtySiteTrial(site.Id, customer.Id, usd("5"), "same")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids = append(ids, grant.Id)
		})
	}
	wg.Wait()
	require.Empty(t, errs)
	require.Len(t, ids, 2)
	assert.Equal(t, ids[0], ids[1])
	assert.InDelta(t, 15, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
	var stored model.User
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.EqualValues(t, perUnit("5"), stored.Quota)
}

// registerSiteCustomer 模拟分站注册：带 IP 和设备 cookie，走 AdtyAfterRegister。
func registerSiteCustomer(t *testing.T, db *gorm.DB, site *model.AdtySite, name, ip, device string) *model.User {
	t.Helper()
	user := referralUser(t, db, name)
	require.NoError(t, model.AdtySetUserSiteTx(db, user.Id, site.Id))
	user.SiteId = site.Id
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/user/register", nil)
	ctx.Request.RemoteAddr = ip + ":1234"
	ctx.Request.Header.Set("Cookie", AdtyDeviceCookie+"="+device)
	AdtyAfterRegister(ctx, user)
	return user
}

func recordFingerprint(t *testing.T, db *gorm.DB, userId int, ip, device string) {
	t.Helper()
	require.NoError(t, db.Create(&model.AdtyUserFingerprint{UserId: userId, IpHash: HashReferralRiskValue(ip), DeviceHash: HashReferralRiskValue(device), Kind: "login", CreatedAt: time.Now().Unix()}).Error)
}

func TestSelfUseFlagSkipsEarning(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "selfuse")
	recordFingerprint(t, db, owner.Id, "203.0.113.7", "owner-device")
	sitePrice(t, db, site.Id, 0, "", "1.25")
	customer := registerSiteCustomer(t, db, site, "selfuse-customer", "203.0.113.7", "other-device")
	var row model.AdtySiteCustomer
	require.NoError(t, db.First(&row, customer.Id).Error)
	assert.Equal(t, model.AdtySiteRiskSelfUse, row.RiskFlag)

	consumeOnSite(t, customer, perUnit("125"))
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningSkipped].Equal(usd("45")))
	assert.True(t, totals[model.AdtySiteEarningUsage].IsZero())
	var skipped model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningSkipped).First(&skipped).Error)
	assert.Equal(t, model.AdtySiteRiskSelfUse, skipped.Reason)

	clean := registerSiteCustomer(t, db, site, "clean-customer", "198.51.100.1", "fresh-device")
	var cleanRow model.AdtySiteCustomer
	require.NoError(t, db.First(&cleanRow, clean.Id).Error)
	assert.Empty(t, cleanRow.RiskFlag)
}

func TestMigrationFlagSkipsEarning(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "migrate")
	payer := referralUser(t, db, "main-payer")
	require.NoError(t, db.Create(&model.TopUp{UserId: payer.Id, Amount: 10, TradeNo: "MAIN-PAID", Status: common.TopUpStatusSuccess}).Error)
	recordFingerprint(t, db, payer.Id, "192.0.2.10", "payer-device")
	customer := registerSiteCustomer(t, db, site, "migrated-customer", "192.0.2.99", "payer-device")
	var row model.AdtySiteCustomer
	require.NoError(t, db.First(&row, customer.Id).Error)
	assert.Equal(t, model.AdtySiteRiskMigration, row.RiskFlag)
	consumeOnSite(t, customer, perUnit("10"))
	assert.False(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningSkipped].IsZero())

	resellerSetting().MigrationCheckEnabled = false
	other := registerSiteCustomer(t, db, site, "migrated-unchecked", "192.0.2.98", "payer-device")
	var otherRow model.AdtySiteCustomer
	require.NoError(t, db.First(&otherRow, other.Id).Error)
	assert.Empty(t, otherRow.RiskFlag, "the check can be switched off")
}

func TestReviewedCustomerEarns(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "reviewed")
	recordFingerprint(t, db, owner.Id, "203.0.113.8", "owner-device-2")
	customer := registerSiteCustomer(t, db, site, "reviewed-customer", "203.0.113.8", "x")
	consumeOnSite(t, customer, perUnit("10"))
	require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, true))
	consumeOnSite(t, customer, perUnit("10"))
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningSkipped].IsZero(), "pending skipped rows were converted")
	assert.True(t, totals[model.AdtySiteEarningUsage].Equal(usd("4")), "$20 × (1 − 0.8)")
}

func TestSettleAfterFreezeIntoAccount(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "settle")
	customer := siteCustomer(t, db, site, "settle-customer")
	sitePrice(t, db, site.Id, 0, "", "1.25")
	saved := adtySiteEarningDayFn
	t.Cleanup(func() { adtySiteEarningDayFn = saved })
	adtySiteEarningDayFn = func() string { return "2026-10-01" }
	consumeOnSite(t, customer, perUnit("125"))
	adtySiteEarningDayFn = func() string { return "2026-10-05" }
	consumeOnSite(t, customer, perUnit("12.5"))

	day := func(value string) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02 15:04", value, time.Local)
		require.NoError(t, err)
		return parsed
	}
	settled, err := RunAdtySiteSettlementOnce(day("2026-10-07 04:00"))
	require.NoError(t, err)
	assert.Zero(t, settled, "freeze_days 7 not reached")

	settled, err = RunAdtySiteSettlementOnce(day("2026-10-08 04:00"))
	require.NoError(t, err)
	assert.Equal(t, 1, settled)
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	acc := account(t, db, affiliate.Id)
	assert.InDelta(t, 45, acc.AvailableAmount, 1e-9)
	var ledger model.ReferralCommissionLedger
	require.NoError(t, db.Where("affiliate_id = ? AND type = ?", affiliate.Id, "site_earning").First(&ledger).Error)
	assert.InDelta(t, 45, ledger.DeltaAvailable, 1e-9)

	// 同一天结算后又有消费，记进新的槽位，下次结算再算。
	adtySiteEarningDayFn = func() string { return "2026-10-01" }
	consumeOnSite(t, customer, perUnit("12.5"))
	settled, err = RunAdtySiteSettlementOnce(day("2026-10-13 04:00"))
	require.NoError(t, err)
	assert.Equal(t, 1, settled)
	acc = account(t, db, affiliate.Id)
	assert.InDelta(t, 54, acc.AvailableAmount, 1e-9, "$45 + $4.5 + $4.5")
}

func TestPausedSiteNotSettled(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "paused-settle")
	customer := siteCustomer(t, db, site, "paused-customer")
	saved := adtySiteEarningDayFn
	t.Cleanup(func() { adtySiteEarningDayFn = saved })
	adtySiteEarningDayFn = func() string { return "2026-09-01" }
	consumeOnSite(t, customer, perUnit("10"))
	_, err := SetAdtySiteStatus(site.Id, 1, model.AdtySiteStatusPaused, "")
	require.NoError(t, err)
	settled, err := RunAdtySiteSettlementOnce(time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	require.NoError(t, err)
	assert.Zero(t, settled)
	var row model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ?", site.Id).First(&row).Error)
	assert.Equal(t, model.AdtySiteEarningPending, row.Status)
}

func TestNegativeRowsSettleWithoutFreeze(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "neg-now")
	customer := siteCustomer(t, db, site, "neg-now-customer")
	saved := adtySiteEarningDayFn
	t.Cleanup(func() { adtySiteEarningDayFn = saved })
	adtySiteEarningDayFn = func() string { return "2026-10-05" }
	siteTopUp(t, customer, site, "EPU-neg-now", 100, "10")
	consumeOnSite(t, customer, perUnit("10"))
	settled, err := RunAdtySiteSettlementOnce(time.Date(2026, 10, 6, 4, 0, 0, 0, time.Local))
	require.NoError(t, err)
	assert.Equal(t, 1, settled)
	var bonus, usage model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningBonus).First(&bonus).Error)
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningUsage).First(&usage).Error)
	assert.Equal(t, model.AdtySiteEarningAvailable, bonus.Status)
	assert.Equal(t, model.AdtySiteEarningPending, usage.Status)
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	assert.InDelta(t, -10, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
}

func TestNegativeSettlementBlocksWithdraw(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "negative")
	customer := siteCustomer(t, db, site, "negative-customer")
	saved := adtySiteEarningDayFn
	t.Cleanup(func() { adtySiteEarningDayFn = saved })
	adtySiteEarningDayFn = func() string { return "2026-09-01" }
	siteTopUp(t, customer, site, "EPU-negative", 100, "10")
	consumeOnSite(t, customer, perUnit("10")) // +$2，加赠 −$10
	_, err := RunAdtySiteSettlementOnce(time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	require.NoError(t, err)
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	assert.InDelta(t, -8, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
	_, err = Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: owner.Id, Amount: usd("25"), AccountType: model.ReferralWithdrawToUSDT,
		AccountNetwork: "TRC20", AccountNo: "T" + "A1B2C3D4E5F6G7H8J9K1L2M3N4P5Q6R7S", IdempotencyKey: "neg-1"})
	assert.ErrorIs(t, err, ErrReferralInsufficient)
}

func TestSiteOwnerTransfersToBalanceInstantly(t *testing.T) {
	db := prepareSiteDB(t)
	owner, _ := openActiveSite(t, db, "owner-transfer")
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ReferralAffiliate{}).Where("id = ?", affiliate.Id).Update("site_owner_since", time.Now().Unix()).Error)
	require.NoError(t, db.Model(&model.ReferralCommissionAccount{}).Where("affiliate_id = ?", affiliate.Id).Update("available_amount", 100).Error)
	var before model.User
	require.NoError(t, db.First(&before, owner.Id).Error)
	// 站长收益和邀请佣金一样：转入余额不用审核，立即到账（任务 6.4）。
	transfer, err := Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: owner.Id, Amount: usd("30"), AccountType: model.ReferralWithdrawToBalance, IdempotencyKey: "ot-1"})
	require.NoError(t, err)
	assert.Equal(t, model.ReferralWithdrawalStatusPaid, transfer.Status)
	var after model.User
	require.NoError(t, db.First(&after, owner.Id).Error)
	assert.Equal(t, before.Quota+int(perUnit("30")), after.Quota)
	// 提现到 USDT 照常人工审核。
	withdrawal, err := Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: owner.Id, Amount: usd("30"), AccountType: model.ReferralWithdrawToUSDT,
		AccountNetwork: "TRC20", AccountNo: "T" + "A1B2C3D4E5F6G7H8J9K1L2M3N4P5Q6R7S", IdempotencyKey: "ot-2"})
	require.NoError(t, err)
	assert.Equal(t, model.ReferralWithdrawalStatusPending, withdrawal.Status)
}

func TestForcedCloseVoidsAndClawsBack(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "clawback")
	customer := siteCustomer(t, db, site, "clawback-customer")
	saved := adtySiteEarningDayFn
	t.Cleanup(func() { adtySiteEarningDayFn = saved })
	adtySiteEarningDayFn = func() string { return "2026-09-01" }
	consumeOnSite(t, customer, perUnit("100")) // +$20
	_, err := RunAdtySiteSettlementOnce(time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	require.NoError(t, err)
	adtySiteEarningDayFn = func() string { return "2026-10-01" }
	consumeOnSite(t, customer, perUnit("50")) // +$10，还没结算
	_, err = ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{ClawBack: true, Immediate: true})
	require.NoError(t, err)
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	assert.InDelta(t, 0, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
	var open int64
	require.NoError(t, db.Model(&model.AdtySiteEarningDaily{}).Where("site_id = ? AND status <> ?", site.Id, model.AdtySiteEarningVoid).Count(&open).Error)
	assert.Zero(t, open)

	// 已结算 100、未结算加赠 -150、可提现仍是 100：负数先入账，扣回不超过当时可提现，账户停在 -50。
	owner2, site2 := openActiveSite(t, db, "claw-debt")
	customer2 := siteCustomer(t, db, site2, "claw-debt-customer")
	adtySiteEarningDayFn = func() string { return "2026-09-01" }
	consumeOnSite(t, customer2, perUnit("500")) // k 默认 1，d 0.8 → +$100
	_, err = RunAdtySiteSettlementOnce(time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	require.NoError(t, err)
	adtySiteEarningDayFn = func() string { return "2026-10-02" }
	siteTopUp(t, customer2, site2, "EPU-claw-debt", 1500, "10") // bonus −$150，还在冻结期
	affiliate2, err := Referral.EnsureAffiliate(owner2.Id)
	require.NoError(t, err)
	assert.InDelta(t, 100, account(t, db, affiliate2.Id).AvailableAmount, 1e-9)
	_, err = ScheduleAdtySiteClose(site2.Id, 1, AdtySiteClosePlan{ClawBack: true, Immediate: true})
	require.NoError(t, err)
	assert.InDelta(t, -50, account(t, db, affiliate2.Id).AvailableAmount, 1e-9)
}

func TestEarningHookConcurrentUpsert(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "concurrent")
	customer := siteCustomer(t, db, site, "concurrent-customer")
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			AdtySiteEarningHook(site.Id, customer.Id, perUnit("1"), 1.25, 0.8, model.AdtySiteEarningUsage)
		})
	}
	wg.Wait()
	var row model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningUsage).First(&row).Error)
	assert.EqualValues(t, perUnit("50"), row.Quota)
	assert.True(t, row.Earning.Equal(usd("18")), "50 × $0.36, got %s", row.Earning)
	assert.True(t, row.Revenue.Equal(usd("50")))
	var rows int64
	require.NoError(t, db.Model(&model.AdtySiteEarningDaily{}).Where("site_id = ?", site.Id).Count(&rows).Error)
	assert.EqualValues(t, 1, rows)
}

// 上游的任务退款测试在 SQLite 上跑：带分站快照的任务走到记账钩子时，SQLite 也要能写。
func TestEarningHookOnSQLite(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(model.AdtySiteTables()...))
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM adty_site_earning_daily")
		model.DB.Exec("DELETE FROM adty_site_customers")
	})
	for range 2 {
		AdtySiteEarningHook(3, 4, perUnit("1"), 1.25, 0.8, model.AdtySiteEarningUsage)
	}
	var row model.AdtySiteEarningDaily
	require.NoError(t, model.DB.Where("site_id = 3").First(&row).Error)
	assert.EqualValues(t, perUnit("2"), row.Quota)
	assert.True(t, row.Earning.Equal(usd("0.72")))
}

func TestReviewedSkippedDoesNotHoldSettlement(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "review-hold")
	recordFingerprint(t, db, owner.Id, "203.0.113.20", "owner-device-hold")
	customer := registerSiteCustomer(t, db, site, "review-hold-customer", "203.0.113.20", "other")
	saved := adtySiteEarningDayFn
	t.Cleanup(func() { adtySiteEarningDayFn = saved })
	adtySiteEarningDayFn = func() string { return "2026-09-01" }
	consumeOnSite(t, customer, perUnit("10"))
	require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, false))

	_, err := RunAdtySiteSettlementOnce(time.Date(2026, 10, 20, 4, 0, 0, 0, time.Local))
	require.NoError(t, err)
	var stored model.AdtySite
	require.NoError(t, db.First(&stored, site.Id).Error)
	assert.Empty(t, stored.SettleHoldReason)
	var skipped model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningSkipped).First(&skipped).Error)
	assert.Equal(t, model.AdtySiteEarningPending, skipped.Status)
}

func TestSettlementHoldsWhenUsageDoesNotMatchLogs(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "mismatch")
	customer := siteCustomer(t, db, site, "mismatch-customer")
	consumeOnSite(t, customer, perUnit("10"))
	require.NoError(t, db.Model(&model.AdtySiteEarningDaily{}).Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningUsage).
		Update("earning", usd("99")).Error)

	// FreezeDays 默认是 7，必须走过冻结期，这一天的行才会进对账。
	due := time.Now().AddDate(0, 0, 10)
	settled, err := RunAdtySiteSettlementOnce(due)
	require.NoError(t, err)
	assert.Zero(t, settled)
	var stored model.AdtySite
	require.NoError(t, db.First(&stored, site.Id).Error)
	assert.Contains(t, stored.SettleHoldReason, "do not match consume logs")
	var row model.AdtySiteEarningDaily
	require.NoError(t, db.Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteEarningUsage).First(&row).Error)
	assert.Equal(t, model.AdtySiteEarningPending, row.Status)

	resumed, err := ResumeAdtySiteSettlement(site.Id, 1, due)
	require.NoError(t, err)
	assert.Contains(t, resumed.SettleHoldReason, "do not match consume logs", "resume checks the numbers again")
}

func TestEarningHookFailureIncrementsCounter(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "hook-fail")
	customer := siteCustomer(t, db, site, "hook-fail-customer")
	AdtySiteEarningHook(site.Id, customer.Id, perUnit("1"), 1.25, 0.8, strings.Repeat("x", 40))
	var stored model.AdtySite
	require.NoError(t, db.First(&stored, site.Id).Error)
	assert.EqualValues(t, 1, stored.EarningHookFailures)
}
