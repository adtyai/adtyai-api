package service

import (
	"fmt"
	"strings"
	"sync"
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
	assert.EqualValues(t, 51_250_000, quote.CreditQuota)
}

func TestPromoPercentAndFixed(t *testing.T) {
	db := preparePromoDB(t)
	user := promoUser(t, db, "promo-a")
	promoCode(t, db, model.AdtyPromoCode{Code: "ten", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(10)})
	promoCode(t, db, model.AdtyPromoCode{Code: "FIVE", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(5)})

	// $100：普通加赠 2.5% = $2.5，优惠 10% = $10，入账 (100 + 2.5 + 10) × 500000。
	quote, err := AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, Code: " Ten "})
	require.NoError(t, err)
	assert.Equal(t, "2.5", quote.BonusUSD.String())
	assert.Equal(t, "10", quote.PromoBonusUSD.String())
	assert.EqualValues(t, 56_250_000, quote.CreditQuota)
	assert.Equal(t, "100", quote.PayAmount.String(), "promo codes never change what the user pays")

	quote, err = AdtyQuoteFor("default", 30, AdtyMethodAlipay, AdtyPromoInput{UserId: user.Id, Code: "five"})
	require.NoError(t, err)
	assert.Equal(t, "5", quote.PromoBonusUSD.String())
	assert.EqualValues(t, (30*1000+450+5000)*500, quote.CreditQuota) // 30 + 0.45 + 5
	assert.Equal(t, "219", quote.PayAmount.String())

	// 没有码时和原来一样。
	quote, err = AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id})
	require.NoError(t, err)
	assert.True(t, quote.PromoBonusUSD.IsZero())
	assert.EqualValues(t, 51_250_000, quote.CreditQuota)

	_, err = AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, Code: "nope"})
	require.ErrorIs(t, err, ErrAdtyPromoInvalid)

	// 入账：面额 + 加赠 + 优惠加赠一起到账，次数加一。
	_, err = promoOrder(t, user, "PROMO-A1", 100, "ten")
	require.NoError(t, err)
	require.NoError(t, payPromoOrder(t, "PROMO-A1", 100))
	assert.EqualValues(t, 56_250_000, userQuota(t, db, user.Id))
	code, err := model.GetAdtyPromoCodeByCode("TEN")
	require.NoError(t, err)
	assert.Equal(t, 1, code.UsedCount)
}

func TestPromoMinFaceAndExpiry(t *testing.T) {
	db := preparePromoDB(t)
	user := promoUser(t, db, "promo-b")
	now := time.Now().Unix()
	promoCode(t, db, model.AdtyPromoCode{Code: "MIN50", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(3), MinFaceUSD: decimal.NewFromInt(50)})
	promoCode(t, db, model.AdtyPromoCode{Code: "OLD", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(3), ExpiresAt: now - 1})
	promoCode(t, db, model.AdtyPromoCode{Code: "SOON", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(3), StartsAt: now + 3600})
	off := promoCode(t, db, model.AdtyPromoCode{Code: "OFF", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(3)})
	require.NoError(t, db.Model(off).Update("enabled", false).Error)
	promoCode(t, db, model.AdtyPromoCode{Code: "SITE7", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(3), SiteId: 7})

	in := func(code string) AdtyPromoInput { return AdtyPromoInput{UserId: user.Id, Code: code} }
	_, err := AdtyQuoteFor("default", 49, AdtyMethodUSDTTRC20, in("MIN50"))
	var message *common.Message
	require.ErrorAs(t, err, &message)
	assert.Equal(t, AdtyPromoMinFaceKey, message.Key)
	assert.Equal(t, "50.00", message.Params["amount"])
	_, err = AdtyQuoteFor("default", 50, AdtyMethodUSDTTRC20, in("MIN50"))
	require.NoError(t, err)
	for _, code := range []string{"OLD", "SOON", "OFF", "SITE7"} {
		_, err = AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, in(code))
		require.ErrorIs(t, err, ErrAdtyPromoInvalid, code)
	}
}

func TestPromoPerUserLimitConcurrent(t *testing.T) {
	db := preparePromoDB(t)
	promoCode(t, db, model.AdtyPromoCode{Code: "ONCE", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(5), PerUserLimit: 1, MaxUses: 2})
	user := promoUser(t, db, "promo-c")

	// 一个人同时下 5 单都带码（下单不占次数），同时付款：只有一单拿到优惠加赠。
	const orders = 5
	for i := range orders {
		_, err := promoOrder(t, user, fmt.Sprintf("PROMO-C%d", i), 10, "ONCE")
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	errs := make([]error, orders)
	for i := range orders {
		wg.Go(func() { errs[i] = payPromoOrder(t, fmt.Sprintf("PROMO-C%d", i), 10) })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err, "every paid order is credited")
	}
	// 5 × (10 + 0.1) + 一次 5。
	assert.EqualValues(t, (orders*10100+5000)*500, userQuota(t, db, user.Id))
	var redemptions int64
	require.NoError(t, db.Model(&model.AdtyPromoRedemption{}).Count(&redemptions).Error)
	assert.EqualValues(t, 1, redemptions)

	// 同一个人再报价：已经用过了。
	_, err := AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, Code: "ONCE"})
	require.ErrorIs(t, err, ErrAdtyPromoUsed)

	// 总次数：第二个人能用，第三个人报价时已经用完。
	second, third := promoUser(t, db, "promo-c2"), promoUser(t, db, "promo-c3")
	_, err = promoOrder(t, second, "PROMO-C-2", 10, "ONCE")
	require.NoError(t, err)
	_, err = promoOrder(t, third, "PROMO-C-3", 10, "ONCE")
	require.NoError(t, err)
	require.NoError(t, payPromoOrder(t, "PROMO-C-2", 10))
	require.NoError(t, payPromoOrder(t, "PROMO-C-3", 10))
	assert.EqualValues(t, 10_100*500+5000*500, userQuota(t, db, second.Id))
	assert.EqualValues(t, 10_100*500, userQuota(t, db, third.Id), "the code ran out before this order was paid")
	_, err = AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: promoUser(t, db, "promo-c4").Id, Code: "ONCE"})
	require.ErrorIs(t, err, ErrAdtyPromoInvalid)

	var code model.AdtyPromoCode
	require.NoError(t, db.Where("code = ?", "ONCE").First(&code).Error)
	assert.Equal(t, 2, code.UsedCount)
}

func TestPromoNewUserOnly(t *testing.T) {
	db := preparePromoDB(t)
	promoCode(t, db, model.AdtyPromoCode{Code: "WELCOME", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(20), NewUsersOnly: true})
	fresh := promoUser(t, db, "promo-new")
	_, err := AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: fresh.Id, Code: "WELCOME"})
	require.NoError(t, err)

	paid := promoUser(t, db, "promo-paid")
	_, err = promoOrder(t, paid, "PROMO-D1", 10, "")
	require.NoError(t, err)
	require.NoError(t, payPromoOrder(t, "PROMO-D1", 10))
	_, err = AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: paid.Id, Code: "WELCOME"})
	require.ErrorIs(t, err, ErrAdtyPromoNewUsersOnly)

	// 只有失败或过期订单的人仍算新用户。
	failed := promoUser(t, db, "promo-failed")
	require.NoError(t, db.Create(&model.TopUp{UserId: failed.Id, TradeNo: "PROMO-D2", Status: common.TopUpStatusExpired}).Error)
	_, err = AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: failed.Id, Code: "WELCOME"})
	require.NoError(t, err)

	// 每人次数为 0 的新用户码仍然每人只能入账一次。两笔未付订单一起付款，只有一笔带优惠。
	repeat := promoCode(t, db, model.AdtyPromoCode{Code: "ONCEWELCOME", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(2), NewUsersOnly: true})
	require.Zero(t, repeat.PerUserLimit)
	for _, tradeNo := range []string{"PROMO-D3", "PROMO-D4"} {
		_, err = promoOrder(t, fresh, tradeNo, 10, "ONCEWELCOME")
		require.NoError(t, err)
	}
	require.NoError(t, payPromoOrder(t, "PROMO-D3", 10))
	require.NoError(t, payPromoOrder(t, "PROMO-D4", 10))
	var redemptions int64
	require.NoError(t, db.Model(&model.AdtyPromoRedemption{}).Where("code_id = ?", repeat.Id).Count(&redemptions).Error)
	assert.EqualValues(t, 1, redemptions)
	var stored []model.AdtyTopupOrder
	require.NoError(t, db.Where("trade_no IN ?", []string{"PROMO-D3", "PROMO-D4"}).Find(&stored).Error)
	withPromo := 0
	for _, order := range stored {
		if order.PromoBonusUSD.IsPositive() {
			withPromo++
		}
	}
	assert.Equal(t, 1, withPromo)

	// 换账号但 IP 哈希相同，而且原账号已经充值成功，不再算新用户。
	payer := promoUser(t, db, "promo-payer")
	twin := promoUser(t, db, "promo-twin")
	require.NoError(t, db.Create(&model.AdtyUserFingerprint{UserId: payer.Id, IpHash: "shared-ip", DeviceHash: "payer-device", Kind: "login", CreatedAt: time.Now().Unix()}).Error)
	require.NoError(t, db.Create(&model.AdtyUserFingerprint{UserId: twin.Id, IpHash: "shared-ip", DeviceHash: "twin-device", Kind: "login", CreatedAt: time.Now().Unix()}).Error)
	_, err = promoOrder(t, payer, "PROMO-D5", 10, "")
	require.NoError(t, err)
	require.NoError(t, payPromoOrder(t, "PROMO-D5", 10))
	_, err = AdtyQuoteFor("default", 10, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: twin.Id, Code: "WELCOME"})
	require.ErrorIs(t, err, ErrAdtyPromoNewUsersOnly)
	_, err = promoOrder(t, twin, "PROMO-D6", 10, "WELCOME")
	require.ErrorIs(t, err, ErrAdtyPromoNewUsersOnly)
}

func TestPromoNewUserConcurrentReuse(t *testing.T) {
	db := preparePromoDB(t)
	code := promoCode(t, db, model.AdtyPromoCode{Code: "RACEWELCOME", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(2), NewUsersOnly: true})
	user := promoUser(t, db, "promo-race")
	for i := range 2 {
		_, err := promoOrder(t, user, fmt.Sprintf("PROMO-RACE-%d", i), 10, "RACEWELCOME")
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { errs[i] = payPromoOrder(t, fmt.Sprintf("PROMO-RACE-%d", i), 10) })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var redemptions int64
	require.NoError(t, db.Model(&model.AdtyPromoRedemption{}).Where("code_id = ?", code.Id).Count(&redemptions).Error)
	assert.EqualValues(t, 1, redemptions)
	assert.EqualValues(t, 6_050_000+5_050_000, userQuota(t, db, user.Id))
}

func TestPromoFirstPaidEligibilityAcrossDifferentCodes(t *testing.T) {
	for _, sharedFingerprint := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared_fingerprint_%t", sharedFingerprint), func(t *testing.T) {
			db := preparePromoDB(t)
			first := promoUser(t, db, "promo-cross-first")
			second := first
			if sharedFingerprint {
				second = promoUser(t, db, "promo-cross-second")
				for _, user := range []*model.User{first, second} {
					require.NoError(t, db.Create(&model.AdtyUserFingerprint{UserId: user.Id, IpHash: "cross-code-shared-ip", Kind: "login"}).Error)
				}
			}
			for i, user := range []*model.User{first, second} {
				code := fmt.Sprintf("CROSS%d", i)
				promoCode(t, db, model.AdtyPromoCode{Code: code, Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(2), NewUsersOnly: true})
				_, err := promoOrder(t, user, fmt.Sprintf("PROMO-CROSS-%d", i), 10, code)
				require.NoError(t, err)
			}
			// Keep the two eligibility reads overlapping when there is no shared
			// serialization fence. A correct fence makes the first reader time out
			// here, commit, and only then lets the second reader observe its payment.
			arrived := make(chan struct{})
			var mu sync.Mutex
			reads := 0
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:cross-code-first-paid", func(tx *gorm.DB) {
				if tx.Statement.Table != "top_ups" || !strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "count(") {
					return
				}
				mu.Lock()
				reads++
				if reads == 2 {
					close(arrived)
				}
				mu.Unlock()
				select {
				case <-arrived:
				case <-time.After(250 * time.Millisecond):
				}
			}))
			t.Cleanup(func() { db.Callback().Query().Remove("test:cross-code-first-paid") })
			var wg sync.WaitGroup
			errs := make([]error, 2)
			for i := range 2 {
				wg.Go(func() { errs[i] = payPromoOrder(t, fmt.Sprintf("PROMO-CROSS-%d", i), 10) })
			}
			wg.Wait()
			for _, err := range errs {
				require.NoError(t, err)
			}
			var count int64
			require.NoError(t, db.Model(&model.AdtyPromoRedemption{}).Count(&count).Error)
			assert.EqualValues(t, 1, count, "different codes share the same first-paid eligibility")
		})
	}
}

func TestPromoRefundClawsBack(t *testing.T) {
	db := preparePromoDB(t)
	user := promoUser(t, db, "promo-e")
	promoCode(t, db, model.AdtyPromoCode{Code: "BACK", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(7)})
	_, err := promoOrder(t, user, "PROMO-E1", 100, "BACK")
	require.NoError(t, err)
	require.NoError(t, payPromoOrder(t, "PROMO-E1", 100))
	assert.EqualValues(t, (100_000+2_500+7_000)*500, userQuota(t, db, user.Id))

	_, err = refundAdtyTopUp(t, "PROMO-E1", "customer asked", 1, "promo-refund-1")
	require.NoError(t, err)
	assert.Zero(t, userQuota(t, db, user.Id), "face, normal bonus and promo bonus are all taken back")
}

func TestPromoNotCountedForCommission(t *testing.T) {
	db := preparePromoDB(t)
	user := promoUser(t, db, "promo-f")
	promoCode(t, db, model.AdtyPromoCode{Code: "NOCOMM", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(9)})
	quote, err := promoOrder(t, user, "PROMO-F1", 100, "NOCOMM")
	require.NoError(t, err)
	ext, err := model.GetAdtyTopupOrder("PROMO-F1")
	require.NoError(t, err)
	// 佣金按面额计（service/referral_commission.go），优惠加赠单独存，不进面额。
	assert.Equal(t, "100", ext.FaceUSD.String())
	assert.Equal(t, "9", ext.PromoBonusUSD.String())
	assert.Equal(t, quote.CreditQuota, ext.CreditQuota)
}

// 站长出资的优惠码（12.4 + 11.5）：测试库用 prepareSiteDB（d 0.8、max_site_bonus_extra 2 点）。
func prepareSitePromo(t *testing.T) (*gorm.DB, *model.AdtySite, *model.User) {
	t.Helper()
	db := prepareSiteDB(t)
	enablePromoCodes(t)
	confirmAdtyCompliance(t)
	setAdtyBonusRates(t, map[int]float64{100: 2.5})
	_, site := openActiveSite(t, db, "promo-site")
	// 站点 $100 档 3%（主站 2.5% + 额外 2 点的上限是 4.5%）。
	require.NoError(t, SaveAdtySiteBonusRates(site.Id, map[int]float64{100: 3}))
	customer := siteCustomer(t, db, site, "promo-site-customer")
	return db, AdtySiteById(site.Id), customer
}

// sitePromoOrder 按分站报价下单（和 createAdtyOrder 写的字段一样，SiteId 和优惠码快照都带上）。
func sitePromoOrder(t *testing.T, site *model.AdtySite, user *model.User, tradeNo string, face int64, code string) (*AdtyQuote, error) {
	t.Helper()
	quote, err := AdtyQuoteForSite(site, user.Group, face, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: user.Id, SiteId: user.SiteId, Code: code})
	if err != nil {
		return nil, err
	}
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: user.Id, Amount: face, TradeNo: tradeNo, PaymentMethod: AdtyMethodUSDTTRC20, PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: user.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: AdtyMethodUSDTTRC20,
			FaceUSD: quote.FaceUSD, BonusRate: quote.BonusRate, BonusUSD: quote.BonusUSD, CreditQuota: quote.CreditQuota,
			PayAmount: quote.PayAmount, PayCurrency: quote.PayCurrency, TopupGroupRatio: quote.TopupGroupRatio,
			PromoCodeId: quote.PromoCodeId, PromoBonusUSD: quote.PromoBonusUSD, PromoFunder: quote.PromoFunder,
			SiteId: site.Id, ExpireAt: time.Now().Unix() + 900},
	))
	return quote, nil
}

func TestSitePromoDeductsOwnerEarning(t *testing.T) {
	db, site, customer := prepareSitePromo(t)
	promoCode(t, db, model.AdtyPromoCode{Code: "OWNER1", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(1), SiteId: site.Id, Funder: model.AdtyPromoFunderSiteOwner})

	quote, err := sitePromoOrder(t, site, customer, "SPROMO-A", 100, "OWNER1")
	require.NoError(t, err)
	assert.Equal(t, "3", quote.BonusUSD.String())
	assert.Equal(t, "1", quote.PromoBonusUSD.String())
	require.NoError(t, payPromoOrder(t, "SPROMO-A", 100))
	assert.EqualValues(t, perUnit("104"), userQuota(t, db, customer.Id))
	// 站点加赠 $3 和站长出资的优惠加赠 $1 一起从站长收益里扣。
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningBonus].Equal(usd("-4")))
}

func TestPlatformPromoNotChargedToOwner(t *testing.T) {
	db, site, customer := prepareSitePromo(t)
	promoCode(t, db, model.AdtyPromoCode{Code: "PLAT5", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(5), SiteId: site.Id})
	_, err := sitePromoOrder(t, site, customer, "SPROMO-B", 100, "PLAT5")
	require.NoError(t, err)
	require.NoError(t, payPromoOrder(t, "SPROMO-B", 100))
	assert.EqualValues(t, perUnit("108"), userQuota(t, db, customer.Id))
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningBonus].Equal(usd("-3")), "only the site's own tier bonus")
}

func TestSitePromoCappedByMaxSiteBonusExtra(t *testing.T) {
	db, site, customer := prepareSitePromo(t)
	// 主站 $100 档 2.5% + 额外 2 点 = 4.5%；站点档 3%，站长码最多再加 1.5%。
	promoCode(t, db, model.AdtyPromoCode{Code: "OK15", Kind: model.AdtyPromoKindPercent, Value: decimal.RequireFromString("1.5"), SiteId: site.Id, Funder: model.AdtyPromoFunderSiteOwner})
	promoCode(t, db, model.AdtyPromoCode{Code: "TOO16", Kind: model.AdtyPromoKindPercent, Value: decimal.RequireFromString("1.6"), SiteId: site.Id, Funder: model.AdtyPromoFunderSiteOwner})
	promoCode(t, db, model.AdtyPromoCode{Code: "FIX2", Kind: model.AdtyPromoKindFixed, Value: decimal.NewFromInt(2), SiteId: site.Id, Funder: model.AdtyPromoFunderSiteOwner})
	in := func(code string) AdtyPromoInput {
		return AdtyPromoInput{UserId: customer.Id, SiteId: site.Id, Code: code}
	}
	_, err := AdtyQuoteForSite(site, "default", 100, AdtyMethodUSDTTRC20, in("OK15"))
	require.NoError(t, err)
	_, err = AdtyQuoteForSite(site, "default", 100, AdtyMethodUSDTTRC20, in("TOO16"))
	require.ErrorIs(t, err, ErrAdtySitePromoOverCap)
	// 固定 $2 在 $100 上是 2%，超出；在 $200 上是 1%，没超。
	_, err = AdtyQuoteForSite(site, "default", 100, AdtyMethodUSDTTRC20, in("FIX2"))
	require.ErrorIs(t, err, ErrAdtySitePromoOverCap)
	_, err = AdtyQuoteForSite(site, "default", 200, AdtyMethodUSDTTRC20, in("FIX2"))
	require.NoError(t, err)

	// max_site_bonus_extra 没设时按 0 算：站点档已经高出主站，站长码一律超出。
	adty_setting.GetResellerSetting().MaxSiteBonusExtra = nil
	_, err = AdtyQuoteForSite(site, "default", 200, AdtyMethodUSDTTRC20, in("FIX2"))
	require.ErrorIs(t, err, ErrAdtySitePromoOverCap)
}

func TestSitePromoOtherSiteRejected(t *testing.T) {
	db, site, customer := prepareSitePromo(t)
	_, other := openActiveSite(t, db, "promo-other")
	promoCode(t, db, model.AdtyPromoCode{Code: "OTHER1", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(1), SiteId: other.Id, Funder: model.AdtyPromoFunderSiteOwner})
	promoCode(t, db, model.AdtyPromoCode{Code: "MINE1", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(1), SiteId: site.Id, Funder: model.AdtyPromoFunderSiteOwner})
	_, err := AdtyQuoteForSite(site, "default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: customer.Id, SiteId: site.Id, Code: "OTHER1"})
	require.ErrorIs(t, err, ErrAdtyPromoInvalid)
	// 主站客户不能用分站站长的码。
	mainUser := promoUser(t, db, "promo-main-user")
	_, err = AdtyQuoteFor("default", 100, AdtyMethodUSDTTRC20, AdtyPromoInput{UserId: mainUser.Id, Code: "MINE1"})
	require.ErrorIs(t, err, ErrAdtyPromoInvalid)
}

func TestSitePromoRefundReversesEarning(t *testing.T) {
	db, site, customer := prepareSitePromo(t)
	promoCode(t, db, model.AdtyPromoCode{Code: "BACK1", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(1), SiteId: site.Id, Funder: model.AdtyPromoFunderSiteOwner})
	_, err := sitePromoOrder(t, site, customer, "SPROMO-C", 100, "BACK1")
	require.NoError(t, err)
	require.NoError(t, payPromoOrder(t, "SPROMO-C", 100))
	_, err = refundAdtyTopUp(t, "SPROMO-C", "chargeback", 1, "site-promo-refund")
	require.NoError(t, err)
	totals := earningTotals(t, db, site.Id)
	assert.True(t, totals[model.AdtySiteEarningBonus].Equal(usd("-4")))
	assert.True(t, totals[model.AdtySiteEarningBonusReversal].Equal(usd("4")))
	assert.Zero(t, userQuota(t, db, customer.Id))
}
