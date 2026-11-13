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
	"github.com/stretchr/testify/require"
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
	require.NoError(t, model.MigrateAdtyTables())
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
	require.Contains(t, below.Body.String(), "Stripe payment is not available")

	quoted := callStripe(t, RequestStripeAmount, user.Id, `{"amount":500000,"payment_method":"stripe"}`)
	require.Contains(t, quoted.Body.String(), "Stripe payment is not available")

	over := callStripe(t, RequestStripeAmount, user.Id, `{"amount":5000500000,"payment_method":"stripe"}`)
	require.Contains(t, over.Body.String(), "Stripe payment is not available")
	partial := callStripe(t, RequestStripeAmount, user.Id, `{"amount":600000,"payment_method":"stripe"}`)
	require.Contains(t, partial.Body.String(), "Stripe payment is not available")
	paid := callStripe(t, RequestStripePay, user.Id, `{"amount":500000,"payment_method":"stripe"}`)
	require.Contains(t, paid.Body.String(), "Stripe payment is not available")
	var created int64
	require.NoError(t, db.Model(&model.TopUp{}).Where("user_id = ?", user.Id).Count(&created).Error)
	require.Zero(t, created)
}
