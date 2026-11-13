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
	"github.com/stretchr/testify/require"
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
	require.Contains(t, quoted.Body.String(), "Stripe payment is not available")
	paid := callStripe(t, RequestStripePay, user.Id, `{"amount":2,"payment_method":"stripe"}`)
	require.Contains(t, paid.Body.String(), "Stripe payment is not available")
	var created int64
	require.NoError(t, db.Model(&model.TopUp{}).Where("user_id = ?", user.Id).Count(&created).Error)
	require.Zero(t, created)
	require.Zero(t, reloadUserQuota(t, db, user.Id))

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
