package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func capturePaymentWebhookLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	common.LogWriterMu.Lock()
	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter = buf
	gin.DefaultErrorWriter = buf
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldOut
		gin.DefaultErrorWriter = oldErr
		common.LogWriterMu.Unlock()
	})
	return buf
}

func postPaymentWebhook(handler gin.HandlerFunc, path, header, signature, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if header != "" {
		req.Header.Set(header, signature)
	}
	ctx.Request = req
	handler(ctx)
	return recorder
}

// TestPaymentWebhookLogsOmitSignatureAndBody drives the shipped webhook
// handlers and rejects any log line that keeps a signature or card payload.
func TestPaymentWebhookLogsOmitSignatureAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logs := capturePaymentWebhookLogs(t)
	signature := "whsec_do_not_log_this_signature"
	card := "card_do_not_log_411111"
	body := `{"event":"checkout.completed","card":"` + card + `"}`

	savedSecret, savedTest := setting.CreemWebhookSecret, setting.CreemTestMode
	savedKey, savedProducts := setting.CreemApiKey, setting.CreemProducts
	t.Cleanup(func() {
		setting.CreemWebhookSecret, setting.CreemTestMode = savedSecret, savedTest
		setting.CreemApiKey, setting.CreemProducts = savedKey, savedProducts
	})
	setting.CreemWebhookSecret = ""
	setting.CreemTestMode = false
	require.False(t, verifyCreemSignature(body, signature, ""))

	disabled := postPaymentWebhook(CreemWebhook, "/api/creem/webhook", CreemSignatureHeader, signature, body)
	waffo := postPaymentWebhook(WaffoWebhook, "/api/waffo/webhook", "X-SIGNATURE", signature, body)
	pancake := postPaymentWebhook(WaffoPancakeWebhook, "/api/waffo-pancake/webhook/test", "X-Waffo-Signature", signature, body)
	assert.Equal(t, http.StatusForbidden, disabled.Code)
	assert.Equal(t, http.StatusForbidden, waffo.Code)
	assert.Equal(t, http.StatusForbidden, pancake.Code)

	confirmPaymentComplianceForTest(t)
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	setting.CreemWebhookSecret = "creem_webhook_secret"
	require.True(t, isCreemWebhookEnabled())

	missing := postPaymentWebhook(CreemWebhook, "/api/creem/webhook", "", "", body)
	assert.Equal(t, http.StatusUnauthorized, missing.Code)

	signedBody := `{"card":"` + card + `"`
	signed := generateCreemSignature(signedBody, setting.CreemWebhookSecret)
	parsed := postPaymentWebhook(CreemWebhook, "/api/creem/webhook", CreemSignatureHeader, signed, signedBody)
	assert.Equal(t, http.StatusBadRequest, parsed.Code)

	logged := logs.String()
	assert.NotContains(t, logged, signature)
	assert.NotContains(t, logged, signed)
	assert.NotContains(t, logged, card)
	assert.NotContains(t, logged, body)
	assert.NotContains(t, logged, signedBody)
	assert.Contains(t, logged, "Creem webhook secret is not configured")
	assert.Contains(t, logged, "webhook_disabled")
	assert.Contains(t, logged, "missing signature")
	assert.Contains(t, logged, "failed to parse")

	savedStripeKey := setting.StripeApiSecret
	savedStripeHook := setting.StripeWebhookSecret
	savedStripePrice := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = savedStripeKey
		setting.StripeWebhookSecret = savedStripeHook
		setting.StripePriceId = savedStripePrice
	})
	setting.StripeApiSecret = "sk_test_do_not_log"
	setting.StripeWebhookSecret = "whsec_stripe_secret_do_not_log"
	setting.StripePriceId = "price_test"
	require.True(t, isStripeWebhookEnabled())
	stripeResp := postPaymentWebhook(StripeWebhook, "/api/stripe/webhook", "Stripe-Signature", signature, body)
	assert.NotEqual(t, http.StatusOK, stripeResp.Code)
	logged = logs.String()
	assert.NotContains(t, logged, signature)
	assert.NotContains(t, logged, card)
	assert.NotContains(t, logged, setting.StripeWebhookSecret)
	assert.NotContains(t, logged, setting.StripeApiSecret)
}
