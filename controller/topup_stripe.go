package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

var stripeAdaptor = &StripeAdaptor{}

const stripeMaxTopupUnits int64 = 10000

// stripeNewCheckoutSession 是创建 Checkout 的出口。测试可以换成不访问 Stripe 的实现。
var stripeNewCheckoutSession = session.New

// StripePayRequest represents a payment request for Stripe checkout.
type StripePayRequest struct {
	PaymentScene string `json:"payment_scene"`
	// Amount is the quantity of units to purchase.
	Amount int64 `json:"amount"`
	// PaymentMethod specifies the payment method (e.g., "stripe").
	PaymentMethod string `json:"payment_method"`
	// SuccessURL is the optional custom URL to redirect after successful payment.
	// If empty, defaults to the server's console log page.
	SuccessURL string `json:"success_url,omitempty"`
	// CancelURL is the optional custom URL to redirect when payment is canceled.
	// If empty, defaults to the server's console topup page.
	CancelURL string `json:"cancel_url,omitempty"`
	PromoCode string `json:"promo_code,omitempty"`
}

type StripeAdaptor struct {
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	if req.Amount < getStripeMinTopup() {
		paymentError(c, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": getStripeMinTopup()}))
		return
	}
	if req.Amount > getStripeMaxTopup() {
		paymentError(c, common.NewMessage("Top-up amount cannot exceed {{max}}", map[string]any{"max": getStripeMaxTopup()}))
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		paymentError(c, common.NewMessage("Failed to get the user group"))
		return
	}
	pay, ok := stripeQuotedPay(req.Amount, group)
	if !ok {
		paymentError(c, common.NewMessage("Top-up amount must be a whole number of quota units"))
		return
	}
	if rejectInvalidCreditedQuota(c, id, stripeCreditedQuota(pay.Quantity)) {
		return
	}
	if !pay.Money.GreaterThan(decimal.NewFromFloat(0.01)) {
		paymentError(c, common.NewMessage("Payment amount is too low"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": pay.Money.StringFixed(2)})
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if req.PaymentMethod != model.PaymentMethodStripe {
		paymentError(c, common.NewMessage("Unsupported payment channel"))
		return
	}
	if !isStripeTopUpEnabled() {
		paymentError(c, common.NewMessage("Stripe payment is not available"))
		return
	}
	if req.Amount < getStripeMinTopup() {
		common.ApiErrorStatus(c, http.StatusOK, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": getStripeMinTopup()}), gin.H{"data": 10})
		return
	}
	if req.Amount > getStripeMaxTopup() {
		common.ApiErrorStatus(c, http.StatusOK, common.NewMessage("Top-up amount cannot exceed {{max}}", map[string]any{"max": getStripeMaxTopup()}), gin.H{"data": 10})
		return
	}

	if req.SuccessURL != "" && common.ValidateRedirectURL(req.SuccessURL) != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Payment success redirect URL is not in the trusted domain list"), gin.H{"data": ""})
		return
	}

	if req.CancelURL != "" && common.ValidateRedirectURL(req.CancelURL) != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Payment cancel redirect URL is not in the trusted domain list"), gin.H{"data": ""})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		paymentError(c, common.NewMessage("User does not exist"))
		return
	}
	pay, ok := stripeQuotedPay(req.Amount, user.Group)
	if !ok {
		common.ApiErrorStatus(c, http.StatusOK, common.NewMessage("Top-up amount must be a whole number of quota units"), gin.H{"data": 10})
		return
	}
	if rejectInvalidCreditedQuota(c, id, stripeCreditedQuota(pay.Quantity)) {
		return
	}

	reference := fmt.Sprintf("new-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	topUp := &model.TopUp{
		UserId:          id,
		Amount:          pay.Quantity,
		Money:           pay.Money.InexactFloat64(),
		TradeNo:         referenceId,
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	err = createAdtyHostedOrder(c, topUp, stripeCreditedQuota(pay.Quantity), "USD", "wallet_topup", stripePaymentEnvironment(setting.StripeApiSecret), req.PromoCode, req.PaymentScene)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Stripe failed to create top-up order user_id=%d trade_no=%s amount=%d error=%q", id, referenceId, req.Amount, err.Error()))
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}
	checkout, err := genStripeCheckout(referenceId, user.StripeCustomer, user.Email, pay.Money, req.SuccessURL, req.CancelURL)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}
	if err := model.UpdateAdtyTopupOrderFields(referenceId, map[string]any{"gateway_trade_id": checkout.ID, "pay_url": checkout.URL}); err != nil {
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}
	logger.LogInfo(c.Request.Context(), common.LogText("Stripe top-up order created user_id=%d trade_no=%s amount=%d money=%.2f", id, referenceId, pay.Quantity, topUp.Money))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": checkout.URL,
			"order_id": referenceId,
		},
	})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	ctx := c.Request.Context()
	if !isStripeWebhookEnabled() {
		logger.LogWarn(ctx, common.LogText("Stripe webhook rejected reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(ctx, common.LogText("Stripe webhook failed to read request body path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	logger.LogInfo(ctx, common.LogText("Stripe webhook received request path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
	event, err := webhook.ConstructEventWithOptions(payload, signature, setting.StripeWebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		logger.LogWarn(ctx, common.LogText("Stripe webhook signature verification failed path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	callerIp := c.ClientIP()
	logger.LogInfo(ctx, common.LogText("Stripe webhook signature verified event_type=%s client_ip=%s path=%q", string(event.Type), callerIp, c.Request.RequestURI))
	var handleErr error
	if reference := event.GetObjectValue("client_reference_id"); reference != "" {
		if err := model.RecordAdtyVerifiedCallback(reference, model.PaymentProviderStripe, string(event.Type)); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
	}
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		handleErr = sessionCompleted(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionExpired:
		sessionExpired(ctx, event)
	case stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		handleErr = sessionAsyncPaymentSucceeded(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		sessionAsyncPaymentFailed(ctx, event, callerIp)
	case stripe.EventTypeChargeRefunded, stripe.EventTypeChargeDisputeCreated, stripe.EventTypeChargeDisputeClosed, stripe.EventTypeChargeDisputeFundsWithdrawn, stripe.EventTypeChargeDisputeFundsReinstated:
		resourceID := event.GetObjectValue("id")
		if event.Type == stripe.EventTypeChargeRefunded {
			// A charge can receive several partial refunds. Resending the same
			// cumulative refund is idempotent; a later refund needs a new review.
			resourceID += ":" + event.GetObjectValue("amount_refunded")
		}
		handleErr = recordHostedReversal(model.PaymentProviderStripe, string(event.Type), resourceID, event.GetObjectValue("metadata", "adty_trade_no"), event.GetObjectValue("payment_intent"), string(payload))
	default:
		logger.LogInfo(ctx, common.LogText("Stripe webhook ignored event event_type=%s client_ip=%s", string(event.Type), callerIp))
	}
	if handleErr != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusOK)
}

func sessionCompleted(ctx context.Context, event stripe.Event, callerIp string) error {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "complete" != status {
		logger.LogWarn(ctx, common.LogText("Stripe checkout.completed has unexpected status, ignored trade_no=%s status=%s client_ip=%s", referenceId, status, callerIp))
		return nil
	}

	paymentStatus := event.GetObjectValue("payment_status")
	if paymentStatus != "paid" {
		if paymentStatus == "no_payment_required" {
			recordAdtyPaymentReview(model.PaymentProviderStripe, referenceId, event.GetObjectValue("id"), string(event.Data.Raw), model.ErrPaymentAmountMismatch)
		}
		logger.LogInfo(ctx, common.LogText("Stripe Checkout payment not completed, waiting for async result trade_no=%s payment_status=%s client_ip=%s", referenceId, paymentStatus, callerIp))
		return nil
	}

	return fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentSucceeded handles delayed payment methods (bank transfer, SEPA, etc.)
// that confirm payment after the checkout session completes.
func sessionAsyncPaymentSucceeded(ctx context.Context, event stripe.Event, callerIp string) error {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogInfo(ctx, common.LogText("Stripe async payment succeeded trade_no=%s client_ip=%s", referenceId, callerIp))

	return fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentFailed marks orders as failed when delayed payment methods
// ultimately fail (e.g. bank transfer not received, SEPA rejected).
func sessionAsyncPaymentFailed(ctx context.Context, event stripe.Event, callerIp string) {
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogWarn(ctx, common.LogText("Stripe async payment failed trade_no=%s client_ip=%s", referenceId, callerIp))

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, common.LogText("Stripe async payment failed event missing order number client_ip=%s", callerIp))
		return
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)

	topUp := model.GetTopUpByTradeNo(referenceId)
	if topUp == nil {
		logger.LogWarn(ctx, common.LogText("Stripe async payment failed but local order not found trade_no=%s client_ip=%s", referenceId, callerIp))
		return
	}

	if topUp.PaymentProvider != model.PaymentProviderStripe {
		logger.LogWarn(ctx, common.LogText("Stripe async payment failed but order payment gateway mismatch trade_no=%s payment_provider=%s client_ip=%s", referenceId, topUp.PaymentProvider, callerIp))
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		logger.LogInfo(ctx, common.LogText("Stripe async payment failed but order not pending, ignored trade_no=%s status=%s client_ip=%s", referenceId, topUp.Status, callerIp))
		return
	}

	topUp.Status = common.TopUpStatusFailed
	if err := topUp.Update(); err != nil {
		logger.LogError(ctx, common.LogText("Stripe failed to mark top-up order as failed trade_no=%s client_ip=%s error=%q", referenceId, callerIp, err.Error()))
		return
	}
	logger.LogInfo(ctx, common.LogText("Stripe top-up order marked as failed trade_no=%s client_ip=%s", referenceId, callerIp))
}

// fulfillOrder is the shared logic for crediting quota after payment is confirmed.
func fulfillOrder(ctx context.Context, event stripe.Event, referenceId string, customerId string, callerIp string) error {
	if len(referenceId) == 0 {
		logger.LogWarn(ctx, common.LogText("Stripe order completion missing order number client_ip=%s", callerIp))
		return nil
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)

	topUp := model.GetTopUpByTradeNo(referenceId)
	if topUp == nil {
		logger.LogWarn(ctx, common.LogText("Stripe top-up order not found trade_no=%s client_ip=%s", referenceId, callerIp))
		return fmt.Errorf("stripe top-up order not found")
	}
	paidCents, paidOK := stripeAmountTotalCents(event)
	purchaseAmount := decimal.Zero
	if raw := event.GetObjectValue("amount_subtotal"); raw != "" {
		subtotal, parseErr := strconv.ParseInt(raw, 10, 64)
		discount, discountErr := stripeCheckoutDetailCents(event, "amount_discount")
		tax, taxErr := stripeCheckoutDetailCents(event, "amount_tax")
		shipping, shippingErr := stripeCheckoutDetailCents(event, "amount_shipping")
		total := decimal.NewFromInt(subtotal).Sub(decimal.NewFromInt(discount)).Add(decimal.NewFromInt(tax)).Add(decimal.NewFromInt(shipping))
		paidOK = paidOK && parseErr == nil && discountErr == nil && taxErr == nil && shippingErr == nil && subtotal > 0 && discount <= subtotal && subtotal == stripeMoneyCents(topUp.Money) && total.Equal(decimal.NewFromInt(paidCents))
		purchaseAmount = decimal.NewFromInt(subtotal).Div(decimal.NewFromInt(100))
	} else {
		paidOK = paidOK && paidCents == stripeMoneyCents(topUp.Money)
	}
	if !paidOK || paidCents <= 0 {
		logger.LogError(ctx, common.LogText("Stripe paid amount does not match the quote trade_no=%s paid_cents=%d expected_cents=%d client_ip=%s", referenceId, paidCents, stripeMoneyCents(topUp.Money), callerIp))
		return fmt.Errorf("stripe paid amount mismatch")
	}
	ext, err := model.GetAdtyTopupOrder(referenceId)
	if err != nil {
		return err
	}
	if ext != nil {
		_, err = model.CompleteAdtyTopUp(referenceId, model.AdtyPaymentFacts{
			Provider: model.PaymentProviderStripe, Method: model.PaymentMethodStripe,
			PaidAmount: decimal.NewFromInt(paidCents).Div(decimal.NewFromInt(100)), PaidCurrency: event.GetObjectValue("currency"),
			PurchaseAmount: purchaseAmount,
			GatewayTradeId: event.GetObjectValue("id"), TxHash: event.GetObjectValue("payment_intent"),
			ProductId: event.GetObjectValue("metadata", "adty_product"), Environment: hostedPaymentEnvironment(!event.Livemode),
			Source: model.AdtyPaymentSourceCallback,
		}, callerIp)
	} else {
		if !strings.EqualFold(event.GetObjectValue("currency"), "USD") {
			return model.ErrPaymentAmountMismatch
		}
		err = model.Recharge(referenceId, customerId, callerIp)
	}
	if err != nil {
		if errors.Is(err, model.ErrPaymentOrphanNotCredit) {
			recordAdtyPaymentReview(model.PaymentProviderStripe, referenceId, event.GetObjectValue("id"), string(event.Data.Raw), err)
			return nil
		}
		logger.LogError(ctx, common.LogText("Stripe top-up processing failed trade_no=%s event_type=%s client_ip=%s error=%q", referenceId, string(event.Type), callerIp, err.Error()))
		return err
	}

	total, _ := strconv.ParseFloat(event.GetObjectValue("amount_total"), 64)
	currency := strings.ToUpper(event.GetObjectValue("currency"))
	logger.LogInfo(ctx, common.LogText("Stripe top-up succeeded trade_no=%s amount_total=%.2f currency=%s event_type=%s client_ip=%s", referenceId, total/100, currency, string(event.Type), callerIp))
	return nil
}

func sessionExpired(ctx context.Context, event stripe.Event) {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		logger.LogWarn(ctx, common.LogText("Stripe checkout.expired has unexpected status, ignored trade_no=%s status=%s", referenceId, status))
		return
	}

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, common.LogText("Stripe checkout.expired missing order number"))
		return
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusExpired)
	if errors.Is(err, model.ErrTopUpNotFound) {
		logger.LogWarn(ctx, common.LogText("Stripe top-up order not found, cannot mark as expired trade_no=%s", referenceId))
		return
	}
	if err != nil {
		logger.LogError(ctx, common.LogText("Stripe top-up order expiry processing failed trade_no=%s error=%q", referenceId, err.Error()))
		return
	}

	logger.LogInfo(ctx, common.LogText("Stripe top-up order expired trade_no=%s", referenceId))
}

// genStripeLink generates a Stripe Checkout session URL for payment.
// It creates a new checkout session with the specified parameters and returns the payment URL.
//
// Parameters:
//   - referenceId: unique reference identifier for the transaction
//   - customerId: existing Stripe customer ID (empty string if new customer)
//   - email: customer email address for new customer creation
//   - amount: quantity of units to purchase
//   - successURL: custom URL to redirect after successful payment (empty for default)
//   - cancelURL: custom URL to redirect when payment is canceled (empty for default)
//
// Returns the checkout session URL or an error if the session creation fails.
func genStripeLink(referenceId string, customerId string, email string, payMoney decimal.Decimal, successURL string, cancelURL string) (string, error) {
	checkout, err := genStripeCheckout(referenceId, customerId, email, payMoney, successURL, cancelURL)
	if err != nil {
		return "", err
	}
	return checkout.URL, nil
}

func genStripeCheckout(referenceId string, customerId string, email string, payMoney decimal.Decimal, successURL string, cancelURL string) (*stripe.CheckoutSession, error) {
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return nil, errors.New("invalid Stripe API key")
	}

	stripe.Key = setting.StripeApiSecret

	// Use custom URLs if provided, otherwise use defaults
	if successURL == "" {
		successURL = paymentReturnPath("/usage-logs")
	}
	if cancelURL == "" {
		cancelURL = paymentReturnPath("/wallet")
	}

	cents := payMoney.Round(2).Mul(decimal.NewFromInt(100)).IntPart()
	if cents <= 0 {
		return nil, errors.New("stripe payment amount is too small")
	}
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(successURL),
		CancelURL:         stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Quantity: stripe.Int64(1),
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String(string(stripe.CurrencyUSD)),
					UnitAmount: stripe.Int64(cents),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Top-up"),
					},
				},
			},
		},
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
	}
	params.AddMetadata("adty_product", "wallet_topup")
	params.AddMetadata("adty_trade_no", referenceId)
	params.AllowPromotionCodes = stripe.Bool(setting.StripePromotionCodesEnabled)
	params.PaymentIntentData = &stripe.CheckoutSessionPaymentIntentDataParams{Metadata: map[string]string{"adty_trade_no": referenceId, "adty_product": "wallet_topup"}}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}

		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := stripeNewCheckoutSession(params)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// stripeQuote 是一次 Stripe 充值的报价。应付金额、Checkout 实扣和订单 money 都用 Money；入账额度是 Quantity 乘额度单位，不再乘分组倍率或单价。
type stripeQuote struct {
	Quantity int64
	Money    decimal.Decimal
}

func stripeQuotedPay(requestAmount int64, group string) (stripeQuote, bool) {
	quantity, ok := stripeCheckoutQuantity(requestAmount)
	if !ok {
		return stripeQuote{}, false
	}
	ratio := common.GetTopupGroupRatio(group)
	if ratio <= 0 {
		ratio = 1
	}
	discount := 1.0
	if ds, found := operation_setting.GetPaymentSetting().AmountDiscount[int(requestAmount)]; found && ds > 0 {
		discount = ds
	}
	money := decimal.NewFromInt(quantity).
		Mul(decimal.NewFromFloat(setting.StripeUnitPrice)).
		Mul(decimal.NewFromFloat(ratio)).
		Mul(decimal.NewFromFloat(discount)).
		Round(2)
	return stripeQuote{Quantity: quantity, Money: money}, true
}

func stripeCreditedQuota(quantity int64) decimal.Decimal {
	return decimal.NewFromInt(quantity).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
}

func stripeMoneyCents(money float64) int64 {
	return decimal.NewFromFloat(money).Round(2).Mul(decimal.NewFromInt(100)).IntPart()
}

func stripeAmountTotalCents(event stripe.Event) (int64, bool) {
	raw := strings.TrimSpace(event.GetObjectValue("amount_total"))
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func stripeCheckoutDetailCents(event stripe.Event, key string) (int64, error) {
	raw := event.GetObjectValue("total_details", key)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, model.ErrPaymentAmountMismatch
	}
	return value, nil
}

func stripeAmountScale() int64 {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return 1
	}
	scale := int64(common.QuotaPerUnit)
	if scale < 1 {
		return 1
	}
	return scale
}

func getStripeMinTopup() int64 {
	return int64(setting.StripeMinTopUp) * stripeAmountScale()
}

func getStripeMaxTopup() int64 {
	return stripeMaxTopupUnits * stripeAmountScale()
}

// stripeCheckoutQuantity 把请求金额换成 Stripe 商品数量。TOKENS 展示下数量是金额除以额度单位，必须整除。
func stripeCheckoutQuantity(amount int64) (int64, bool) {
	scale := stripeAmountScale()
	if amount <= 0 || amount%scale != 0 {
		return 0, false
	}
	return amount / scale, true
}
