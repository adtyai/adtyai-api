package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"github.com/waffo-com/waffo-go/types/order"
)

// Query uses authenticated provider APIs and the original order binding.
// A closed new-checkout configuration does not invalidate an old payment.
func (p adtyHostedProvider) Query(ctx context.Context, ext *model.AdtyTopupOrder) (*model.AdtyPaymentFacts, error) {
	if ext == nil || ext.Provider != p.Name() {
		return nil, model.ErrPaymentMethodMismatch
	}
	facts := &model.AdtyPaymentFacts{Provider: ext.Provider, Method: ext.Method, Source: model.AdtyPaymentSourceQuery}
	switch p.Name() {
	case model.PaymentProviderStripe:
		if ext.GatewayTradeId == "" || strings.TrimSpace(setting.StripeApiSecret) == "" || stripePaymentEnvironment(setting.StripeApiSecret) != ext.ProviderEnvironment {
			return nil, errors.New("original Stripe checkout credentials are unavailable")
		}
		var result struct {
			ID                string            `json:"id"`
			ClientReferenceID string            `json:"client_reference_id"`
			PaymentStatus     string            `json:"payment_status"`
			Status            string            `json:"status"`
			Livemode          *bool             `json:"livemode"`
			AmountTotal       int64             `json:"amount_total"`
			AmountSubtotal    int64             `json:"amount_subtotal"`
			Currency          string            `json:"currency"`
			PaymentIntent     string            `json:"payment_intent"`
			Metadata          map[string]string `json:"metadata"`
			TotalDetails      struct {
				Discount int64 `json:"amount_discount"`
				Tax      int64 `json:"amount_tax"`
				Shipping int64 `json:"amount_shipping"`
			} `json:"total_details"`
		}
		payload, err := queryHostedJSON(ctx, "https://api.stripe.com/v1/checkout/sessions/"+url.PathEscape(ext.GatewayTradeId), "Authorization", "Bearer "+setting.StripeApiSecret, &result)
		if err != nil {
			return nil, err
		}
		if result.ID != ext.GatewayTradeId || result.ClientReferenceID != ext.TradeNo || result.Livemode == nil || hostedPaymentEnvironment(!*result.Livemode) != ext.ProviderEnvironment {
			return nil, model.ErrPaymentMethodMismatch
		}
		if result.Status != "complete" || result.PaymentStatus != "paid" {
			return nil, nil
		}
		total := decimal.NewFromInt(result.AmountSubtotal).Sub(decimal.NewFromInt(result.TotalDetails.Discount)).Add(decimal.NewFromInt(result.TotalDetails.Tax)).Add(decimal.NewFromInt(result.TotalDetails.Shipping))
		if result.AmountTotal <= 0 || result.AmountSubtotal <= 0 || result.TotalDetails.Discount < 0 || result.TotalDetails.Discount > result.AmountSubtotal || result.TotalDetails.Tax < 0 || result.TotalDetails.Shipping < 0 || !total.Equal(decimal.NewFromInt(result.AmountTotal)) || result.PaymentIntent == "" {
			return nil, model.ErrPaymentAmountMismatch
		}
		// A completed Checkout remains paid after a refund or dispute. Confirm
		// the current charge before recovering an otherwise lost callback.
		var intent struct {
			ID             string            `json:"id"`
			Status         string            `json:"status"`
			Livemode       *bool             `json:"livemode"`
			AmountReceived int64             `json:"amount_received"`
			Currency       string            `json:"currency"`
			Metadata       map[string]string `json:"metadata"`
			Charge         *struct {
				ID             string `json:"id"`
				PaymentIntent  string `json:"payment_intent"`
				Status         string `json:"status"`
				Amount         int64  `json:"amount"`
				AmountRefunded int64  `json:"amount_refunded"`
				Currency       string `json:"currency"`
				Paid           *bool  `json:"paid"`
				Captured       *bool  `json:"captured"`
				Refunded       *bool  `json:"refunded"`
				Disputed       *bool  `json:"disputed"`
			} `json:"latest_charge"`
		}
		intentPayload, err := queryHostedJSON(ctx, "https://api.stripe.com/v1/payment_intents/"+url.PathEscape(result.PaymentIntent)+"?expand%5B%5D=latest_charge", "Authorization", "Bearer "+setting.StripeApiSecret, &intent)
		if err != nil {
			return nil, err
		}
		charge := intent.Charge
		if intent.ID != result.PaymentIntent || intent.Status != "succeeded" || intent.Livemode == nil || *intent.Livemode != *result.Livemode || intent.AmountReceived != result.AmountTotal || !strings.EqualFold(intent.Currency, result.Currency) || intent.Metadata["adty_trade_no"] != ext.TradeNo || charge == nil || charge.ID == "" || charge.PaymentIntent != intent.ID || charge.Status != "succeeded" || charge.Amount != result.AmountTotal || !strings.EqualFold(charge.Currency, result.Currency) || charge.Paid == nil || !*charge.Paid || charge.Captured == nil || !*charge.Captured || charge.Refunded == nil || *charge.Refunded || charge.Disputed == nil || *charge.Disputed || charge.AmountRefunded != 0 {
			return nil, errors.New("Stripe charge requires reconciliation")
		}
		facts.PaidAmount, facts.PurchaseAmount = decimal.NewFromInt(result.AmountTotal).Div(decimal.NewFromInt(100)), decimal.NewFromInt(result.AmountSubtotal).Div(decimal.NewFromInt(100))
		facts.PaidCurrency, facts.ProductId, facts.Environment = result.Currency, result.Metadata["adty_product"], hostedPaymentEnvironment(!*result.Livemode)
		facts.GatewayTradeId, facts.TxHash, facts.Payload = result.ID, result.PaymentIntent, common.GetJsonString(map[string]string{"checkout": payload, "payment_intent": intentPayload})
	case model.PaymentProviderCreem:
		if ext.GatewayTradeId == "" || strings.TrimSpace(setting.CreemApiKey) == "" || hostedPaymentEnvironment(setting.CreemTestMode) != ext.ProviderEnvironment {
			return nil, errors.New("original Creem checkout credentials are unavailable")
		}
		baseURL := "https://api.creem.io"
		if ext.ProviderEnvironment == "test" {
			baseURL = "https://test-api.creem.io"
		}
		var event CreemWebhookEvent
		payload, err := queryHostedJSON(ctx, baseURL+"/v1/checkouts?checkout_id="+url.QueryEscape(ext.GatewayTradeId), "x-api-key", setting.CreemApiKey, &event.Object)
		if err != nil {
			return nil, err
		}
		result := event.Object
		if result.Id != ext.GatewayTradeId || result.RequestId != ext.TradeNo || result.Mode != ext.ProviderEnvironment {
			return nil, model.ErrPaymentMethodMismatch
		}
		if result.Status != "completed" || result.Order.Status != "paid" {
			return nil, nil
		}
		purchase := result.Order
		if purchase.Type != "onetime" || purchase.Id == "" || purchase.Mode != ext.ProviderEnvironment || purchase.AmountPaid <= 0 || purchase.Amount <= 0 || purchase.Amount != purchase.SubTotal || purchase.DiscountAmount < 0 || purchase.DiscountAmount > purchase.SubTotal || purchase.TaxAmount < 0 || int64(purchase.SubTotal)-int64(purchase.DiscountAmount)+int64(purchase.TaxAmount) != int64(purchase.AmountPaid) || (result.Units != 0 && result.Units != 1) {
			return nil, model.ErrPaymentAmountMismatch
		}
		facts.PaidAmount, facts.PurchaseAmount = decimal.NewFromInt(int64(purchase.AmountPaid)).Div(decimal.NewFromInt(100)), decimal.NewFromInt(int64(purchase.Amount)).Div(decimal.NewFromInt(100))
		facts.PaidCurrency, facts.ProductId, facts.Environment = purchase.Currency, purchase.Product, result.Mode
		if facts.ProductId == "" {
			facts.ProductId = result.Product.Id
		}
		facts.GatewayTradeId, facts.TxHash, facts.Payload = result.Id, purchase.Id, payload
	case model.PaymentProviderWaffo:
		sdk, err := getWaffoSDKForEnvironment(ext.ProviderEnvironment == "test")
		if err != nil {
			return nil, err
		}
		result, err := sdk.Order().Inquiry(ctx, &order.InquiryOrderParams{PaymentRequestID: ext.TradeNo, AcquiringOrderID: ext.GatewayTradeId}, nil)
		if err != nil || result == nil || !result.IsSuccess() || result.Data == nil {
			return nil, errors.New("Waffo order query failed")
		}
		purchase := result.Data
		if purchase.MerchantOrderID != ext.TradeNo || purchase.AcquiringOrderID == "" || (ext.GatewayTradeId != "" && purchase.AcquiringOrderID != ext.GatewayTradeId) {
			return nil, model.ErrPaymentMethodMismatch
		}
		if purchase.OrderStatus != "PAY_SUCCESS" {
			return nil, nil
		}
		facts.PaidAmount, err = decimal.NewFromString(purchase.OrderAmount)
		if err != nil {
			return nil, model.ErrPaymentAmountMismatch
		}
		facts.PaidCurrency, facts.Environment = purchase.OrderCurrency, ext.ProviderEnvironment
		if purchase.MerchantInfo != nil {
			facts.MerchantId = purchase.MerchantInfo.MerchantID
		}
		facts.GatewayTradeId, facts.TxHash, facts.Payload = purchase.AcquiringOrderID, purchase.AcquiringOrderID, common.GetJsonString(purchase)
	case model.PaymentProviderWaffoPancake:
		return service.QueryWaffoPancakePayment(ctx, ext)
	default:
		return nil, model.ErrPaymentMethodMismatch
	}
	return facts, nil
}

// queryHostedJSON is the bounded TLS transport shared by Stripe and Creem.
func queryHostedJSON(ctx context.Context, endpoint, header, credential string, target any) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set(header, credential)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", errors.New("payment provider query is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("payment provider query returned HTTP %d", response.StatusCode)
	}
	const maxPayload = 1 << 20
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxPayload+1))
	if err != nil || len(payload) > maxPayload {
		return "", errors.New("payment provider query response is invalid")
	}
	if err := common.Unmarshal(payload, target); err != nil {
		return "", errors.New("payment provider query response is invalid")
	}
	return string(payload), nil
}
