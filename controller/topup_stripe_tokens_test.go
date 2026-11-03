package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
)

func callStripe(t *testing.T, handler gin.HandlerFunc, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/amount", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	handler(ctx)
	return recorder
}

func TestStripeTokenDisplayLimitsMatchCheckoutQuantity(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
	user := &model.User{Username: "stripe-tokens", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(user).Error)

	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousUnit := common.QuotaPerUnit
	previousMin := setting.StripeMinTopUp
	previousKey := setting.StripeApiSecret
	previousWebhook := setting.StripeWebhookSecret
	previousPrice := setting.StripePriceId
	previousSession := stripeNewCheckoutSession
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	common.QuotaPerUnit = 500000
	setting.StripeMinTopUp = 1
	setting.StripeApiSecret = "sk_test_f065"
	setting.StripeWebhookSecret = "whsec_f065"
	setting.StripePriceId = "price_f065"
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
		common.QuotaPerUnit = previousUnit
		setting.StripeMinTopUp = previousMin
		setting.StripeApiSecret = previousKey
		setting.StripeWebhookSecret = previousWebhook
		setting.StripePriceId = previousPrice
		stripeNewCheckoutSession = previousSession
	})

	below := callStripe(t, RequestStripeAmount, user.Id, `{"amount":100,"payment_method":"stripe"}`)
	require.Contains(t, below.Body.String(), "Top-up amount cannot be less than")
	require.Contains(t, below.Body.String(), "500000")

	quoted := callStripe(t, RequestStripeAmount, user.Id, `{"amount":500000,"payment_method":"stripe"}`)
	require.Contains(t, quoted.Body.String(), `"message":"success"`)
	require.NotContains(t, quoted.Body.String(), "cannot exceed")

	over := callStripe(t, RequestStripeAmount, user.Id, `{"amount":5000500000,"payment_method":"stripe"}`)
	require.Contains(t, over.Body.String(), "Top-up amount cannot exceed")
	require.Contains(t, over.Body.String(), "5000000000")

	partial := callStripe(t, RequestStripeAmount, user.Id, `{"amount":600000,"payment_method":"stripe"}`)
	require.Contains(t, partial.Body.String(), "whole number of quota units")

	var unitAmount int64
	stripeNewCheckoutSession = func(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
		require.NotNil(t, params.LineItems[0].PriceData)
		require.Equal(t, int64(1), *params.LineItems[0].Quantity)
		unitAmount = *params.LineItems[0].PriceData.UnitAmount
		return &stripe.CheckoutSession{URL: "https://pay.example/checkout"}, nil
	}
	paid := callStripe(t, RequestStripePay, user.Id, `{"amount":500000,"payment_method":"stripe"}`)
	require.Contains(t, paid.Body.String(), "https://pay.example/checkout")
	require.Equal(t, int64(800), unitAmount)
	var order model.TopUp
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&order).Error)
	require.Equal(t, int64(1), order.Amount)
	require.True(t, decimal.NewFromFloat(order.Money).Equal(decimal.NewFromInt(8)))
}
