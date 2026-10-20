package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTopupGatewayChargeUsesSnapshotNotCreditBase(t *testing.T) {
	ten := decimal.RequireFromString("10.00")
	require.True(t, topupGatewayChargeCovers("10.00", "USD", "usd", ten))
	require.True(t, topupGatewayChargeCovers("10.00", "USD", "USD", decimal.RequireFromString("10.80")))
	require.False(t, topupGatewayChargeCovers("10.00", "USD", "USD", decimal.RequireFromString("9.99")))
	require.False(t, topupGatewayChargeCovers("10.00", "USD", "EUR", ten))
	require.False(t, topupGatewayChargeCovers("", "USD", "USD", ten))
	require.False(t, topupGatewayChargeCovers("10.00", "", "USD", ten))

	amount, currency, err := stripeExpectedCharge(800, 2, "usd")
	require.NoError(t, err)
	require.Equal(t, "16.00", amount)
	require.Equal(t, "USD", currency)
	_, _, err = stripeExpectedCharge(0, 1, "usd")
	require.Error(t, err)
	_, _, err = stripeExpectedCharge(100, 1, "XXX")
	require.Error(t, err)

	creemAmount, creemCurrency, err := creemExpectedCharge(10, "eur")
	require.NoError(t, err)
	require.Equal(t, "10.00", creemAmount)
	require.Equal(t, "EUR", creemCurrency)
	_, _, err = creemExpectedCharge(10, "")
	require.Error(t, err)

	paid, ok := minorUnitsToMajor(1000, "USD")
	require.True(t, ok)
	require.True(t, topupGatewayChargeCovers("10.00", "USD", "USD", paid))
	dollarsAsCents, ok := minorUnitsToMajor(10, "USD")
	require.True(t, ok)
	require.False(t, topupGatewayChargeCovers("10.00", "USD", "USD", dollarsAsCents))

	require.True(t, waffoCallbackCovers("12.50", "USD", "12.50", "USD", "12.50", "USD"))
	require.False(t, waffoCallbackCovers("12.50", "USD", "12.50", "USD", "10.00", "USD"))
	require.True(t, waffoCallbackCovers("12.50", "USD", "12.50", "USD", "1800", "JPY"))
	require.False(t, waffoCallbackCovers("12.50", "USD", "10.00", "USD", "", ""))
}

func TestStripeTopupFulfillRejectsUnderpayAndCreditsLegacySnapshot(t *testing.T) {
	db := openTopupAmountDB(t)
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "payer", Status: common.UserStatusEnabled, Quota: 100}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 7, Amount: 2, Money: 20, TradeNo: "STRIPE-TOPUP-SHORT",
		PaymentProvider: model.PaymentProviderStripe, ExpectedPayAmount: "10.00", ExpectedPayCurrency: "USD",
		Status: common.TopUpStatusPending,
	}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 7, Amount: 2, Money: 20, TradeNo: "STRIPE-TOPUP-OLD",
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending,
	}).Error)

	fulfillOrder(t.Context(), stripeAmountEvent(100, "usd"), "STRIPE-TOPUP-SHORT", "cus_1", "203.0.113.8")
	fulfillOrder(t.Context(), stripeAmountEvent(1000, "usd"), "STRIPE-TOPUP-OLD", "cus_1", "203.0.113.8")

	var short, old model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "STRIPE-TOPUP-SHORT").First(&short).Error)
	require.NoError(t, db.Where("trade_no = ?", "STRIPE-TOPUP-OLD").First(&old).Error)
	require.Equal(t, common.TopUpStatusPending, short.Status)
	require.Equal(t, common.TopUpStatusSuccess, old.Status)

	var user model.User
	require.NoError(t, db.First(&user, 7).Error)
	require.Greater(t, user.Quota, 100)
}

func TestStripeTopupFulfillCreditsWhenGatewayCoversSnapshot(t *testing.T) {
	db := openTopupAmountDB(t)
	require.NoError(t, db.Create(&model.User{Id: 8, Username: "payer-ok", Status: common.UserStatusEnabled, Quota: 100}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 8, Amount: 2, Money: 2, TradeNo: "STRIPE-TOPUP-OK",
		PaymentMethod: model.PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe,
		ExpectedPayAmount: "16.00", ExpectedPayCurrency: "USD", Status: common.TopUpStatusPending,
	}).Error)

	fulfillOrder(t.Context(), stripeAmountEvent(1600, "usd"), "STRIPE-TOPUP-OK", "cus_ok", "203.0.113.9")

	var got model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "STRIPE-TOPUP-OK").First(&got).Error)
	require.Equal(t, common.TopUpStatusSuccess, got.Status)
	var user model.User
	require.NoError(t, db.First(&user, 8).Error)
	require.Greater(t, user.Quota, 100)
}

func TestStripeSubscriptionCheckErrorRetriesInsteadOfAck(t *testing.T) {
	db := openTopupAmountDB(t)
	require.NoError(t, db.Create(&model.SubscriptionOrder{
		UserId: 8, PlanId: 404, Money: 10, TradeNo: "STRIPE-SUB-MISSING-PLAN",
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending,
	}).Error)

	err := fulfillOrder(context.Background(), stripeAmountEvent(1000, "usd"), "STRIPE-SUB-MISSING-PLAN", "cus", "127.0.0.1")
	require.Error(t, err)
	var got model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", "STRIPE-SUB-MISSING-PLAN").First(&got).Error)
	require.Equal(t, common.TopUpStatusPending, got.Status)
}

func TestDecideTopupPayCallbackSplitsSnapshotAndLegacy(t *testing.T) {
	ten := decimal.RequireFromString("10.00")
	require.Equal(t, topupCallbackAckNoCredit, decideTopupPayCallback("10.00", "USD", true, decimal.RequireFromString("9.99"), false))
	require.Equal(t, topupCallbackCredit, decideTopupPayCallback("10.00", "USD", true, ten, true))
	require.Equal(t, topupCallbackCredit, decideTopupPayCallback("", "", true, ten, false))
	require.Equal(t, topupCallbackRetry, decideTopupPayCallback("", "", true, decimal.Zero, false))
	require.Equal(t, topupCallbackRetry, decideTopupPayCallback("", "", false, decimal.Zero, false))
}

func TestCreemTopupRejectsDollarInterpretation(t *testing.T) {
	db := openTopupAmountDB(t)
	require.NoError(t, db.Create(&model.User{Id: 9, Username: "creem-payer", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 9, Amount: 1000, Money: 10, TradeNo: "CREEM-SHORT",
		PaymentProvider: model.PaymentProviderCreem, ExpectedPayAmount: "10.00", ExpectedPayCurrency: "USD",
		Status: common.TopUpStatusPending,
	}).Error)

	event := CreemWebhookEvent{Id: "evt_creem", EventType: "checkout.completed"}
	event.Object.RequestId = "CREEM-SHORT"
	event.Object.Order.Id = "ord_creem"
	event.Object.Order.Status = "paid"
	event.Object.Order.Type = "onetime"
	event.Object.Order.AmountPaid = 10
	event.Object.Order.Currency = "USD"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/api/creem/webhook", nil)
	handleCheckoutCompleted(c, &event)

	require.Equal(t, 200, recorder.Code)
	var got model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "CREEM-SHORT").First(&got).Error)
	require.Equal(t, common.TopUpStatusPending, got.Status)
}

func TestCreemLegacySnapshotCreditsPositivePaidAmount(t *testing.T) {
	db := openTopupAmountDB(t)
	require.NoError(t, db.Create(&model.User{Id: 11, Username: "creem-legacy", Status: common.UserStatusEnabled, Quota: 100}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 11, Amount: 1000, Money: 10, TradeNo: "CREEM-OLD",
		PaymentProvider: model.PaymentProviderCreem, Status: common.TopUpStatusPending,
	}).Error)

	event := CreemWebhookEvent{Id: "evt_creem_old", EventType: "checkout.completed"}
	event.Object.RequestId = "CREEM-OLD"
	event.Object.Order.Id = "ord_creem_old"
	event.Object.Order.Status = "paid"
	event.Object.Order.Type = "onetime"
	event.Object.Order.AmountPaid = 1000
	event.Object.Order.Currency = "USD"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/api/creem/webhook", nil)
	handleCheckoutCompleted(c, &event)

	require.Equal(t, 200, recorder.Code)
	var got model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "CREEM-OLD").First(&got).Error)
	require.Equal(t, common.TopUpStatusSuccess, got.Status)
	var user model.User
	require.NoError(t, db.First(&user, 11).Error)
	require.Greater(t, user.Quota, 100)
}

func TestPancakeLegacySnapshotRetriesWhenPaidAmountMissing(t *testing.T) {
	db := openTopupAmountDB(t)
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 12, Amount: 10, Money: 10, TradeNo: "PANCAKE-OLD",
		PaymentProvider: model.PaymentProviderWaffoPancake, Status: common.TopUpStatusPending,
	}).Error)

	require.Equal(t, topupCallbackCredit, pancakePendingTopupAction("PANCAKE-OLD", "12.50", "USD"))
	require.Equal(t, topupCallbackRetry, pancakePendingTopupAction("PANCAKE-OLD", "", "USD"))
	require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", "PANCAKE-OLD").Updates(map[string]any{
		"expected_pay_amount": "12.50", "expected_pay_currency": "USD",
	}).Error)
	require.Equal(t, topupCallbackAckNoCredit, pancakePendingTopupAction("PANCAKE-OLD", "1.00", "USD"))
}

func openTopupAmountDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.SubscriptionOrder{}, &model.Log{}, &model.LedgerEntry{}))
	prevDB, prevLogDB := model.DB, model.LOG_DB
	prevMain, prevLog := common.MainDatabaseType(), common.LogDatabaseType()
	prevRedis := common.RedisEnabled
	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = prevDB
		model.LOG_DB = prevLogDB
		common.SetDatabaseTypes(prevMain, prevLog)
		common.RedisEnabled = prevRedis
	})
	return db
}
