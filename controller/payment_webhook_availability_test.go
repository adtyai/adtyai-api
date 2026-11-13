package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func confirmPaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func TestRetiredHostedTopUpAndWebhooksStayClosedEvenWhenConfigured(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalStripeAPISecret := setting.StripeApiSecret
	originalStripeWebhookSecret := setting.StripeWebhookSecret
	originalStripePriceID := setting.StripePriceId
	originalCreemAPIKey := setting.CreemApiKey
	originalCreemProducts := setting.CreemProducts
	originalCreemWebhookSecret := setting.CreemWebhookSecret
	originalWaffoEnabled := setting.WaffoEnabled
	originalWaffoSandbox := setting.WaffoSandbox
	originalWaffoAPIKey := setting.WaffoApiKey
	originalWaffoPrivateKey := setting.WaffoPrivateKey
	originalWaffoPublicCert := setting.WaffoPublicCert
	originalWaffoSandboxAPIKey := setting.WaffoSandboxApiKey
	originalWaffoSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	originalWaffoSandboxPublicCert := setting.WaffoSandboxPublicCert
	originalPancakeMerchantID := setting.WaffoPancakeMerchantID
	originalPancakePrivateKey := setting.WaffoPancakePrivateKey
	originalPancakeProductID := setting.WaffoPancakeProductID
	originalPancakeEnvironment := setting.WaffoPancakeEnvironment
	t.Cleanup(func() {
		setting.StripeApiSecret = originalStripeAPISecret
		setting.StripeWebhookSecret = originalStripeWebhookSecret
		setting.StripePriceId = originalStripePriceID
		setting.CreemApiKey = originalCreemAPIKey
		setting.CreemProducts = originalCreemProducts
		setting.CreemWebhookSecret = originalCreemWebhookSecret
		setting.WaffoEnabled = originalWaffoEnabled
		setting.WaffoSandbox = originalWaffoSandbox
		setting.WaffoApiKey = originalWaffoAPIKey
		setting.WaffoPrivateKey = originalWaffoPrivateKey
		setting.WaffoPublicCert = originalWaffoPublicCert
		setting.WaffoSandboxApiKey = originalWaffoSandboxAPIKey
		setting.WaffoSandboxPrivateKey = originalWaffoSandboxPrivateKey
		setting.WaffoSandboxPublicCert = originalWaffoSandboxPublicCert
		setting.WaffoPancakeMerchantID = originalPancakeMerchantID
		setting.WaffoPancakePrivateKey = originalPancakePrivateKey
		setting.WaffoPancakeProductID = originalPancakeProductID
		setting.WaffoPancakeEnvironment = originalPancakeEnvironment
	})

	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	setting.StripePriceId = "price_123"
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	setting.CreemWebhookSecret = "creem_secret"
	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = "api"
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	setting.WaffoSandboxApiKey = "sandbox_api"
	setting.WaffoSandboxPrivateKey = "sandbox_private"
	setting.WaffoSandboxPublicCert = "sandbox_public"
	setting.WaffoPancakeMerchantID = "merchant"
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeProductID = "product"
	setting.WaffoPancakeEnvironment = "test"

	require.False(t, isStripeTopUpEnabled())
	require.False(t, isStripeWebhookEnabled())
	require.False(t, isCreemTopUpEnabled())
	require.False(t, isCreemWebhookEnabled())
	require.False(t, isWaffoTopUpEnabled())
	require.False(t, isWaffoWebhookEnabled())
	require.False(t, isWaffoPancakeTopUpEnabled())
	require.False(t, isWaffoPancakeWebhookEnabled())
}

func TestGetTopUpInfoDoesNotAdvertiseRetiredHostedMethods(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	gin.SetMode(gin.TestMode)
	originalStripeAPISecret := setting.StripeApiSecret
	originalStripeWebhookSecret := setting.StripeWebhookSecret
	originalStripePriceID := setting.StripePriceId
	originalCreemAPIKey := setting.CreemApiKey
	originalCreemProducts := setting.CreemProducts
	originalCreemWebhookSecret := setting.CreemWebhookSecret
	originalWaffoEnabled := setting.WaffoEnabled
	originalWaffoSandbox := setting.WaffoSandbox
	originalWaffoAPIKey := setting.WaffoApiKey
	originalWaffoPrivateKey := setting.WaffoPrivateKey
	originalWaffoPublicCert := setting.WaffoPublicCert
	originalPancakeMerchantID := setting.WaffoPancakeMerchantID
	originalPancakePrivateKey := setting.WaffoPancakePrivateKey
	originalPancakeProductID := setting.WaffoPancakeProductID
	originalPancakeEnvironment := setting.WaffoPancakeEnvironment
	t.Cleanup(func() {
		setting.StripeApiSecret = originalStripeAPISecret
		setting.StripeWebhookSecret = originalStripeWebhookSecret
		setting.StripePriceId = originalStripePriceID
		setting.CreemApiKey = originalCreemAPIKey
		setting.CreemProducts = originalCreemProducts
		setting.CreemWebhookSecret = originalCreemWebhookSecret
		setting.WaffoEnabled = originalWaffoEnabled
		setting.WaffoSandbox = originalWaffoSandbox
		setting.WaffoApiKey = originalWaffoAPIKey
		setting.WaffoPrivateKey = originalWaffoPrivateKey
		setting.WaffoPublicCert = originalWaffoPublicCert
		setting.WaffoPancakeMerchantID = originalPancakeMerchantID
		setting.WaffoPancakePrivateKey = originalPancakePrivateKey
		setting.WaffoPancakeProductID = originalPancakeProductID
		setting.WaffoPancakeEnvironment = originalPancakeEnvironment
	})
	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	setting.StripePriceId = "price_123"
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123","name":"Pack","price":2,"currency":"USD","quota":1000}]`
	setting.CreemWebhookSecret = "creem_secret"
	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = "api"
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	setting.WaffoPancakeMerchantID = "merchant"
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeProductID = "product"
	setting.WaffoPancakeEnvironment = "test"

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body struct {
		Success bool
		Data    struct {
			EnableStripeTopup       bool `json:"enable_stripe_topup"`
			EnableCreemTopup        bool `json:"enable_creem_topup"`
			EnableWaffoTopup        bool `json:"enable_waffo_topup"`
			EnableWaffoPancakeTopup bool `json:"enable_waffo_pancake_topup"`
		}
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.False(t, body.Data.EnableStripeTopup)
	require.False(t, body.Data.EnableCreemTopup)
	require.False(t, body.Data.EnableWaffoTopup)
	require.False(t, body.Data.EnableWaffoPancakeTopup)
}

func TestEpayWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.False(t, isEpayWebhookEnabled())

	operation_setting.EpayKey = "epay_key"
	require.True(t, isEpayWebhookEnabled())

	operation_setting.PayMethods = nil
	require.True(t, isEpayWebhookEnabled())
	require.False(t, isEpayTopUpEnabled())
}

func TestEpayCallbacksVerifyBeforeSettlement(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	originalServerAddress := system_setting.ServerAddress
	originalDB := model.DB
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
		system_setting.ServerAddress = originalServerAddress
		model.DB = originalDB
	})
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "test-key"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	system_setting.ServerAddress = "https://dashboard.example.com"
	// Rejected callbacks and valid non-success events must never reach settlement.
	// A nil database makes an accidental attempt to settle fail the test.
	model.DB = nil
	require.True(t, isEpayWebhookEnabled())

	// Both ambiguous maps below had this same valid signing string in v0.0.4.
	// This must fail because of parameter ambiguity, not a mismatched signature.
	ambiguousSign := epay.MD5String("money=1.00&name=item&out_trade_no=other&out_trade_no=order&pid=epay_id&trade_status=TRADE_SUCCESS&type=alipay", "test-key")
	pendingSign := epay.MD5String("money=1.00&name=item%26other&out_trade_no=order&pid=epay_id&trade_status=WAIT_BUYER_PAY&type=alipay", "test-key")
	cases := []struct {
		name         string
		productName  string
		tradeNo      string
		status       string
		sign         string
		validPending bool
	}{
		{"separator in product name", "item&out_trade_no=other", "order", epay.StatusTradeSuccess, ambiguousSign, false},
		{"separator in order number", "item", "other&out_trade_no=order", epay.StatusTradeSuccess, ambiguousSign, false},
		{"invalid signature", "item", "order", epay.StatusTradeSuccess, "invalid", false},
		{"missing signature", "item", "order", epay.StatusTradeSuccess, "", false},
		{"valid pending with literal percent escape", "item%26other", "order", "WAIT_BUYER_PAY", pendingSign, true},
	}
	endpoints := []struct {
		name             string
		path             string
		handler          gin.HandlerFunc
		status           int
		rejectedBody     string
		pendingBody      string
		rejectedLocation string
		pendingLocation  string
	}{
		{
			name: "topup notify", path: "/api/user/epay/notify", handler: EpayNotify,
			status: http.StatusOK, rejectedBody: "fail", pendingBody: "success",
		},
	}
	router := gin.New()
	for _, endpoint := range endpoints {
		router.Any(endpoint.path, endpoint.handler)
	}
	for _, endpoint := range endpoints {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			for _, tc := range cases {
				t.Run(endpoint.name+"/"+method+"/"+tc.name, func(t *testing.T) {
					params := url.Values{
						"pid": {"epay_id"}, "type": {"alipay"}, "money": {"1.00"},
						"name": {tc.productName}, "out_trade_no": {tc.tradeNo},
						"trade_status": {tc.status}, "sign_type": {"MD5"},
					}
					if tc.sign != "" {
						params.Set("sign", tc.sign)
					}
					request := httptest.NewRequest(method, endpoint.path+"?"+params.Encode(), nil)
					if method == http.MethodPost {
						request = httptest.NewRequest(method, endpoint.path, strings.NewReader(params.Encode()))
						request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					}
					recorder := httptest.NewRecorder()
					require.NotPanics(t, func() { router.ServeHTTP(recorder, request) })
					assert.Equal(t, endpoint.status, recorder.Code)
					wantBody, wantLocation := endpoint.rejectedBody, endpoint.rejectedLocation
					if tc.validPending {
						wantBody, wantLocation = endpoint.pendingBody, endpoint.pendingLocation
					}
					if wantBody != "" {
						assert.Equal(t, wantBody, recorder.Body.String())
					}
					assert.Equal(t, wantLocation, recorder.Header().Get("Location"))
				})
			}
		}
	}
}
