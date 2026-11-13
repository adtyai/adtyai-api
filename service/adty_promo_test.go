package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func enablePromoCodes(t *testing.T) {
	t.Helper()
	cfg := adty_setting.GetPromoSetting()
	saved := cfg.PromoEnabled
	cfg.PromoEnabled = true
	t.Cleanup(func() { cfg.PromoEnabled = saved })
}

func preparePromoDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, testAdtyBonusRates)
	enablePromoCodes(t)
	oldRedis, oldBatch := common.RedisEnabled, common.BatchUpdateEnabled
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	t.Cleanup(func() { common.RedisEnabled, common.BatchUpdateEnabled = oldRedis, oldBatch })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.Option{}, &model.AuditLog{}))
	require.NoError(t, model.MigrateAdtyTables())
	return db
}

func promoUser(t *testing.T, db *gorm.DB, name string) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "x", Group: "default", AuthVersion: 1, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	return user
}

func promoCode(t *testing.T, db *gorm.DB, code model.AdtyPromoCode) *model.AdtyPromoCode {
	t.Helper()
	code.Code = model.NormalizeAdtyPromoCode(code.Code)
	code.Enabled = true
	if code.Funder == "" {
		code.Funder = model.AdtyPromoFunderPlatform
	}
	require.NoError(t, db.Create(&code).Error)
	return &code
}

// promoOrder 按报价下一笔 USDT 订单（和 createAdtyOrder 写的字段一样）。
func promoOrder(t *testing.T, user *model.User, tradeNo string, face int64, code string) (*AdtyQuote, error) {
	t.Helper()
	quote, err := AdtyQuoteFor(user.Group, face, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, SiteId: user.SiteId, Code: code})
	if err != nil {
		return nil, err
	}
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: user.Id, Amount: face, TradeNo: tradeNo, PaymentMethod: AdtyMethodUSDTTRC20, PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: user.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: AdtyMethodUSDTTRC20,
			FaceUSD: quote.FaceUSD, BonusRate: quote.BonusRate, BonusUSD: quote.BonusUSD, CreditQuota: quote.CreditQuota,
			PayAmount: quote.PayAmount, PayCurrency: quote.PayCurrency, TopupGroupRatio: quote.TopupGroupRatio,
			PromoCodeId: quote.PromoCodeId, PromoBonusUSD: quote.PromoBonusUSD, ExpireAt: time.Now().Unix() + 900},
	))
	return quote, nil
}

func payPromoOrder(t *testing.T, tradeNo string, face int64) error {
	t.Helper()
	_, err := model.CompleteAdtyTopUp(tradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(face), FiatAmount: decimal.NewFromInt(face),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	return err
}

func userQuota(t *testing.T, db *gorm.DB, userId int) int64 {
	t.Helper()
	var quota int64
	require.NoError(t, db.Model(&model.User{}).Select("quota").Where("id = ?", userId).Scan(&quota).Error)
	return quota
}

func TestPromoCodesAreRetired(t *testing.T) {
	_, bonus, err := adtyPromoBonusFor(AdtyPromoInput{UserId: 1, Code: "SPRING"}, decimal.NewFromInt(100))
	require.ErrorIs(t, err, ErrAdtyPromoRetired)
	assert.True(t, bonus.IsZero())
	code, bonus, err := adtyPromoBonusFor(AdtyPromoInput{Code: "   "}, decimal.NewFromInt(100))
	require.NoError(t, err)
	assert.Nil(t, code)
	assert.True(t, bonus.IsZero())
}

func TestPromoOffRejectsCodeOnQuote(t *testing.T) {
	db := preparePromoDB(t)
	adty_setting.GetPromoSetting().PromoEnabled = false
	user := promoUser(t, db, "promo-off")
	promoCode(t, db, model.AdtyPromoCode{Code: "OFF10PCT", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(10)})

	_, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, Code: "OFF10PCT"})
	require.ErrorIs(t, err, ErrAdtyPromoRetired)
	_, err = AdtyQuoteForSite(nil, "default", 100, AdtyMethodAlipay, AdtyPromoInput{UserId: user.Id, Code: " off10pct "})
	require.ErrorIs(t, err, ErrAdtyPromoRetired)

	quote, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, Code: "   "})
	require.NoError(t, err)
	assert.True(t, quote.PromoBonusUSD.IsZero())
	assert.EqualValues(t, 50_000_000, quote.CreditQuota)
}

func TestPromoPercentAndFixed(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-a")
}

func TestPromoMinFaceAndExpiry(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-b")
}

func TestPromoPerUserLimitConcurrent(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-c")
}

func TestPromoNewUserOnly(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-d")
}

func TestPromoNewUserConcurrentReuse(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-race")
}

func TestPromoFirstPaidEligibilityAcrossDifferentCodes(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-cross")
}

func TestPromoRefundClawsBack(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-e")
}

func TestPromoNotCountedForCommission(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-f")
}

func TestSitePromoDeductsOwnerEarning(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-site")
}

func TestPlatformPromoNotChargedToOwner(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-platform")
}

func TestSitePromoCappedByMaxSiteBonusExtra(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-cap")
}

func TestSitePromoOtherSiteRejected(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-other")
}

func TestSitePromoRefundReversesEarning(t *testing.T) {
	assertPromoQuoteRetired(t, "promo-reverse")
}

func assertPromoQuoteRetired(t *testing.T, name string) {
	t.Helper()
	db := preparePromoDB(t)
	user := promoUser(t, db, name)
	_, err := AdtyQuoteFor(user.Group, 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, Code: "TEN"})
	require.ErrorIs(t, err, ErrAdtyPromoRetired)
}
