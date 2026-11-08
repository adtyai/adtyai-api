package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefundKeepsDurableWalletAndLedgerAtomicOnFailure(t *testing.T) {
	for _, databaseFailure := range []bool{true, false} {
		t.Run(map[bool]string{true: "database rollback", false: "metadata cache outage"}[databaseFailure], func(t *testing.T) {
			db := prepareAdtyTopUpDB(t)
			server := useUserCacheMiniRedis(t)
			resetBatchUpdateTestState(t)
			common.BatchUpdateEnabled = true
			user := seedAdtyTopUpUser(t, db, "refund-atomic", 55000000)
			seedCreditedOrder(t, db, user.Id, "RF-atomic", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
			require.NoError(t, DecreaseUserQuota(user.Id, 15000000, false))
			_, err := GetUserCache(user.Id)
			require.NoError(t, err)
			if databaseFailure {
				const callback = "audit_refund_ledger_failure"
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "adty_balance_ledgers" {
						tx.AddError(gorm.ErrInvalidTransaction)
					}
				}))
				t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })
			} else {
				server.SetError("READONLY")
			}
			_, err = RefundAdtyTopUp("RF-atomic", "synthetic refund", 1, "atomic-refund", 35000000)
			var stored User
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, -15000000, stored.QuotaPending)
			var count int64
			require.NoError(t, db.Model(&AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&count).Error)
			if databaseFailure {
				require.ErrorIs(t, err, gorm.ErrInvalidTransaction)
				require.Equal(t, 55000000, stored.Quota)
				require.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "RF-atomic"))
				require.Zero(t, count)
			} else {
				require.NoError(t, err, "metadata failures cannot undo a committed refund")
				require.Equal(t, 15000000, stored.Quota)
				require.Equal(t, TopUpStatusRefunded, topUpStatus(t, db, "RF-atomic"))
				require.EqualValues(t, 1, count)
				again, err := RefundAdtyTopUp("RF-atomic", "synthetic refund", 1, "atomic-refund", 35000000)
				require.NoError(t, err)
				require.EqualValues(t, 40000000, again.DeductedQuota)
			}
			server.SetError("")
			current, err := GetUserCache(user.Id)
			require.NoError(t, err)
			if databaseFailure {
				require.Equal(t, 40000000, current.Quota)
			} else {
				require.Zero(t, current.Quota)
			}
		})
	}
}

func prepareAdtyTopUpDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	oldRedis, oldPerUnit := common.RedisEnabled, common.QuotaPerUnit
	common.RedisEnabled = false
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.RedisEnabled, common.QuotaPerUnit = oldRedis, oldPerUnit })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &TopUp{}, &Log{}))
	require.NoError(t, MigrateAdtyTables())
	return db
}

func seedAdtyTopUpUser(t *testing.T, db *gorm.DB, name string, quota int) *User {
	t.Helper()
	user := &User{Username: name, Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: quota, AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	return user
}

// seedAdtyOrder 建一笔 $100 加赠 2.5% 的订单（入账 51,250,000）。
func seedAdtyOrder(t *testing.T, userID int, tradeNo, provider, status string) *AdtyTopupOrder {
	t.Helper()
	method, currency, pay := "usdt.trc20", "USDT", decimal.RequireFromString("50.01")
	if provider == PaymentProviderAdtyEpay {
		method, currency, pay = "alipay", "CNY", decimal.RequireFromString("730.00")
	}
	topUp := &TopUp{UserId: userID, Amount: 100, Money: 1, TradeNo: tradeNo, PaymentMethod: method, PaymentProvider: provider, CreateTime: 1, Status: status}
	ext := &AdtyTopupOrder{
		TradeNo: tradeNo, UserId: userID, Provider: provider, Method: method,
		FaceUSD: decimal.NewFromInt(100), BonusRate: decimal.RequireFromString("2.5"), BonusUSD: decimal.RequireFromString("2.5"),
		CreditQuota: 51_250_000, PayAmount: pay, PayCurrency: currency, TopupGroupRatio: decimal.NewFromInt(1),
		ExpireAt: common.GetTimestamp() + 900, PayAddress: "TAddr", GatewayTradeId: "gw-" + tradeNo,
	}
	require.NoError(t, CreateAdtyTopUp(topUp, ext))
	return ext
}

func usdtFacts(tradeNo string, paid string) AdtyPaymentFacts {
	return AdtyPaymentFacts{
		Provider: PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
		PaidAmount: decimal.RequireFromString(paid), FiatAmount: decimal.NewFromInt(100), PaidCurrency: "USDT",
		PayAddress: "TAddr", GatewayTradeId: "gw-" + tradeNo, TxHash: "0xhash", Source: AdtyPaymentSourceCallback,
	}
}

func userQuota(t *testing.T, db *gorm.DB, id int) int {
	t.Helper()
	var user User
	require.NoError(t, db.Select("quota").First(&user, id).Error)
	return user.Quota
}

func topUpStatus(t *testing.T, db *gorm.DB, tradeNo string) string {
	t.Helper()
	var topUp TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&topUp).Error)
	return topUp.Status
}

func TestEpayDailyLimitCountsPendingReservations(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	require.NoError(t, db.AutoMigrate(&AdtyEpayChannel{}, &AdtyEpayChannelMethod{}))
	channel := &AdtyEpayChannel{Name: "cap", Status: AdtyEpayStatusEnabled, MerchantId: "1", MerchantKey: "k"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&AdtyEpayChannelMethod{
		ChannelId: channel.Id, Method: "alipay", Enabled: true, DailyLimitCNY: decimal.NewFromInt(10),
	}).Error)
	user := seedAdtyTopUpUser(t, db, "epay-cap", 0)
	makeOrder := func(trade string) error {
		topUp := &TopUp{
			UserId: user.Id, Amount: 1, Money: 10, TradeNo: trade, PaymentMethod: "alipay",
			PaymentProvider: PaymentProviderAdtyEpay, CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
		}
		ext := &AdtyTopupOrder{
			TradeNo: trade, UserId: user.Id, Provider: PaymentProviderAdtyEpay, Method: "alipay",
			FaceUSD: decimal.NewFromInt(1), PayAmount: decimal.NewFromInt(10), PayCurrency: "CNY",
			ChannelId: channel.Id, CreditQuota: 1, ExpireAt: time.Now().Unix() + 600,
		}
		return CreateAdtyTopUp(topUp, ext)
	}
	require.NoError(t, makeOrder("EPY-cap-1"))
	require.ErrorIs(t, makeOrder("EPY-cap-2"), ErrAdtyEpayDailyLimit)
}

func TestReplacedPendingOrderIsNotAutoCredited(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "replaced-payer", 0)
	seedAdtyOrder(t, user.Id, "EPU-replaced", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := ReplaceAdtyTopUpAfterLock("EPU-replaced", user.Id, nil)
	require.NoError(t, err)
	_, err = CompleteAdtyTopUp("EPU-replaced", usdtFacts("EPU-replaced", "50.01"), "")
	require.ErrorIs(t, err, ErrTopUpStatusInvalid)
	assert.Equal(t, 0, userQuota(t, db, user.Id))
	assert.Equal(t, TopUpStatusReplaced, topUpStatus(t, db, "EPU-replaced"))
}

func TestCompleteAdtyTopUpCreditsFaceAndBonusOnce(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "credit-once", 0)
	seedAdtyOrder(t, user.Id, "EPU-once", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)

	done, err := CompleteAdtyTopUp("EPU-once", usdtFacts("EPU-once", "50.01"), "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, done)
	assert.Equal(t, 51_250_000, userQuota(t, db, user.Id))
	assert.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "EPU-once"))

	done, err = CompleteAdtyTopUp("EPU-once", usdtFacts("EPU-once", "50.01"), "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, done)

	other := usdtFacts("EPU-once", "50.01")
	other.TxHash = "0xother-payment"
	done, err = CompleteAdtyTopUp("EPU-once", other, "127.0.0.1")
	require.ErrorIs(t, err, ErrAdtyDuplicatePayment)
	assert.False(t, done)
	assert.Equal(t, 51_250_000, userQuota(t, db, user.Id))
	var reviews int64
	require.NoError(t, db.Model(&PaymentOrphanEvent{}).Where("event_type = ? AND reference_id = ?", "duplicate_payment", "EPU-once").Count(&reviews).Error)
	assert.EqualValues(t, 1, reviews)
	assert.Equal(t, 51_250_000, userQuota(t, db, user.Id))

	ext, err := GetAdtyTopupOrder("EPU-once")
	require.NoError(t, err)
	assert.Equal(t, "0xhash", ext.PaidTxHash)
	assert.Equal(t, "50.01", ext.PaidAmount.String())
}

func TestCompleteAdtyTopUpConcurrent(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "credit-race", 0)
	seedAdtyOrder(t, user.Id, "EPU-race", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)

	var wg sync.WaitGroup
	var mu sync.Mutex
	firstCredits := 0
	for range 10 {
		wg.Go(func() {
			done, err := CompleteAdtyTopUp("EPU-race", usdtFacts("EPU-race", "50.01"), "")
			if err == nil && !done {
				mu.Lock()
				firstCredits++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 1, firstCredits)
	assert.Equal(t, 51_250_000, userQuota(t, db, user.Id))
}

func TestCompleteAdtyTopUpLateStatuses(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	for _, status := range []string{common.TopUpStatusExpired, TopUpStatusCanceled, common.TopUpStatusFailed} {
		t.Run(status, func(t *testing.T) {
			user := seedAdtyTopUpUser(t, db, "late-"+status, 0)
			tradeNo := "EPU-late-" + status
			seedAdtyOrder(t, user.Id, tradeNo, PaymentProviderAdtyBEpusdt, status)
			done, err := CompleteAdtyTopUp(tradeNo, usdtFacts(tradeNo, "50.01"), "")
			require.NoError(t, err)
			assert.False(t, done)
			assert.Equal(t, 51_250_000, userQuota(t, db, user.Id))
		})
	}
}

func TestCompleteAdtyTopUpRejects(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "reject", 0)
	seedAdtyOrder(t, user.Id, "EPU-refunded", PaymentProviderAdtyBEpusdt, TopUpStatusRefunded)
	_, err := CompleteAdtyTopUp("EPU-refunded", usdtFacts("EPU-refunded", "50.01"), "")
	require.ErrorIs(t, err, ErrTopUpStatusInvalid)

	seedAdtyOrder(t, user.Id, "EPU-mismatch", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	facts := usdtFacts("EPU-mismatch", "50.01")
	facts.Provider = PaymentProviderAdtyEpay
	_, err = CompleteAdtyTopUp("EPU-mismatch", facts, "")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	_, err = CompleteAdtyTopUp("EPU-missing", usdtFacts("EPU-missing", "50.01"), "")
	require.ErrorIs(t, err, ErrTopUpNotFound)
	assert.Zero(t, userQuota(t, db, user.Id))
}

func TestCompleteAdtyTopUpAmountMismatch(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "amount", 0)

	seedAdtyOrder(t, user.Id, "EPU-short", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := CompleteAdtyTopUp("EPU-short", usdtFacts("EPU-short", "49.99"), "")
	require.ErrorIs(t, err, ErrPaymentAmountMismatch)

	seedAdtyOrder(t, user.Id, "EPU-addr", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	facts := usdtFacts("EPU-addr", "50.01")
	facts.PayAddress = "TOther"
	_, err = CompleteAdtyTopUp("EPU-addr", facts, "")
	require.ErrorIs(t, err, ErrPaymentAmountMismatch)

	seedAdtyOrder(t, user.Id, "EPY-cny", PaymentProviderAdtyEpay, common.TopUpStatusPending)
	_, err = CompleteAdtyTopUp("EPY-cny", AdtyPaymentFacts{
		Provider: PaymentProviderAdtyEpay, Method: "alipay", PaidAmount: decimal.RequireFromString("729.99"),
		PaidCurrency: "CNY", Source: AdtyPaymentSourceCallback,
	}, "")
	require.ErrorIs(t, err, ErrPaymentAmountMismatch)

	assert.Zero(t, userQuota(t, db, user.Id))
	assert.Equal(t, common.TopUpStatusPending, topUpStatus(t, db, "EPU-short"))
}

func TestCompleteAdtyTopUpQuotaLimit(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "limit", common.MaxWalletQuota-1)
	seedAdtyOrder(t, user.Id, "EPU-limit", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := CompleteAdtyTopUp("EPU-limit", usdtFacts("EPU-limit", "50.01"), "")
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	assert.Equal(t, common.TopUpStatusPending, topUpStatus(t, db, "EPU-limit"))
}

func TestRechargeEpayRejectsMismatchedMoney(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "epay-money", 0)
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 2, Money: 10, TradeNo: "EPAY-SHORT",
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}).Error)
	_, err := RechargeEpay("EPAY-SHORT", "alipay", decimal.NewFromInt(1), "")
	require.ErrorIs(t, err, ErrPaymentAmountMismatch)
	assert.Zero(t, userQuota(t, db, user.Id))
	assert.Equal(t, common.TopUpStatusPending, topUpStatus(t, db, "EPAY-SHORT"))

	_, err = RechargeEpay("EPAY-SHORT", "alipay", decimal.RequireFromString("10.00"), "")
	require.NoError(t, err)
	assert.Equal(t, 1000000, userQuota(t, db, user.Id))
	assert.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "EPAY-SHORT"))
}

func TestManualCompleteAlreadySuccessDoesNotWriteZeroLog(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "repeat-manual", 0)
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 2, Money: 2, TradeNo: "EPAY-REPEAT",
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}).Error)
	require.NoError(t, ManualCompleteTopUp("EPAY-REPEAT", "127.0.0.1"))
	quota := userQuota(t, db, user.Id)
	require.NotZero(t, quota)
	var logs int64
	require.NoError(t, db.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&logs).Error)
	require.Equal(t, int64(1), logs)

	require.NoError(t, ManualCompleteTopUp("EPAY-REPEAT", "127.0.0.1"))
	require.NoError(t, db.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&logs).Error)
	require.Equal(t, int64(1), logs)
	assert.Equal(t, quota, userQuota(t, db, user.Id))
	var zeroUser int64
	require.NoError(t, db.Model(&Log{}).Where("user_id = ? AND type = ?", 0, LogTypeTopup).Count(&zeroUser).Error)
	assert.Zero(t, zeroUser)
}

func TestManualCompleteCreemUsesStoredQuota(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "creem-manual", 0)
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 1000, Money: 2, TradeNo: "CREEM-MANUAL",
		PaymentMethod: "creem", PaymentProvider: PaymentProviderCreem, Status: common.TopUpStatusPending,
	}).Error)
	require.NoError(t, ManualCompleteTopUp("CREEM-MANUAL", "127.0.0.1"))
	assert.Equal(t, 1000, userQuota(t, db, user.Id))
	assert.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "CREEM-MANUAL"))
}

func TestUpstreamPathsRejectAdtyOrders(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "upstream", 0)
	seedAdtyOrder(t, user.Id, "EPY-upstream", PaymentProviderAdtyEpay, common.TopUpStatusPending)

	_, err := RechargeEpay("EPY-upstream", "alipay", decimal.Zero, "")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)
	err = ManualCompleteTopUp("EPY-upstream", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Recharge Audit")
	assert.Equal(t, common.TopUpStatusPending, topUpStatus(t, db, "EPY-upstream"))

	_, err = CompleteAdtyTopUp("EPY-upstream", AdtyPaymentFacts{
		Provider: PaymentProviderAdtyEpay, Method: "alipay", PaidAmount: decimal.RequireFromString("730"),
		PaidCurrency: "CNY", Source: AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, 51_250_000, userQuota(t, db, user.Id))
}

func TestExpireAdtyTopUpsOnlyOwnPendingPastExpiry(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "expire", 0)
	past := seedAdtyOrder(t, user.Id, "EPU-past", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	require.NoError(t, db.Model(past).Update("expire_at", 100).Error)
	seedAdtyOrder(t, user.Id, "EPU-future", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	paid := seedAdtyOrder(t, user.Id, "EPU-paid", PaymentProviderAdtyBEpusdt, common.TopUpStatusSuccess)
	require.NoError(t, db.Model(paid).Update("expire_at", 100).Error)
	require.NoError(t, db.Create(&TopUp{UserId: user.Id, TradeNo: "UPSTREAM-1", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}).Error)

	expired, err := ExpireAdtyTopUps(common.GetTimestamp(), 50)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	assert.Equal(t, "EPU-past", expired[0].TradeNo)
	assert.Equal(t, common.TopUpStatusExpired, topUpStatus(t, db, "EPU-past"))
	assert.Equal(t, common.TopUpStatusPending, topUpStatus(t, db, "EPU-future"))
	assert.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "EPU-paid"))
	assert.Equal(t, common.TopUpStatusPending, topUpStatus(t, db, "UPSTREAM-1"))
}

func TestRefundAdtyTopUp(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "refund", 0)
	seedAdtyOrder(t, user.Id, "EPU-refund", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	_, err := refundAdtyTopUp(t, "EPU-refund", "chargeback", 1, "refund-key-1")
	require.Error(t, err, "pending orders cannot be refunded")

	_, err = CompleteAdtyTopUp("EPU-refund", usdtFacts("EPU-refund", "50.01"), "")
	require.NoError(t, err)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", 1_000_000).Error)

	// 余额 1,000,000 不到赠金 1,250,000：扣光余额，本金一分不退，余额不变负。
	first, err := refundAdtyTopUp(t, "EPU-refund", "chargeback", 1, "refund-key-1")
	require.NoError(t, err)
	assert.EqualValues(t, -1_000_000, first.Ledger.DeltaQuota)
	assert.EqualValues(t, 1_000_000, first.Ledger.BalanceBefore)
	assert.EqualValues(t, 0, first.Ledger.BalanceAfter)
	assert.EqualValues(t, 1_000_000, first.DeductedQuota)
	assert.EqualValues(t, 1_250_000, first.BonusQuota)
	assert.EqualValues(t, 0, first.RefundQuota)
	assert.True(t, first.RefundAmount.IsZero(), "refund amount %s", first.RefundAmount)
	assert.Equal(t, "USDT", first.Currency)
	assert.Equal(t, 0, userQuota(t, db, user.Id))
	assert.Equal(t, TopUpStatusRefunded, topUpStatus(t, db, "EPU-refund"))

	again, err := refundAdtyTopUp(t, "EPU-refund", "chargeback", 1, "refund-key-1")
	require.NoError(t, err, "replaying the same key returns the first result")
	assert.Equal(t, first.Ledger.Id, again.Ledger.Id)
	assert.Equal(t, first.DeductedQuota, again.DeductedQuota)
	assert.Equal(t, first.BonusQuota, again.BonusQuota)
	assert.Equal(t, first.RefundQuota, again.RefundQuota)
	assert.True(t, first.RefundAmount.Equal(again.RefundAmount))
	assert.Equal(t, 0, userQuota(t, db, user.Id))

	seedAdtyOrder(t, user.Id, "EPU-other", PaymentProviderAdtyBEpusdt, common.TopUpStatusSuccess)
	_, err = refundAdtyTopUp(t, "EPU-other", "chargeback", 1, "refund-key-1")
	require.ErrorIs(t, err, ErrAdtyIdempotencyConflict)

	_, err = CompleteAdtyTopUp("EPU-refund", usdtFacts("EPU-refund", "50.01"), "")
	require.ErrorIs(t, err, ErrTopUpStatusInvalid, "a refunded order is never credited again")
}

// seedCreditedOrder 建一笔已到账的订单：面额 face 美元、赠金 bonus 美元（含优惠加赠 promo），实付 paid。
func seedCreditedOrder(t *testing.T, db *gorm.DB, userID int, tradeNo, provider, face, bonus, promo, paid string) {
	t.Helper()
	ext := seedAdtyOrder(t, userID, tradeNo, provider, common.TopUpStatusSuccess)
	faceUSD, bonusUSD, promoUSD := decimal.RequireFromString(face), decimal.RequireFromString(bonus), decimal.RequireFromString(promo)
	credit := faceUSD.Add(bonusUSD).Add(promoUSD).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart()
	currency := "USDT"
	if provider == PaymentProviderAdtyEpay {
		currency = "CNY"
	}
	require.NoError(t, db.Model(ext).Updates(map[string]any{
		"face_usd": faceUSD, "bonus_usd": bonusUSD, "promo_bonus_usd": promoUSD, "credit_quota": credit,
		"paid_amount": decimal.RequireFromString(paid), "paid_currency": currency,
	}).Error)
}

func TestRefundAdtyTopUpOnlyUnspentPrincipal(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	q := func(usd string) int64 {
		return decimal.RequireFromString(usd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	cases := []struct {
		name, provider, face, bonus, promo, paid string
		balance                                  int64
		deducted, bonusQuota, refundQuota        int64
		refund                                   string
	}{
		// 付 100 得 110：余额 80 → 扣 80、退 70；余额 5 → 扣 5、退 0；余额 110 → 扣 110、退 100。
		{name: "spent-30", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "10", promo: "0", paid: "100", balance: q("80"), deducted: q("80"), bonusQuota: q("10"), refundQuota: q("70"), refund: "70"},
		{name: "spent-105", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "10", promo: "0", paid: "100", balance: q("5"), deducted: q("5"), bonusQuota: q("10"), refundQuota: 0, refund: "0"},
		{name: "untouched", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "10", promo: "0", paid: "100", balance: q("110"), deducted: q("110"), bonusQuota: q("10"), refundQuota: q("100"), refund: "100"},
		// USDT 按网关给的小数位向下取整：14.123457 × 70 / 100 = 9.8864199 → 9.886419。
		{name: "usdt-floor", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "10", promo: "0", paid: "14.123457", balance: q("80"), deducted: q("80"), bonusQuota: q("10"), refundQuota: q("70"), refund: "9.886419"},
		// 网关只给 2 位小数：33.33 × 33.333333 / 100 = 11.1099… → 11.10。
		{name: "usdt-2dp", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "10", promo: "0", paid: "33.33", balance: q("43.333333"), deducted: q("43.333333"), bonusQuota: q("10"), refundQuota: q("33.333333"), refund: "11.1"},
		// 人民币 2 位小数向下取整：730 × 33.333333 / 100 = 243.3333… → 243.33。
		{name: "cny-floor", provider: PaymentProviderAdtyEpay, face: "100", bonus: "10", promo: "0", paid: "730", balance: q("43.333333"), deducted: q("43.333333"), bonusQuota: q("10"), refundQuota: q("33.333333"), refund: "243.33"},
		{name: "cny-full", provider: PaymentProviderAdtyEpay, face: "100", bonus: "10", promo: "0", paid: "730", balance: q("110"), deducted: q("110"), bonusQuota: q("10"), refundQuota: q("100"), refund: "730"},
		// 优惠码加赠也算赠金：100 + 2.5 + 7，余额 80 → 退 80 − 9.5 = 70.5。
		{name: "promo-bonus", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "2.5", promo: "7", paid: "100", balance: q("80"), deducted: q("80"), bonusQuota: q("9.5"), refundQuota: q("70.5"), refund: "70.5"},
		// 余额为 0 或负数：不扣、不退，余额不变，订单照样 refunded。
		{name: "zero-balance", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "10", promo: "0", paid: "100", balance: 0, deducted: 0, bonusQuota: q("10"), refundQuota: 0, refund: "0"},
		{name: "negative-balance", provider: PaymentProviderAdtyBEpusdt, face: "100", bonus: "10", promo: "0", paid: "100", balance: -q("3"), deducted: 0, bonusQuota: q("10"), refundQuota: 0, refund: "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := seedAdtyTopUpUser(t, db, "unspent-"+tc.name, 0)
			tradeNo := "RF-" + tc.name
			seedCreditedOrder(t, db, user.Id, tradeNo, tc.provider, tc.face, tc.bonus, tc.promo, tc.paid)
			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", tc.balance).Error)

			result, err := refundAdtyTopUp(t, tradeNo, "chargeback", 1, "key-"+tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.deducted, result.DeductedQuota, "R")
			assert.Equal(t, tc.bonusQuota, result.BonusQuota, "G")
			assert.Equal(t, tc.refundQuota, result.RefundQuota, "P")
			assert.True(t, result.RefundAmount.Equal(decimal.RequireFromString(tc.refund)), "refund %s, want %s", result.RefundAmount, tc.refund)
			assert.EqualValues(t, tc.balance-tc.deducted, userQuota(t, db, user.Id))
			assert.Equal(t, -tc.deducted, result.Ledger.DeltaQuota)
			assert.Equal(t, tc.balance-tc.deducted, result.Ledger.BalanceAfter)
			assert.Equal(t, TopUpStatusRefunded, topUpStatus(t, db, tradeNo))

			ext, err := GetAdtyTopupOrder(tradeNo)
			require.NoError(t, err)
			assert.Equal(t, tc.refundQuota, ext.RefundQuota)
			assert.True(t, ext.RefundAmount.Equal(result.RefundAmount))
			assert.NotZero(t, ext.RefundedAt)
		})
	}
}

func TestCheckinQuotaIsNotRefundedAsPrincipal(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	require.NoError(t, db.AutoMigrate(&Checkin{}))
	saved := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 5000, MaxQuota: 5000}
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = saved })

	q := func(usd string) int64 {
		return decimal.RequireFromString(usd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	user := seedAdtyTopUpUser(t, db, "checkin-refund", 0)
	seedCreditedOrder(t, db, user.Id, "RF-checkin", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", q("110")).Error)
	record, err := UserCheckin(user.Id)
	require.NoError(t, err)
	require.Equal(t, 5000, record.QuotaAwarded)

	reserved, err := TryReserveUserQuota(user.Id, int(q("100")))
	require.NoError(t, err)
	require.True(t, reserved)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, int(q("10"))+5000, stored.Quota)
	require.Equal(t, 5000, stored.CheckinQuota, "花掉本金后签到赠送还在")

	result, err := refundAdtyTopUp(t, "RF-checkin", "chargeback", 1, "key-checkin")
	require.NoError(t, err)
	assert.Equal(t, q("10"), result.DeductedQuota)
	assert.EqualValues(t, 0, result.RefundQuota)
	assert.True(t, result.RefundAmount.IsZero(), "refund %s", result.RefundAmount)
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, 5000, stored.Quota)
	assert.Equal(t, 5000, stored.CheckinQuota)

	untouched := seedAdtyTopUpUser(t, db, "checkin-full", 0)
	seedCreditedOrder(t, db, untouched.Id, "RF-checkin-full", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	require.NoError(t, db.Model(&User{}).Where("id = ?", untouched.Id).Updates(map[string]any{
		"quota": q("110") + 5000, "checkin_quota": 5000,
	}).Error)
	full, err := refundAdtyTopUp(t, "RF-checkin-full", "chargeback", 1, "key-checkin-full")
	require.NoError(t, err)
	assert.Equal(t, q("110"), full.DeductedQuota)
	assert.Equal(t, q("100"), full.RefundQuota)
	var left User
	require.NoError(t, db.First(&left, untouched.Id).Error)
	assert.Equal(t, 5000, left.Quota)
	assert.Equal(t, 5000, left.CheckinQuota)
}

func TestRefundAdtyTopUpTakesOnlyThisOrderCredit(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	user := seedAdtyTopUpUser(t, db, "two-orders", 0)
	seedCreditedOrder(t, db, user.Id, "RF-two-a", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	seedCreditedOrder(t, db, user.Id, "RF-two-b", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", 110_000_000).Error)

	result, err := refundAdtyTopUp(t, "RF-two-a", "chargeback", 1, "two-a")
	require.NoError(t, err)
	assert.EqualValues(t, 55_000_000, result.DeductedQuota, "only this order's credit C is taken back")
	assert.EqualValues(t, 50_000_000, result.RefundQuota)
	assert.True(t, result.RefundAmount.Equal(decimal.NewFromInt(100)))
	assert.Equal(t, 55_000_000, userQuota(t, db, user.Id))
}

// 生产配置是 Redis 加批量更新（deploy/adty/docker-compose.yml）：消费先扣缓存，数据库要等批量刷新。
// 退款读余额必须以缓存为准，否则读到刷新前的旧余额，会多扣、多退，缓存余额还会变负。
func TestRefundAdtyTopUpUsesCachedBalanceUnderBatchUpdates(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	q := func(usd string) int64 {
		return decimal.RequireFromString(usd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	user := seedAdtyTopUpUser(t, db, "refund-batch", 0)
	seedCreditedOrder(t, db, user.Id, "RF-batch", PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", q("110")).Error)
	// 花掉 $30：缓存余额 $80，数据库还是 $110（批量还没刷）。
	reserved, err := TryReserveUserQuota(user.Id, int(q("30")))
	require.NoError(t, err)
	require.True(t, reserved)
	require.EqualValues(t, q("110"), userQuota(t, db, user.Id), "batch delta is not flushed yet")

	result, err := refundAdtyTopUp(t, "RF-batch", "chargeback", 1, "key-batch")
	require.NoError(t, err)
	assert.Equal(t, q("80"), result.DeductedQuota, "R comes from the live (cached) balance")
	assert.Equal(t, q("70"), result.RefundQuota)
	assert.True(t, result.RefundAmount.Equal(decimal.RequireFromString("70")), "refund %s", result.RefundAmount)
	cached, err := GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Zero(t, cached, "cached balance ends at 0, not negative")
	batchUpdate()
	assert.Zero(t, userQuota(t, db, user.Id), "database ends at 0 after the batch flush")
}

// 开了 Redis（不开批量）时走缓存扣回：三个例子和负余额的结果和数据库路径一样，缓存和数据库都落到同一个值。
func TestRefundAdtyTopUpWithRedis(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	q := func(usd string) int64 {
		return decimal.RequireFromString(usd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	cases := []struct {
		name                    string
		balance, deducted, back int64
		refund                  string
	}{
		{"spent-30", q("80"), q("80"), q("70"), "70"},
		{"spent-105", q("5"), q("5"), 0, "0"},
		{"untouched", q("110"), q("110"), q("100"), "100"},
		{"negative", -q("3"), 0, 0, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := seedAdtyTopUpUser(t, db, "redis-"+tc.name, 0)
			tradeNo := "RFR-" + tc.name
			seedCreditedOrder(t, db, user.Id, tradeNo, PaymentProviderAdtyBEpusdt, "100", "10", "0", "100")
			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", tc.balance).Error)
			_, err := GetUserCache(user.Id) // 缓存从数据库水合
			require.NoError(t, err)

			result, err := refundAdtyTopUp(t, tradeNo, "chargeback", 1, "key-redis-"+tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.deducted, result.DeductedQuota, "R")
			assert.Equal(t, tc.back, result.RefundQuota, "P")
			assert.True(t, result.RefundAmount.Equal(decimal.RequireFromString(tc.refund)), "refund %s", result.RefundAmount)
			cached, err := GetUserQuota(user.Id, false)
			require.NoError(t, err)
			assert.EqualValues(t, tc.balance-tc.deducted, cached, "cache")
			assert.EqualValues(t, tc.balance-tc.deducted, userQuota(t, db, user.Id), "database")
			assert.Equal(t, tc.balance-tc.deducted, result.Ledger.BalanceAfter)

			again, err := refundAdtyTopUp(t, tradeNo, "chargeback", 1, "key-redis-"+tc.name)
			require.NoError(t, err)
			assert.Equal(t, result.DeductedQuota, again.DeductedQuota, "replay returns the same R")
			cached, err = GetUserQuota(user.Id, false)
			require.NoError(t, err)
			assert.EqualValues(t, tc.balance-tc.deducted, cached, "replay does not charge the cache again")
		})
	}
}
