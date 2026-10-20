package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"gorm.io/gorm"
)

func TestSubscriptionChargeCoversPlan(t *testing.T) {
	ten := decimal.NewFromInt(10)
	require.True(t, subscriptionChargeCoversPlan(10, "USD", "usd", ten))
	require.True(t, subscriptionChargeCoversPlan(10, "USD", "USD", decimal.RequireFromString("10.80")))
	require.False(t, subscriptionChargeCoversPlan(10, "USD", "USD", decimal.RequireFromString("9.99")))
	require.False(t, subscriptionChargeCoversPlan(10, "USD", "EUR", ten))
	require.False(t, subscriptionChargeCoversPlan(10, "USD", "", ten))
	require.True(t, subscriptionChargeCoversPlan(10, "CNY", "RMB", ten))
	require.True(t, subscriptionChargeCoversPlan(0, "USD", "USD", decimal.Zero))

	jpy, ok := minorUnitsToMajor(1000, "jpy")
	require.True(t, ok)
	require.True(t, jpy.Equal(decimal.NewFromInt(1000)))
	usd, ok := minorUnitsToMajor(1000, "USD")
	require.True(t, ok)
	require.True(t, usd.Equal(decimal.RequireFromString("10")))
	_, ok = minorUnitsToMajor(1000, "XXX")
	require.False(t, ok)
}

func TestStripeSubscriptionFulfillRejectsUnderpay(t *testing.T) {
	db := openSubscriptionAmountDB(t)
	require.NoError(t, db.Create(&model.SubscriptionPlan{
		Title: "pro", PriceAmount: 10, Currency: "USD", Enabled: true,
	}).Error)
	var plan model.SubscriptionPlan
	require.NoError(t, db.First(&plan).Error)
	require.NoError(t, db.Create(&model.SubscriptionOrder{
		UserId: plan.Id, PlanId: plan.Id, Money: 10, TradeNo: "SUB-STRIPE-SHORT",
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending,
	}).Error)

	fulfillOrder(t.Context(), stripeAmountEvent(100, "usd"), "SUB-STRIPE-SHORT", "cus_1", "203.0.113.8")

	var got model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", "SUB-STRIPE-SHORT").First(&got).Error)
	require.Equal(t, common.TopUpStatusPending, got.Status)
}

func TestStripeSubscriptionReplaySkipsAmountCheck(t *testing.T) {
	db := openSubscriptionAmountDB(t)
	require.NoError(t, db.Create(&model.SubscriptionOrder{
		UserId: 1, PlanId: 1, Money: 10, TradeNo: "SUB-STRIPE-DONE",
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusSuccess,
	}).Error)

	fulfillOrder(t.Context(), stripeAmountEvent(100, "usd"), "SUB-STRIPE-DONE", "cus_1", "203.0.113.8")

	var got model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", "SUB-STRIPE-DONE").First(&got).Error)
	require.Equal(t, common.TopUpStatusSuccess, got.Status)
}

func openSubscriptionAmountDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionOrder{}))
	prevDB := model.DB
	prevMain, prevLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = prevDB
		common.SetDatabaseTypes(prevMain, prevLog)
	})
	return db
}

func stripeAmountEvent(amountTotal int64, currency string) stripe.Event {
	return stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"amount_total": float64(amountTotal),
		"currency":     currency,
	}}}
}
