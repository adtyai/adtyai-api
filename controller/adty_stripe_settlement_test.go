package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81/webhook"
)

func TestStripeSignedSubtotalDiscountAndTaxKeepTheFrozenFace(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	oldSecret, oldRedis := setting.StripeWebhookSecret, common.RedisEnabled
	setting.StripeWebhookSecret, common.RedisEnabled = "whsec_hosted_totals", false
	t.Cleanup(func() { setting.StripeWebhookSecret, common.RedisEnabled = oldSecret, oldRedis })
	user := model.User{Username: "stripe-signed-totals", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	order := model.TopUp{UserId: user.Id, TradeNo: "stripe-totals", Amount: 1, Money: 10, PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending}
	ext := model.AdtyTopupOrder{TradeNo: order.TradeNo, UserId: user.Id, Provider: order.PaymentProvider, Method: order.PaymentMethod,
		FaceUSD: decimal.NewFromInt(1), CreditQuota: 500000, PayAmount: decimal.NewFromInt(10), PayCurrency: "USD", GatewayTradeId: "cs_totals", ProviderProductId: "wallet_topup", ProviderEnvironment: "test"}
	require.NoError(t, model.CreateAdtyTopUp(&order, &ext))
	object := map[string]any{
		"id": "cs_totals", "object": "checkout.session", "client_reference_id": order.TradeNo, "status": "complete", "payment_status": "paid",
		"amount_subtotal": 1000, "amount_total": 900, "currency": "usd", "payment_intent": "pi_totals", "metadata": map[string]string{"adty_product": "wallet_topup"},
		"total_details": map[string]int{"amount_discount": 200, "amount_tax": 100, "amount_shipping": 0},
	}
	for _, scenario := range []string{"wrong total", "wrong subtotal", "zero payment", "wrong currency", "wrong environment"} {
		originalTotal, originalSubtotal, originalCurrency, originalDetails := object["amount_total"], object["amount_subtotal"], object["currency"], object["total_details"]
		liveMode := false
		switch scenario {
		case "wrong total":
			object["amount_total"] = 901
		case "wrong subtotal":
			object["amount_subtotal"] = 999
		case "zero payment":
			object["amount_total"], object["total_details"] = 0, map[string]int{"amount_discount": 1000}
		case "wrong currency":
			object["currency"] = "eur"
		case "wrong environment":
			liveMode = true
		}
		require.Equal(t, http.StatusInternalServerError, postHostedStripeEvent(t, object, liveMode).Code, scenario)
		require.Zero(t, reloadUserQuota(t, db, user.Id), scenario)
		object["amount_total"], object["amount_subtotal"], object["currency"], object["total_details"] = originalTotal, originalSubtotal, originalCurrency, originalDetails
	}
	require.Equal(t, http.StatusOK, postHostedStripeEvent(t, object, false).Code)
	require.Equal(t, 500000, reloadUserQuota(t, db, user.Id))
	require.Equal(t, http.StatusOK, postHostedStripeEvent(t, object, false).Code)
	require.Equal(t, 500000, reloadUserQuota(t, db, user.Id))
	frozen, err := model.GetAdtyTopupOrder(order.TradeNo)
	require.NoError(t, err)
	require.Equal(t, "9", frozen.PaidAmount.String())
	require.Equal(t, "1", frozen.FaceUSD.String())
}

func postHostedStripeEvent(t *testing.T, object map[string]any, liveMode bool) *httptest.ResponseRecorder {
	return postHostedStripeTypedEvent(t, "checkout.session.completed", object, liveMode)
}

func TestStripeLaterPartialRefundCreatesANewReviewAfterResolution(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	require.NoError(t, model.MigrateAdtyTables())
	oldSecret := setting.StripeWebhookSecret
	setting.StripeWebhookSecret = "whsec_partial_refunds"
	t.Cleanup(func() { setting.StripeWebhookSecret = oldSecret })
	charge := map[string]any{"id": "ch_partial", "object": "charge", "payment_intent": "pi_partial", "amount_refunded": 200,
		"metadata": map[string]string{"adty_trade_no": "partial-refund-order"}}
	require.Equal(t, http.StatusOK, postHostedStripeTypedEvent(t, "charge.refunded", charge, false).Code)
	var first model.PaymentOrphanEvent
	require.NoError(t, db.Where("provider = ? AND reference_id = ?", model.PaymentProviderStripe, "partial-refund-order").First(&first).Error)
	_, err := model.ResolvePaymentOrphanReview(first.ID, 1, model.PaymentOrphanReviewRequest{
		Status: model.PaymentOrphanStatusPendingReview, TradeNo: first.ReferenceID,
		ExternalAmount: "2", ExternalCurrency: "USD", Proof: "refund-dashboard-1", Note: "first partial refund requires financial reconciliation",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, postHostedStripeTypedEvent(t, "charge.refunded", charge, false).Code)
	charge["amount_refunded"] = 500
	require.Equal(t, http.StatusOK, postHostedStripeTypedEvent(t, "charge.refunded", charge, false).Code)
	require.Equal(t, http.StatusOK, postHostedStripeTypedEvent(t, "charge.refunded", charge, false).Code)
	var events []model.PaymentOrphanEvent
	require.NoError(t, db.Where("provider = ? AND reference_id = ?", model.PaymentProviderStripe, "partial-refund-order").Order("id").Find(&events).Error)
	require.Len(t, events, 2, "a later external refund must remain visible after an earlier review was resolved")
	require.Equal(t, model.PaymentOrphanStatusPendingReview, events[0].Status)
	require.Positive(t, events[0].ReviewID)
	require.Equal(t, model.PaymentOrphanStatusPendingReview, events[1].Status)
}

func postHostedStripeTypedEvent(t *testing.T, eventType string, object map[string]any, liveMode bool) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := common.Marshal(map[string]any{"id": "evt_totals", "object": "event", "type": eventType, "livemode": liveMode, "data": map[string]any{"object": object}})
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: setting.StripeWebhookSecret})
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signed.Header)
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/api/stripe/webhook", StripeWebhook)
	engine.ServeHTTP(recorder, req)
	return recorder
}
