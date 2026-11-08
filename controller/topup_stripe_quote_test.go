package controller

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
	"gorm.io/gorm"
)

func TestStripeQuoteChargeAndCreditUseOneAmount(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	previousRedis := common.RedisEnabled
	previousRDB := common.RDB
	server := miniredis.RunT(t)
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = previousRDB
		common.RedisEnabled = previousRedis
	})
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.AdtyTopupOrder{}, &model.AdtyBalanceLedger{}, &model.AdtyUserFingerprint{}))
	require.NoError(t, model.MigrateAdtyTables())
	user := &model.User{
		Username: "stripe-quote",
		Password: "x",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "f066",
		Quota:    0,
	}
	require.NoError(t, db.Create(user).Error)

	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousUnit := common.QuotaPerUnit
	previousMin := setting.StripeMinTopUp
	previousKey := setting.StripeApiSecret
	previousWebhook := setting.StripeWebhookSecret
	previousPrice := setting.StripePriceId
	previousUnitPrice := setting.StripeUnitPrice
	previousSession := stripeNewCheckoutSession
	previousRatio := common.TopupGroupRatio2JSONString()
	previousDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	maps.Copy(previousDiscounts, operation_setting.GetPaymentSetting().AmountDiscount)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	common.QuotaPerUnit = 500000
	setting.StripeMinTopUp = 1
	setting.StripeApiSecret = "sk_test_f066"
	setting.StripeWebhookSecret = "whsec_f066"
	setting.StripePriceId = "price_f066"
	setting.StripeUnitPrice = 8
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{2: 0.5}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"f066":2}`))
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
		common.QuotaPerUnit = previousUnit
		setting.StripeMinTopUp = previousMin
		setting.StripeApiSecret = previousKey
		setting.StripeWebhookSecret = previousWebhook
		setting.StripePriceId = previousPrice
		setting.StripeUnitPrice = previousUnitPrice
		stripeNewCheckoutSession = previousSession
		operation_setting.GetPaymentSetting().AmountDiscount = previousDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(previousRatio))
	})

	quoted := callStripe(t, RequestStripeAmount, user.Id, `{"amount":2,"payment_method":"stripe"}`)
	require.Contains(t, quoted.Body.String(), `"data":"16.00"`)

	var unitAmount int64
	stripeNewCheckoutSession = func(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
		require.NotNil(t, params.LineItems[0].PriceData)
		require.Equal(t, int64(1), *params.LineItems[0].Quantity)
		unitAmount = *params.LineItems[0].PriceData.UnitAmount
		return &stripe.CheckoutSession{ID: "cs_1600", URL: "https://pay.example/stripe-f066"}, nil
	}
	paid := callStripe(t, RequestStripePay, user.Id, `{"amount":2,"payment_method":"stripe"}`)
	require.Contains(t, paid.Body.String(), "https://pay.example/stripe-f066")
	require.Equal(t, int64(1600), unitAmount)

	var order model.TopUp
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&order).Error)
	require.Equal(t, int64(2), order.Amount)
	require.True(t, decimal.NewFromFloat(order.Money).Equal(decimal.NewFromInt(16)))
	require.Equal(t, common.TopUpStatusPending, order.Status)
	frozen, err := model.GetAdtyTopupOrder(order.TradeNo)
	require.NoError(t, err)
	require.NotNil(t, frozen, "hosted checkout must persist the same settlement snapshot as other payment providers")
	require.True(t, frozen.FaceUSD.Equal(decimal.NewFromInt(2)))
	require.EqualValues(t, 1_000_000, frozen.CreditQuota)
	require.Equal(t, "USD", frozen.PayCurrency)

	mismatch := postStripeCheckoutWebhook(t, order.TradeNo, 1599)
	require.Equal(t, http.StatusInternalServerError, mismatch.Code)
	require.NoError(t, db.First(&order, order.Id).Error)
	require.Equal(t, common.TopUpStatusPending, order.Status)
	require.Equal(t, 0, reloadUserQuota(t, db, user.Id))

	matched := postStripeCheckoutWebhook(t, order.TradeNo, 1600)
	require.Equal(t, http.StatusOK, matched.Code)
	require.NoError(t, db.First(&order, order.Id).Error)
	require.Equal(t, common.TopUpStatusSuccess, order.Status)
	require.Equal(t, 1000000, reloadUserQuota(t, db, user.Id))

	again := callStripe(t, RequestStripePay, user.Id, `{"amount":2,"payment_method":"stripe"}`)
	require.Contains(t, again.Body.String(), "https://pay.example/stripe-f066")
	var manual model.TopUp
	require.NoError(t, db.Where("user_id = ? AND status = ?", user.Id, common.TopUpStatusPending).First(&manual).Error)
	_, err = model.CompleteAdtyTopUp(manual.TradeNo, model.AdtyPaymentFacts{Provider: model.PaymentProviderStripe, Source: model.AdtyPaymentSourceManual}, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 2000000, reloadUserQuota(t, db, user.Id))
}

func postStripeCheckoutWebhook(t *testing.T, tradeNo string, amountTotal int) *httptest.ResponseRecorder {
	t.Helper()
	payload := []byte(fmt.Sprintf(`{"id":"evt_%s_%d","object":"event","type":"checkout.session.completed","data":{"object":{"id":"cs_%d","object":"checkout.session","client_reference_id":"%s","customer":"cus_f066","status":"complete","payment_status":"paid","amount_total":%d,"currency":"usd","metadata":{"adty_product":"wallet_topup"}}}}`, tradeNo, amountTotal, amountTotal, tradeNo, amountTotal))
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: setting.StripeWebhookSecret})
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", io.NopCloser(bytes.NewReader(signed.Payload)))
	req.Header.Set("Stripe-Signature", signed.Header)
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/api/stripe/webhook", StripeWebhook)
	engine.ServeHTTP(recorder, req)
	return recorder
}

func reloadUserQuota(t *testing.T, db *gorm.DB, userID int) int {
	t.Helper()
	var stored model.User
	require.NoError(t, db.First(&stored, userID).Error)
	return stored.Quota
}
