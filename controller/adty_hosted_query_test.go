package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	waffonet "github.com/waffo-com/waffo-go/net"
	waffoutils "github.com/waffo-com/waffo-go/utils"
)

type hostedOrderQuerier interface {
	Query(context.Context, *model.AdtyTopupOrder) (*model.AdtyPaymentFacts, error)
}

func TestHostedReversalBeforePaymentBlocksLateAutomaticCredit(t *testing.T) {
	for _, provider := range []string{model.PaymentProviderStripe, model.PaymentProviderCreem, model.PaymentProviderWaffo, model.PaymentProviderWaffoPancake} {
		t.Run(provider, func(t *testing.T) {
			ext, userID := newHostedQueryOrder(t, provider, "", "", "payment-reversed")
			require.NoError(t, recordHostedReversal(provider, "refund.succeeded", "refund-before-payment", ext.TradeNo, ext.GatewayTradeId, "synthetic signed reversal"))
			facts := model.AdtyPaymentFacts{Provider: provider, Method: ext.Method, PaidAmount: ext.PayAmount, PaidCurrency: ext.PayCurrency,
				Environment: ext.ProviderEnvironment, GatewayTradeId: ext.GatewayTradeId, TxHash: ext.GatewayTradeId}
			for _, source := range []string{model.AdtyPaymentSourceCallback, model.AdtyPaymentSourceQuery} {
				facts.Source = source
				_, err := model.CompleteAdtyTopUp(ext.TradeNo, facts, "")
				require.ErrorIs(t, err, model.ErrPaymentOrphanNotCredit)
				require.Zero(t, reloadUserQuota(t, model.DB, userID))
			}
		})
	}
}

func newHostedQueryOrder(t *testing.T, provider, product, merchant, gateway string) (*model.AdtyTopupOrder, int) {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	user := model.User{Username: provider + "-query", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	order := model.TopUp{UserId: user.Id, TradeNo: provider + "-query", Amount: 2, Money: 2.5, PaymentProvider: provider, PaymentMethod: provider, Status: common.TopUpStatusExpired}
	ext := &model.AdtyTopupOrder{TradeNo: order.TradeNo, UserId: user.Id, Provider: provider, Method: provider, FaceUSD: decimal.NewFromInt(2), CreditQuota: 1000000,
		PayAmount: decimal.RequireFromString("2.5"), PayCurrency: "USD", ProviderProductId: product, ProviderMerchantId: merchant, ProviderEnvironment: "test", GatewayTradeId: gateway}
	require.NoError(t, model.CreateAdtyTopUp(&order, ext))
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	return ext, user.Id
}

func TestCreemHostedQueryValidatesFrozenCheckoutAndRecoversReview(t *testing.T) {
	ext, userID := newHostedQueryOrder(t, model.PaymentProviderCreem, "prod_query", "", "ch_query")
	oldKey, oldMode, oldTransport := setting.CreemApiKey, setting.CreemTestMode, http.DefaultTransport
	setting.CreemApiKey, setting.CreemTestMode = "creem-synthetic-query", true
	t.Cleanup(func() {
		setting.CreemApiKey, setting.CreemTestMode, http.DefaultTransport = oldKey, oldMode, oldTransport
	})
	purchase := map[string]any{"id": "ord_query", "status": "paid", "type": "onetime", "mode": "test", "product": "prod_query", "currency": "USD", "amount": 250, "sub_total": 250, "discount_amount": 50, "tax_amount": 40, "amount_paid": 240}
	checkout := map[string]any{"id": "ch_query", "request_id": ext.TradeNo, "status": "completed", "mode": "test", "units": 1, "order": purchase}
	queries := 0
	http.DefaultTransport = emailHTTPTransport(func(req *http.Request) (*http.Response, error) {
		queries++
		require.Equal(t, "https://test-api.creem.io/v1/checkouts?checkout_id=ch_query", req.URL.String())
		require.Equal(t, "creem-synthetic-query", req.Header.Get("x-api-key"))
		payload, err := common.Marshal(checkout)
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload))}, nil
	})
	for _, field := range []string{"product", "amount_paid", "currency", "mode"} {
		original := purchase[field]
		if field == "amount_paid" {
			purchase[field] = 241
		} else {
			purchase[field] = "wrong"
		}
		require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), ext), field)
		require.Zero(t, reloadUserQuota(t, model.DB, userID), field)
		purchase[field] = original
	}
	checkout["request_id"] = "another-order"
	require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), ext))
	now := time.Now().Unix()
	require.NoError(t, model.DB.Model(&model.AdtyTopupOrder{}).Where("id = ?", ext.Id).Update("created_at", now-180).Error)
	service.RunAdtyTopUpMaintenanceOnce(now)
	var review model.PaymentOrphanEvent
	require.NoError(t, model.DB.Where("event_id = ?", "hosted-query:"+ext.TradeNo).First(&review).Error)
	require.Equal(t, model.PaymentOrphanStatusPendingReview, review.Status)
	require.Zero(t, reloadUserQuota(t, model.DB, userID))
	afterFailure := queries
	checkout["request_id"] = ext.TradeNo
	service.RunAdtyTopUpMaintenanceOnce(now + 30)
	require.Equal(t, afterFailure, queries, "recovery must not poll the same order within one minute")
	service.RunAdtyTopUpMaintenanceOnce(now + 61)
	require.Equal(t, afterFailure+1, queries)
	for range 2 {
		require.NoError(t, service.RecoverAdtyHostedPayment(t.Context(), ext))
	}
	require.EqualValues(t, 1000000, reloadUserQuota(t, model.DB, userID))
	require.NoError(t, model.DB.First(&review, review.ID).Error)
	require.Equal(t, model.PaymentOrphanStatusCredited, review.Status)
}

type waffoQueryHTTPTransport func(context.Context, *waffonet.HttpRequest) (*waffonet.HttpResponse, error)

func (transport waffoQueryHTTPTransport) Send(ctx context.Context, request *waffonet.HttpRequest) (*waffonet.HttpResponse, error) {
	return transport(ctx, request)
}

func TestWaffoHostedQueryVerifiesSignedProviderResponseAndBinding(t *testing.T) {
	ext, userID := newHostedQueryOrder(t, model.PaymentProviderWaffo, "", "merchant-query", "waffo-query")
	private, public := waffoTestKeyPair(t)
	oldKey, oldPrivate, oldPublic, oldTransport := setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert, waffoHTTPTransport
	setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert = "synthetic-waffo-query", private, public
	t.Cleanup(func() {
		setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert, waffoHTTPTransport = oldKey, oldPrivate, oldPublic, oldTransport
	})
	purchase := map[string]any{"merchantOrderId": ext.TradeNo, "paymentRequestId": ext.TradeNo, "acquiringOrderId": ext.GatewayTradeId, "orderStatus": "PAY_SUCCESS", "orderAmount": "2.50", "orderCurrency": "USD", "merchantInfo": map[string]string{"merchantId": "merchant-query"}}
	validSignature := true
	waffoHTTPTransport = waffoQueryHTTPTransport(func(ctx context.Context, request *waffonet.HttpRequest) (*waffonet.HttpResponse, error) {
		require.Contains(t, request.URL, "/order/inquiry")
		require.True(t, waffoutils.Verify(string(request.Body), request.Headers["X-SIGNATURE"], public))
		payload, err := common.Marshal(map[string]any{"code": "0", "data": purchase})
		require.NoError(t, err)
		signature, err := waffoutils.Sign(string(payload), private)
		require.NoError(t, err)
		if !validSignature {
			signature = "invalid"
		}
		return waffonet.NewHttpResponse(http.StatusOK, map[string]string{"X-SIGNATURE": signature}, payload), nil
	})
	validSignature = false
	require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), ext))
	validSignature = true
	for _, field := range []string{"merchantOrderId", "acquiringOrderId", "orderAmount", "orderCurrency", "merchantInfo"} {
		original := purchase[field]
		if field == "merchantInfo" {
			purchase[field] = map[string]string{"merchantId": "wrong"}
		} else {
			purchase[field] = "wrong"
		}
		require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), ext), field)
		require.Zero(t, reloadUserQuota(t, model.DB, userID), field)
		purchase[field] = original
	}
	for range 2 {
		require.NoError(t, service.RecoverAdtyHostedPayment(t.Context(), ext))
	}
	require.EqualValues(t, 1000000, reloadUserQuota(t, model.DB, userID))
}

func TestPancakeHostedQueryRecoversByMerchantReferenceAndTaxedSnapshot(t *testing.T) {
	ext, userID := newHostedQueryOrder(t, model.PaymentProviderWaffoPancake, "PROD_AbCdEfGhIjKlMnOpQrStUv", "STO_AbCdEfGhIjKlMnOpQrStUv", "")
	private, _ := waffoTestKeyPair(t)
	oldMerchant, oldPrivate, oldTransport := setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, http.DefaultTransport
	setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey = "MER_AbCdEfGhIjKlMnOpQrStUv", private
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, http.DefaultTransport = oldMerchant, oldPrivate, oldTransport
	})
	payment := map[string]any{"id": "PAY_AbCdEfGhIjKlMnOpQrStUv", "orderId": "ORD_AbCdEfGhIjKlMnOpQrStUv", "status": "succeeded", "refundStatus": "", "orderMerchantExternalId": ext.TradeNo}
	price := map[string]any{"currency": "USD", "subtotal": "2.50", "taxAmount": "0.40", "total": "2.90"}
	purchase := map[string]any{"id": payment["orderId"], "orderMerchantExternalId": ext.TradeNo, "testMode": true, "currency": "USD", "store": map[string]string{"id": ext.ProviderMerchantId}, "onetimeProduct": map[string]string{"id": ext.ProviderProductId}, "priceSnapshot": price}
	http.DefaultTransport = emailHTTPTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "https://api.waffo.ai/v1/graphql", req.URL.String())
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		require.NoError(t, common.DecodeJson(req.Body, &body))
		data := map[string]any{"payments": []any{payment}}
		if body.Variables["id"] != nil {
			require.Equal(t, payment["orderId"], body.Variables["id"])
			data = map[string]any{"onetimeOrder": purchase}
		} else {
			require.Equal(t, ext.TradeNo, body.Variables["ref"])
		}
		payload, err := common.Marshal(map[string]any{"data": data})
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload))}, nil
	})
	for _, field := range []string{"refundStatus", "orderMerchantExternalId"} {
		original := payment[field]
		payment[field] = "wrong"
		require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), ext), field)
		payment[field] = original
	}
	for _, field := range []string{"store", "onetimeProduct", "testMode", "currency", "orderMerchantExternalId"} {
		original := purchase[field]
		if field == "store" || field == "onetimeProduct" {
			purchase[field] = map[string]string{"id": "wrong"}
		} else if field == "testMode" {
			purchase[field] = false
		} else {
			purchase[field] = "wrong"
		}
		require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), ext), field)
		require.Zero(t, reloadUserQuota(t, model.DB, userID), field)
		purchase[field] = original
	}
	price["total"] = "2.91"
	require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), ext))
	price["total"] = "2.90"
	for range 2 {
		require.NoError(t, service.RecoverAdtyHostedPayment(t.Context(), ext))
	}
	require.EqualValues(t, 1000000, reloadUserQuota(t, model.DB, userID))
	frozen, err := model.GetAdtyTopupOrder(ext.TradeNo)
	require.NoError(t, err)
	require.Equal(t, "2.9", frozen.PaidAmount.String())
	require.Equal(t, "2", frozen.FaceUSD.String())
}

func TestStripeHostedQueryRecoversLostCallbackWithFrozenFacts(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	user := model.User{Username: "stripe-query", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	order := model.TopUp{UserId: user.Id, TradeNo: "stripe-query", Amount: 2, Money: 2.5, PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusExpired}
	ext := model.AdtyTopupOrder{TradeNo: order.TradeNo, UserId: user.Id, Provider: order.PaymentProvider, Method: order.PaymentMethod, FaceUSD: decimal.NewFromInt(2), CreditQuota: 1000000,
		PayAmount: decimal.RequireFromString("2.5"), PayCurrency: "USD", ProviderProductId: "wallet_topup", ProviderEnvironment: "test", GatewayTradeId: "cs_query"}
	require.NoError(t, model.CreateAdtyTopUp(&order, &ext))
	oldKey, oldTransport, oldRedis := setting.StripeApiSecret, http.DefaultTransport, common.RedisEnabled
	setting.StripeApiSecret, common.RedisEnabled = "sk_test_synthetic_query", false
	t.Cleanup(func() {
		setting.StripeApiSecret, http.DefaultTransport, common.RedisEnabled = oldKey, oldTransport, oldRedis
	})
	response := map[string]any{"id": "cs_query", "client_reference_id": order.TradeNo, "status": "complete", "payment_status": "paid", "livemode": false,
		"amount_subtotal": 250, "amount_total": 240, "total_details": map[string]int{"amount_discount": 50, "amount_tax": 40}, "currency": "usd", "payment_intent": "pi_query", "metadata": map[string]string{"adty_product": "wallet_topup"}}
	charge := map[string]any{"id": "ch_query", "payment_intent": "pi_query", "status": "succeeded", "amount": 240, "amount_refunded": 0, "currency": "usd", "paid": true, "captured": true, "refunded": false, "disputed": false}
	intent := map[string]any{"id": "pi_query", "status": "succeeded", "livemode": false, "amount_received": 240, "currency": "usd", "metadata": map[string]string{"adty_trade_no": order.TradeNo}, "latest_charge": charge}
	http.DefaultTransport = emailHTTPTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "api.stripe.com", req.URL.Host)
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "Bearer sk_test_synthetic_query", req.Header.Get("Authorization"))
		body := response
		if req.URL.Path == "/v1/payment_intents/pi_query" {
			require.Equal(t, "latest_charge", req.URL.Query().Get("expand[]"))
			body = intent
		} else {
			require.Equal(t, "/v1/checkout/sessions/cs_query", req.URL.Path)
		}
		payload, err := common.Marshal(body)
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload))}, nil
	})
	query, ok := any(adtyHostedProvider(model.PaymentProviderStripe)).(hostedOrderQuerier)
	require.True(t, ok, "hosted provider must recover a lost payment callback through its authenticated query API")
	for _, field := range []string{"client_reference_id", "id", "amount_total", "currency", "livemode", "metadata"} {
		original := response[field]
		switch field {
		case "client_reference_id", "id", "currency":
			response[field] = "wrong"
		case "amount_total":
			response[field] = 241
		case "livemode":
			response[field] = true
		case "metadata":
			response[field] = map[string]string{"adty_product": "wrong"}
		}
		facts, err := query.Query(t.Context(), &ext)
		if err == nil && facts != nil {
			_, err = model.CompleteAdtyTopUp(order.TradeNo, *facts, "")
		}
		require.Error(t, err, field)
		require.Zero(t, reloadUserQuota(t, db, user.Id), field)
		response[field] = original
	}
	response["payment_status"] = "unpaid"
	facts, err := query.Query(t.Context(), &ext)
	require.NoError(t, err)
	require.Nil(t, facts)
	response["payment_status"] = "paid"
	for _, field := range []string{"amount_refunded", "refunded", "disputed"} {
		original := charge[field]
		if field == "amount_refunded" {
			charge[field] = 10
		} else {
			charge[field] = true
		}
		require.Error(t, service.RecoverAdtyHostedPayment(t.Context(), &ext), field)
		require.Zero(t, reloadUserQuota(t, db, user.Id), field)
		charge[field] = original
	}
	for range 2 {
		facts, err = query.Query(t.Context(), &ext)
		require.NoError(t, err)
		require.NotNil(t, facts)
		require.Equal(t, model.AdtyPaymentSourceQuery, facts.Source)
		_, err = model.CompleteAdtyTopUp(order.TradeNo, *facts, "")
		require.NoError(t, err)
	}
	require.EqualValues(t, 1000000, reloadUserQuota(t, db, user.Id))
}
