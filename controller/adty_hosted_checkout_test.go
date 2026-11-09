package controller

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	waffoutils "github.com/waffo-com/waffo-go/utils"
	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

// Only the provider HTTP boundary and its public trust anchor are replaced.
// The SDK, signature verification, order matching and wallet transaction run normally.
func TestPancakeHostedTokensCheckoutAndSignedFrozenFacts(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	user := model.User{Username: "pancake-hosted-tokens", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	oldMerchant, oldKey, oldProduct, oldStore, oldEnv := setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID, setting.WaffoPancakeStoreID, setting.WaffoPancakeEnvironment
	oldPrice, oldMin, oldUnit, oldRedis := setting.WaffoPancakeUnitPrice, setting.WaffoPancakeMinTopUp, common.QuotaPerUnit, common.RedisEnabled
	oldDisplay, oldRates, oldTransport, oldVerifier := operation_setting.GetGeneralSetting().QuotaDisplayType, adty_setting.GetTopupSetting().BonusRates, http.DefaultTransport, verifyWaffoPancakeWebhook
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID, setting.WaffoPancakeStoreID, setting.WaffoPancakeEnvironment = oldMerchant, oldKey, oldProduct, oldStore, oldEnv
		setting.WaffoPancakeUnitPrice, setting.WaffoPancakeMinTopUp, common.QuotaPerUnit, common.RedisEnabled = oldPrice, oldMin, oldUnit, oldRedis
		operation_setting.GetGeneralSetting().QuotaDisplayType, adty_setting.GetTopupSetting().BonusRates, http.DefaultTransport, verifyWaffoPancakeWebhook = oldDisplay, oldRates, oldTransport, oldVerifier
	})
	setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey = "MER_AbCdEfGhIjKlMnOpQrStUv", privatePEM
	setting.WaffoPancakeProductID, setting.WaffoPancakeStoreID, setting.WaffoPancakeEnvironment = "PROD_AbCdEfGhIjKlMnOpQrStUv", "STO_AbCdEfGhIjKlMnOpQrStUv", "test"
	setting.WaffoPancakeUnitPrice, setting.WaffoPancakeMinTopUp, common.QuotaPerUnit, common.RedisEnabled = 1.25, 1, 500000, false
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	adty_setting.GetTopupSetting().BonusRates = map[int]float64{2: 10}
	http.DefaultTransport = emailHTTPTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "api.waffo.ai", req.URL.Host)
		var request map[string]any
		require.NoError(t, common.DecodeJson(req.Body, &request))
		response := `{"data":{"token":"synthetic-buyer-token","expiresAt":"2099-01-01T00:00:00Z"}}`
		switch req.URL.Path {
		case "/v1/actions/auth/issue-session-token":
			require.Equal(t, service.WaffoPancakeBuyerIdentityFromUserID(user.Id), request["buyerIdentity"])
		case "/v1/actions/checkout/create-session":
			require.Equal(t, setting.WaffoPancakeProductID, request["productId"])
			require.Equal(t, "2.50", request["priceSnapshot"].(map[string]any)["amount"])
			response = `{"data":{"sessionId":"pancake-session","checkoutUrl":"https://checkout.example/pancake","expiresAt":"2099-01-01T00:00:00Z"}}`
		default:
			t.Errorf("unexpected external endpoint %s", req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(response))}, nil
	})
	created := callStripe(t, PostAdtyHostedPayment, user.Id, `{"amount":2,"method":"waffo_pancake"}`)
	require.Contains(t, created.Body.String(), "https://checkout.example/pancake")
	var order model.TopUp
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&order).Error)
	ext, err := model.GetAdtyTopupOrder(order.TradeNo)
	require.NoError(t, err)
	require.EqualValues(t, 2, order.Amount)
	require.Equal(t, "2", ext.FaceUSD.String())
	require.Equal(t, "2.5", ext.PayAmount.String())
	require.EqualValues(t, 1100000, ext.CreditQuota)
	require.Equal(t, "test", ext.ProviderEnvironment)
	// Only inject the external provider's public key; verification still uses
	// the real service and SDK, including timestamp and mode-key selection.
	verifyWaffoPancakeWebhook = func(payload, signature string) (*service.WaffoPancakeWebhookEvent, error) {
		return service.VerifyConfiguredWaffoPancakeWebhook(payload, signature, pancake.WebhookPublicKeys{Test: publicPEM, Prod: publicPEM})
	}
	setting.WaffoPancakeStoreID, setting.WaffoPancakeProductID, setting.WaffoPancakeEnvironment = "STO_ZbCdEfGhIjKlMnOpQrStUv", "PROD_ZbCdEfGhIjKlMnOpQrStUv", "prod"
	data := map[string]any{"orderId": "ORD_AbCdEfGhIjKlMnOpQrStUv", "orderMerchantExternalId": order.TradeNo,
		"merchantProvidedBuyerIdentity": service.WaffoPancakeBuyerIdentityFromUserID(user.Id), "currency": "USD", "amount": "2.50", "orderMetadata": map[string]string{"adty_product": ext.ProviderProductId}}
	envelope := map[string]any{"id": "evt_pancake_frozen", "eventId": "PAY_AbCdEfGhIjKlMnOpQrStUv", "eventType": "order.completed", "mode": "test", "storeId": ext.ProviderMerchantId, "data": data}
	post := func(path string, signingKey *rsa.PrivateKey, timestamp int64) *httptest.ResponseRecorder {
		payload, marshalErr := common.Marshal(envelope)
		require.NoError(t, marshalErr)
		digest := sha256.Sum256(fmt.Appendf(nil, "%d.%s", timestamp, payload))
		signature, signErr := rsa.SignPKCS1v15(rand.Reader, signingKey, crypto.SHA256, digest[:])
		require.NoError(t, signErr)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
		req.Header.Set("X-Waffo-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, base64.StdEncoding.EncodeToString(signature)))
		recorder := httptest.NewRecorder()
		engine := gin.New()
		engine.POST("/api/waffo-pancake/webhook/:env", WaffoPancakeWebhook)
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	path := "/api/waffo-pancake/webhook/test"
	require.Equal(t, http.StatusUnauthorized, post(path, wrongKey, time.Now().UnixMilli()).Code)
	require.Equal(t, http.StatusUnauthorized, post(path, key, time.Now().Add(-time.Hour).UnixMilli()).Code)
	post("/api/waffo-pancake/webhook/prod", key, time.Now().UnixMilli())
	require.Zero(t, reloadUserQuota(t, db, user.Id))
	for _, field := range []string{"amount", "currency", "orderMetadata", "merchantProvidedBuyerIdentity", "storeId", "mode"} {
		target := data
		if field == "storeId" || field == "mode" {
			target = envelope
		}
		original := target[field]
		switch field {
		case "amount":
			target[field] = "2.51"
		case "currency":
			target[field] = "EUR"
		case "orderMetadata":
			target[field] = map[string]string{"adty_product": setting.WaffoPancakeProductID}
		case "merchantProvidedBuyerIdentity":
			target[field] = "new-api-user-999999"
		case "storeId":
			target[field] = setting.WaffoPancakeStoreID
		case "mode":
			target[field] = "prod"
		}
		post(path, key, time.Now().UnixMilli())
		require.Zero(t, reloadUserQuota(t, db, user.Id), field)
		target[field] = original
	}
	data["taxAmount"], data["subtotal"], data["total"] = "0.40", "2.50", "2.91"
	require.Equal(t, http.StatusBadRequest, post(path, key, time.Now().UnixMilli()).Code)
	require.Zero(t, reloadUserQuota(t, db, user.Id))
	delete(data, "total")
	require.Equal(t, http.StatusBadRequest, post(path, key, time.Now().UnixMilli()).Code)
	require.Zero(t, reloadUserQuota(t, db, user.Id))
	delete(data, "subtotal")
	require.Equal(t, http.StatusBadRequest, post(path, key, time.Now().UnixMilli()).Code)
	require.Zero(t, reloadUserQuota(t, db, user.Id))
	data["subtotal"], data["total"] = "2.50", "2.90"
	require.Equal(t, http.StatusOK, post(path, key, time.Now().UnixMilli()).Code)
	require.Equal(t, int(ext.CreditQuota), reloadUserQuota(t, db, user.Id))
	require.Equal(t, http.StatusOK, post(path, key, time.Now().UnixMilli()).Code)
	require.Equal(t, int(ext.CreditQuota), reloadUserQuota(t, db, user.Id))
	frozen, err := model.GetAdtyTopupOrder(order.TradeNo)
	require.NoError(t, err)
	require.Equal(t, "2.9", frozen.PaidAmount.String())
	require.Equal(t, "2", frozen.FaceUSD.String())
	// One signed refund request is one receipt. Provider retries retain their
	// own receipt without creating another reconciliation item or wallet change.
	envelope["eventType"] = "refund.succeeded"
	data["refundTicketMerchantExternalId"] = "pancake-refund-receipt"
	for attempt := int64(1); attempt <= 2; attempt++ {
		require.Equal(t, http.StatusOK, post(path, key, time.Now().UnixMilli()).Code)
		var receipts, reviews int64
		require.NoError(t, db.Model(&model.AdtyOrderPaymentEvent{}).Where("trade_no = ? AND outcome = ? AND event_type = ?", order.TradeNo, "received", "refund.succeeded").Count(&receipts).Error)
		require.Equal(t, attempt, receipts)
		require.NoError(t, db.Model(&model.PaymentOrphanEvent{}).Where("reference_id = ? AND event_type = ?", order.TradeNo, "refund.succeeded").Count(&reviews).Error)
		require.EqualValues(t, 1, reviews)
		require.Equal(t, int(ext.CreditQuota), reloadUserQuota(t, db, user.Id))
	}
}

func TestWaffoSignedPaymentUsesFrozenEnvironmentMerchantAndAmount(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	sceneTestSetting(t, &setting.WaffoEnabled, true)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	oldMode, oldAPI, oldPrivate, oldPublic, oldMerchant, oldRedis := setting.WaffoSandbox, setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert, setting.WaffoMerchantId, common.RedisEnabled
	t.Cleanup(func() {
		setting.WaffoSandbox, setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert, setting.WaffoMerchantId, common.RedisEnabled = oldMode, oldAPI, oldPrivate, oldPublic, oldMerchant, oldRedis
	})
	privateKey, publicKey := waffoTestKeyPair(t)
	setting.WaffoSandbox, setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert, setting.WaffoMerchantId, common.RedisEnabled = true, "synthetic-waffo-api", privateKey, publicKey, "merchant-frozen", false
	user := model.User{Username: "waffo-frozen-facts", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/waffo/pay", nil)
	ctx.Set("id", user.Id)
	order := model.TopUp{UserId: user.Id, TradeNo: "waffo-frozen-facts", Amount: 1, Money: 2.8405, PaymentProvider: model.PaymentProviderWaffo, PaymentMethod: model.PaymentMethodWaffo, Status: common.TopUpStatusPending}
	require.NoError(t, createAdtyHostedOrder(ctx, &order, decimal.NewFromInt(500000), "USD", "", "test", ""))
	ext, err := model.GetAdtyTopupOrder(order.TradeNo)
	require.NoError(t, err)
	require.Equal(t, "merchant-frozen", ext.ProviderMerchantId)
	// The operator can switch new checkouts to production and another merchant
	// while retaining the verification credentials for this older test order.
	setting.WaffoSandbox, setting.WaffoMerchantId = false, "merchant-new"
	result := map[string]any{"merchantOrderId": order.TradeNo, "acquiringOrderId": "waffo-payment-frozen", "orderStatus": "PAY_SUCCESS", "orderAmount": "2.84", "orderCurrency": "USD", "merchantInfo": map[string]string{"merchantId": "merchant-frozen"}}
	post := func(signingKey string) *httptest.ResponseRecorder {
		body, marshalErr := common.Marshal(map[string]any{"eventType": "PAYMENT_NOTIFICATION", "result": result})
		require.NoError(t, marshalErr)
		signature, signErr := waffoutils.Sign(string(body), signingKey)
		require.NoError(t, signErr)
		req := httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", bytes.NewReader(body))
		req.Header.Set("X-SIGNATURE", signature)
		recorder := httptest.NewRecorder()
		engine := gin.New()
		engine.POST("/api/waffo/webhook", WaffoWebhook)
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	wrongKey, _ := waffoTestKeyPair(t)
	require.Equal(t, http.StatusBadRequest, post(wrongKey).Code)
	for _, field := range []string{"orderAmount", "orderCurrency", "merchantInfo"} {
		old := result[field]
		switch field {
		case "orderAmount":
			result[field] = "2.85"
		case "orderCurrency":
			result[field] = "EUR"
		case "merchantInfo":
			result[field] = map[string]string{"merchantId": "merchant-new"}
		}
		post(privateKey)
		require.Zero(t, reloadUserQuota(t, db, user.Id), field)
		result[field] = old
	}
	require.Equal(t, http.StatusOK, post(privateKey).Code)
	require.Equal(t, int(ext.CreditQuota), reloadUserQuota(t, db, user.Id))
	require.Equal(t, http.StatusOK, post(privateKey).Code)
	require.Equal(t, int(ext.CreditQuota), reloadUserQuota(t, db, user.Id))
}

func TestHostedOrderFreezesTheAmountSentInTheCurrencyMinorUnit(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	sceneTestSetting(t, &setting.WaffoEnabled, true)
	sceneTestSetting(t, &setting.WaffoSandbox, true)
	sceneTestSetting(t, &setting.WaffoSandboxApiKey, "synthetic-minor-unit-key")
	sceneTestSetting(t, &setting.WaffoSandboxPrivateKey, "synthetic-minor-unit-private")
	sceneTestSetting(t, &setting.WaffoSandboxPublicCert, "synthetic-minor-unit-public")
	sceneTestSetting(t, &setting.WaffoPancakeMerchantID, "synthetic-minor-unit-merchant")
	sceneTestSetting(t, &setting.WaffoPancakePrivateKey, "synthetic-minor-unit-private")
	sceneTestSetting(t, &setting.WaffoPancakeProductID, "synthetic-minor-unit-product")
	sceneTestSetting(t, &setting.WaffoPancakeEnvironment, "test")
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	for _, tc := range []struct{ provider, currency, want string }{
		{model.PaymentProviderWaffo, "USD", "2.84"},
		{model.PaymentProviderWaffo, "JPY", "3"},
		{model.PaymentProviderWaffoPancake, "USD", "2.84"},
	} {
		t.Run(tc.provider+tc.currency, func(t *testing.T) {
			sceneTestSetting(t, &setting.WaffoCurrency, tc.currency)
			user := model.User{Username: tc.provider + tc.currency, Group: "default", Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&user).Error)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/adty/topup/hosted/pay", nil)
			ctx.Set("id", user.Id)
			order := model.TopUp{UserId: user.Id, TradeNo: "minor-unit-" + tc.provider + tc.currency, Amount: 1,
				Money: 2.8405, PaymentProvider: tc.provider, PaymentMethod: tc.provider, Status: common.TopUpStatusPending}
			require.NoError(t, createAdtyHostedOrder(ctx, &order, decimal.NewFromInt(500000), tc.currency, "", "test", ""))
			ext, err := model.GetAdtyTopupOrder(order.TradeNo)
			require.NoError(t, err)
			require.Equal(t, tc.want, ext.PayAmount.String())
			paid, err := decimal.NewFromString(tc.want)
			require.NoError(t, err)
			require.True(t, paid.Equal(decimal.NewFromFloat(order.Money)))
			_, err = model.CompleteAdtyTopUp(order.TradeNo, model.AdtyPaymentFacts{Provider: tc.provider, Method: tc.provider,
				PaidAmount: paid, PaidCurrency: tc.currency, Environment: "test", MerchantId: ext.ProviderMerchantId,
				GatewayTradeId: "minor-unit-payment", TxHash: "minor-unit-payment", Source: model.AdtyPaymentSourceCallback}, "")
			require.NoError(t, err)
		})
	}
}

type creemCheckoutTransport struct{ t *testing.T }

func (rt creemCheckoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	require.Equal(rt.t, "https://test-api.creem.io/v1/checkouts", req.URL.String())
	require.Equal(rt.t, "creem-test-key", req.Header.Get("x-api-key"))
	var body CreemCheckoutRequest
	require.NoError(rt.t, common.DecodeJson(req.Body, &body))
	require.Equal(rt.t, "prod_fractional", body.ProductId)
	require.NotEmpty(rt.t, body.RequestId)
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(`{"id":"ch_test","checkout_url":"https://checkout.example/creem"}`))}, nil
}

func TestCreemHostedCheckoutFreezesFaceAndAcceptsVerifiedTaxAndDiscount(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	oldRedis, oldRDB := common.RedisEnabled, common.RDB
	redisAddr := os.Getenv("ADTY_TEST_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = miniredis.RunT(t).Addr()
	}
	common.RedisEnabled, common.RDB = true, redis.NewClient(&redis.Options{Addr: redisAddr, DB: 10, MaxRetries: -1})
	t.Cleanup(func() { _ = common.RDB.Close(); common.RedisEnabled, common.RDB = oldRedis, oldRDB })
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	user := model.User{Username: "hosted-creem", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	oldKey, oldSecret, oldProducts, oldMode := setting.CreemApiKey, setting.CreemWebhookSecret, setting.CreemProducts, setting.CreemTestMode
	oldRates, oldTransport, oldUnit := adty_setting.GetTopupSetting().BonusRates, http.DefaultTransport, common.QuotaPerUnit
	oldDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	setting.CreemApiKey, setting.CreemWebhookSecret, setting.CreemTestMode = "creem-test-key", "creem-webhook-test-secret", true
	setting.CreemProducts = `[{"productId":"prod_fractional","name":"Fractional pack","price":10,"currency":"EUR","quota":750000}]`
	adty_setting.GetTopupSetting().BonusRates = map[int]float64{1: 10}
	http.DefaultTransport, common.QuotaPerUnit = creemCheckoutTransport{t}, 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		setting.CreemApiKey, setting.CreemWebhookSecret, setting.CreemProducts, setting.CreemTestMode = oldKey, oldSecret, oldProducts, oldMode
		adty_setting.GetTopupSetting().BonusRates, http.DefaultTransport, common.QuotaPerUnit = oldRates, oldTransport, oldUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplay
	})
	created := callStripe(t, PostAdtyHostedPayment, user.Id, `{"amount":1,"method":"creem:prod_fractional"}`)
	require.Contains(t, created.Body.String(), "https://checkout.example/creem")
	var topUp model.TopUp
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&topUp).Error)
	ext, err := model.GetAdtyTopupOrder(topUp.TradeNo)
	require.NoError(t, err)
	require.EqualValues(t, 825000, ext.CreditQuota)
	require.Equal(t, "1.5", ext.FaceUSD.String())
	// New-order configuration changes cannot invalidate this signed old order.
	setting.CreemProducts = "[]"
	operation_setting.GetPaymentSetting().ComplianceConfirmed = false
	obj := map[string]any{
		"id": "ch_test", "request_id": topUp.TradeNo, "mode": "test", "status": "completed", "units": 1,
		"order": map[string]any{"id": "ord_tax", "product": "prod_fractional", "status": "paid", "type": "onetime", "amount": 1000, "sub_total": 1000, "discount_amount": 200, "tax_amount": 160, "amount_due": 960, "amount_paid": 960, "currency": "EUR", "mode": "test"},
	}
	post := func(signatureOK bool) *httptest.ResponseRecorder {
		payload, err := common.Marshal(map[string]any{"id": "evt_tax", "eventType": "checkout.completed", "object": obj})
		require.NoError(t, err)
		signature := "invalid"
		if signatureOK {
			signature = generateCreemSignature(string(payload), setting.CreemWebhookSecret)
		}
		return postHostedCreemWebhook(payload, signature)
	}
	require.Equal(t, http.StatusUnauthorized, post(false).Code)
	order := obj["order"].(map[string]any)
	for _, field := range []string{"amount_paid", "currency", "product", "mode"} {
		original := order[field]
		switch field {
		case "amount_paid":
			order[field] = 961
		case "currency":
			order[field] = "USD"
		case "product":
			order[field] = "prod_other"
		case "mode":
			order[field] = "prod"
		}
		require.Equal(t, http.StatusOK, post(true).Code)
		require.Zero(t, reloadUserQuota(t, db, user.Id))
		order[field] = original
	}
	require.Equal(t, http.StatusOK, post(true).Code)
	require.Equal(t, 825000, reloadUserQuota(t, db, user.Id))
	require.Equal(t, http.StatusOK, post(true).Code)
	require.Equal(t, 825000, reloadUserQuota(t, db, user.Id))
	ext, err = model.GetAdtyTopupOrder(topUp.TradeNo)
	require.NoError(t, err)
	require.Equal(t, "9.6", ext.PaidAmount.String())
	require.Equal(t, "10", ext.PayAmount.String())
	var count int64
	require.NoError(t, db.Model(&model.AdtyBalanceLedger{}).Where("ref_id = ? AND type = ?", topUp.TradeNo, model.AdtyLedgerTypeTopUp).Count(&count).Error)
	require.EqualValues(t, 1, count)
	for _, eventID := range []string{"evt_refund", "evt_manual_resend"} {
		payload, err := common.Marshal(map[string]any{"id": eventID, "eventType": "refund.created", "object": map[string]any{
			"id": "ref_external", "status": "succeeded", "refund_amount": 960, "refund_currency": "EUR", "mode": "test",
			"checkout": map[string]any{"request_id": topUp.TradeNo}, "order": map[string]any{"id": "ord_tax"},
		}})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, postHostedCreemWebhook(payload, generateCreemSignature(string(payload), setting.CreemWebhookSecret)).Code)
	}
	var reversals []model.PaymentOrphanEvent
	require.NoError(t, db.Where("reference_id = ? AND event_type = ?", topUp.TradeNo, "refund.created").Find(&reversals).Error)
	require.Len(t, reversals, 1, "provider manual resend must not create a second reconciliation item")
	require.Equal(t, model.PaymentOrphanStatusPendingReview, reversals[0].Status)
	require.False(t, model.CanCreditPaymentOrphanEvent(&reversals[0]))
}

func postHostedCreemWebhook(payload []byte, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(CreemSignatureHeader, signature)
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/api/creem/webhook", CreemWebhook)
	engine.ServeHTTP(recorder, req)
	return recorder
}
