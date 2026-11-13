package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

type WaffoPancakePayRequest struct {
	PaymentScene string `json:"payment_scene"`
	Amount       int64  `json:"amount"`
	PromoCode    string `json:"promo_code,omitempty"`
}

func RequestWaffoPancakeAmount(c *gin.Context) {
	if !isWaffoPancakeTopUpEnabled() {
		paymentError(c, common.NewMessage("Waffo Pancake configuration is incomplete"))
		return
	}
	var req WaffoPancakePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}

	if req.Amount < int64(setting.WaffoPancakeMinTopUp) {
		paymentError(c, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": setting.WaffoPancakeMinTopUp}))
		return
	}
	if _, ok := waffoPancakePurchaseUnits(req.Amount); !ok {
		paymentError(c, common.NewMessage("Top-up amount must be a whole number of quota units"))
		return
	}
	id := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, id, req.Amount) {
		return
	}

	group, err := model.GetUserGroup(id, true)
	if err != nil {
		paymentError(c, common.NewMessage("Failed to get the user group"))
		return
	}

	payMoney := getWaffoPancakePayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		paymentError(c, common.NewMessage("Payment amount is too low"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": fmt.Sprintf("%.2f", payMoney)})
}

func getWaffoPancakePayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && ds > 0 {
		discount = ds
	}

	payMoney := dAmount.
		Mul(decimal.NewFromFloat(setting.WaffoPancakeUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount))

	return payMoney.InexactFloat64()
}

func waffoPancakePurchaseUnits(amount int64) (int64, bool) {
	if amount <= 0 {
		return 0, false
	}
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount, true
	}
	scale := int64(common.QuotaPerUnit)
	if scale < 1 {
		scale = 1
	}
	if amount%scale != 0 {
		return 0, false
	}
	return amount / scale, true
}

func formatWaffoPancakeAmount(payMoney float64) string {
	return decimal.NewFromFloat(payMoney).StringFixed(2)
}

func getWaffoPancakeBuyerEmail(user *model.User) string {
	if user != nil && strings.TrimSpace(user.Email) != "" {
		return user.Email
	}
	return ""
}

// The admin config endpoints below accept typed-but-not-yet-saved creds in
// the body and fall back to persisted creds when the body is blank (see
// resolveWaffoPancakeAdminCreds). Only SaveWaffoPancake writes to OptionMap.

type saveWaffoPancakeRequest struct {
	MerchantID  string `json:"merchant_id"`
	PrivateKey  string `json:"private_key"`
	ReturnURL   string `json:"return_url"`
	StoreID     string `json:"store_id"`
	ProductID   string `json:"product_id"`
	Environment string `json:"environment"`
}

type createWaffoPancakePairRequest struct {
	MerchantID string `json:"merchant_id"`
	PrivateKey string `json:"private_key"`
	ReturnURL  string `json:"return_url"`
}

// SaveWaffoPancake atomically persists all five operator-controlled fields.
// Catalog / pair endpoints are transient — only this one writes the OptionMap.
func SaveWaffoPancake(c *gin.Context) {
	var req saveWaffoPancakeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	if err := service.SaveWaffoPancakeConfig(
		c.Request.Context(),
		req.MerchantID,
		req.PrivateKey,
		req.ReturnURL,
		req.StoreID,
		req.ProductID,
		req.Environment,
	); err != nil {
		logger.LogError(c.Request.Context(), common.LogText(
			"Waffo Pancake failed to save config store_id=%q product_id=%q error=%q",
			req.StoreID, req.ProductID, err.Error(),
		))
		paymentError(c, common.NewMessage("Failed to save configuration"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"product_id": setting.WaffoPancakeProductID,
			"store_id":   setting.WaffoPancakeStoreID,
		},
	})
}

// resolveWaffoPancakeAdminCreds prefers body creds (typed-but-not-yet-saved
// values, for verification) and falls back to persisted creds when the body
// is blank (so returning admins don't have to re-paste the private key,
// which is stripped from GET /api/option/).
func resolveWaffoPancakeAdminCreds(bodyMerchantID, bodyPrivateKey string) (string, string) {
	m := strings.TrimSpace(bodyMerchantID)
	k := strings.TrimSpace(bodyPrivateKey)
	if m == "" && k == "" {
		return setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey
	}
	return m, k
}

// CreateWaffoPancakePair mints a Store + OnetimeProduct pair in one round-
// trip. Surfaces an orphan-store flag when the product half fails so the
// frontend can preselect / retry without losing context.
func CreateWaffoPancakePair(c *gin.Context) {
	var req createWaffoPancakePairRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			paymentError(c, common.NewMessage("Invalid parameters"))
			return
		}
	}
	merchantID, privateKey := resolveWaffoPancakeAdminCreds(req.MerchantID, req.PrivateKey)
	if merchantID == "" || privateKey == "" {
		paymentError(c, common.NewMessage("Waffo Pancake credentials are not configured"))
		return
	}
	result, err := service.CreateWaffoPancakePrimaryPair(
		c.Request.Context(), merchantID, privateKey, req.ReturnURL,
	)
	if err != nil {
		orphan := result != nil && result.OrphanStore
		logger.LogError(c.Request.Context(), common.LogText(
			"Waffo Pancake failed to create store and product orphan_store=%t store_id=%q error=%q",
			orphan, func() string {
				if result == nil {
					return ""
				}
				return result.StoreID
			}(), err.Error(),
		))
		data := gin.H{"error": err.Error()}
		if orphan {
			data["store_id"] = result.StoreID
			data["store_name"] = result.StoreName
			data["orphan_store"] = true
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": data})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"store_id":     result.StoreID,
			"store_name":   result.StoreName,
			"product_id":   result.ProductID,
			"product_name": result.ProductName,
		},
	})
}

// ListWaffoPancakeCatalog returns the merchant's Stores + OnetimeProducts.
// Doubles as a credential probe (a successful 200 proves the resolved creds
// authenticate). See resolveWaffoPancakeAdminCreds for credential resolution.
func ListWaffoPancakeCatalog(c *gin.Context) {
	// 私钥只从 POST 正文读取。查询串会进浏览器历史和访问日志。
	var body struct {
		MerchantID string `json:"merchant_id"`
		PrivateKey string `json:"private_key"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := common.DecodeJson(c.Request.Body, &body); err != nil {
			paymentError(c, common.NewMessage("Invalid parameters"))
			return
		}
	}
	merchantID, privateKey := resolveWaffoPancakeAdminCreds(body.MerchantID, body.PrivateKey)
	if merchantID == "" || privateKey == "" {
		paymentError(c, common.NewMessage("Waffo Pancake credentials are not configured"))
		return
	}
	catalog, err := service.ListWaffoPancakeCatalog(c.Request.Context(), merchantID, privateKey)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText(
			"Waffo Pancake failed to fetch store and product catalog error=%q", err.Error(),
		))
		paymentError(c, common.NewMessage("Failed to fetch the catalog"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": catalog})
}

func getWaffoPancakeBuyerIdentity(user *model.User) string {
	if user == nil {
		return ""
	}
	return service.WaffoPancakeBuyerIdentityFromUserID(user.Id)
}

func RequestWaffoPancakePay(c *gin.Context) {
	if !isWaffoPancakeTopUpEnabled() {
		paymentError(c, common.NewMessage("Waffo Pancake configuration is incomplete"))
		return
	}

	var req WaffoPancakePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	if req.Amount < int64(setting.WaffoPancakeMinTopUp) {
		paymentError(c, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": setting.WaffoPancakeMinTopUp}))
		return
	}
	units, ok := waffoPancakePurchaseUnits(req.Amount)
	if !ok {
		paymentError(c, common.NewMessage("Top-up amount must be a whole number of quota units"))
		return
	}
	id := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, id, req.Amount) {
		return
	}

	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		paymentError(c, common.NewMessage("User does not exist"))
		return
	}

	group, err := model.GetUserGroup(id, true)
	if err != nil {
		paymentError(c, common.NewMessage("Failed to get the user group"))
		return
	}

	payMoney := getWaffoPancakePayMoney(req.Amount, group)
	if payMoney < 0.01 {
		paymentError(c, common.NewMessage("Payment amount is too low"))
		return
	}

	tradeNo := fmt.Sprintf("WAFFO_PANCAKE-%d-%d-%s", id, time.Now().UnixMilli(), randstr.String(6))
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          units,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := createAdtyHostedOrder(c, topUp, decimal.NewFromInt(units).Mul(decimal.NewFromFloat(common.QuotaPerUnit)), "USD", setting.WaffoPancakeProductID, setting.WaffoPancakeEnvironment, req.PromoCode, req.PaymentScene); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo Pancake failed to create top-up order user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}

	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		ProductID:     setting.WaffoPancakeProductID,
		BuyerIdentity: getWaffoPancakeBuyerIdentity(user),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      formatWaffoPancakeAmount(payMoney),
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: tradeNo,
		Metadata:                map[string]string{"adty_product": setting.WaffoPancakeProductID},
	})
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo Pancake failed to create checkout session user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}
	if err := model.UpdateAdtyTopupOrderFields(tradeNo, map[string]any{"pay_url": session.CheckoutURL, "gateway_trade_id": session.OrderID}); err != nil {
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}
	logger.LogInfo(c.Request.Context(), common.LogText("Waffo Pancake top-up order created user_id=%d trade_no=%s session_id=%s amount=%d money=%.2f", id, tradeNo, session.SessionID, req.Amount, payMoney))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url":     session.CheckoutURL,
			"session_id":       session.SessionID,
			"expires_at":       session.ExpiresAt,
			"order_id":         tradeNo,
			"token":            session.Token,
			"token_expires_at": session.TokenExpiresAt,
		},
	})
}

var verifyWaffoPancakeWebhook = func(payload, signature string) (*service.WaffoPancakeWebhookEvent, error) {
	return service.VerifyConfiguredWaffoPancakeWebhook(payload, signature)
}

func WaffoPancakeWebhook(c *gin.Context) {
	if !isWaffoPancakeWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), common.LogText("Waffo Pancake webhook rejected reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.String(http.StatusForbidden, "webhook disabled")
		return
	}

	// :env splits test vs prod traffic at the routing layer — operator
	// registers each URL in the matching webhook slot in Pancake's dashboard.
	// We then enforce event.mode == expectedEnv to catch mis-registrations.
	expectedEnv := strings.TrimSpace(c.Param("env"))
	if expectedEnv != "test" && expectedEnv != "prod" {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"Waffo Pancake webhook invalid env path segment env=%q path=%q client_ip=%s",
			expectedEnv, c.Request.RequestURI, c.ClientIP(),
		))
		c.String(http.StatusNotFound, "unknown env")
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo Pancake webhook failed to read request body path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.String(http.StatusBadRequest, "bad request")
		return
	}

	signature := c.GetHeader("X-Waffo-Signature")
	logger.LogInfo(c.Request.Context(), common.LogText("Waffo Pancake webhook received request path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))

	event, err := verifyWaffoPancakeWebhook(string(bodyBytes), signature)
	if err != nil {
		logger.LogWarn(c.Request.Context(), common.LogText("Waffo Pancake webhook signature verification failed path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.String(http.StatusUnauthorized, "invalid signature")
		return
	}

	if !strings.EqualFold(strings.TrimSpace(event.Mode), expectedEnv) {
		logger.LogError(c.Request.Context(), common.LogText(
			"Waffo Pancake webhook env mismatch expected=%q actual_mode=%q event_id=%s order_id=%s client_ip=%s",
			expectedEnv, event.Mode, event.ID, event.Data.OrderID, c.ClientIP(),
		))
		c.String(http.StatusOK, "OK")
		return
	}

	logger.LogInfo(c.Request.Context(), common.LogText("Waffo Pancake webhook signature verified event_type=%s event_id=%s order_id=%s client_ip=%s", event.NormalizedEventType(), event.ID, event.Data.OrderID, c.ClientIP()))
	eventType := event.NormalizedEventType()
	if eventType != "order.completed" && eventType != "refund.succeeded" {
		c.String(http.StatusOK, "OK")
		return
	}

	tradeNo, err := service.ResolveWaffoPancakeTradeNo(event)
	if err != nil {
		if errors.Is(err, service.ErrWaffoPancakeTransient) {
			logger.LogError(c.Request.Context(), common.LogText(
				"Waffo Pancake webhook order lookup failed event_id=%s order_id=%s client_ip=%s error=%q",
				event.ID, event.Data.OrderID, c.ClientIP(), err.Error(),
			))
			c.String(http.StatusInternalServerError, "retry")
			return
		}
		// 订单不存在或买家身份不对是永久错误，回 200 避免无限重试。
		logger.LogError(c.Request.Context(), common.LogText(
			"Waffo Pancake webhook failed to resolve order event_id=%s order_id=%s buyer_identity=%q client_ip=%s error=%q",
			event.ID, event.Data.OrderID, event.Data.MerchantProvidedBuyerIdentity, c.ClientIP(), err.Error(),
		))
		c.String(http.StatusOK, "OK")
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	// Reversals record their receipt after resolving the original order below.
	if eventType != "refund.succeeded" {
		if err := model.RecordAdtyVerifiedCallback(tradeNo, model.PaymentProviderWaffoPancake, eventType); err != nil {
			c.String(http.StatusInternalServerError, "retry")
			return
		}
	}

	ext, lookupErr := model.GetAdtyTopupOrder(tradeNo)
	if lookupErr != nil {
		c.String(http.StatusInternalServerError, "retry")
		return
	}
	if eventType == "refund.succeeded" {
		if ext != nil {
			refundID := event.Data.RefundTicketMerchantExternalID
			if refundID == "" {
				refundID = event.EventID
			}
			if refundID == "" {
				refundID = event.ID
			}
			if err := recordHostedReversal(model.PaymentProviderWaffoPancake, eventType, refundID, tradeNo, event.Data.OrderID, string(bodyBytes)); err != nil {
				c.String(http.StatusInternalServerError, "retry")
				return
			}
			c.String(http.StatusOK, "OK")
			return
		}
		if err := model.RefundWaffoPancake(tradeNo); err != nil {
			logger.LogError(c.Request.Context(), common.LogText("Waffo Pancake refund failed trade_no=%s event_id=%s client_ip=%s error=%q", tradeNo, event.ID, c.ClientIP(), err.Error()))
			c.String(http.StatusInternalServerError, "retry")
			return
		}
		c.String(http.StatusOK, "OK")
		return
	}

	if ext != nil {
		amount, parseErr := decimal.NewFromString(event.Data.Amount)
		purchase := decimal.Zero
		tax := decimal.Zero
		if strings.TrimSpace(event.Data.TaxAmount) != "" {
			var taxErr error
			tax, taxErr = decimal.NewFromString(event.Data.TaxAmount)
			if taxErr != nil {
				parseErr = taxErr
			}
		}
		if event.Data.Subtotal != nil || event.Data.Total != nil {
			if event.Data.Subtotal == nil || event.Data.Total == nil {
				parseErr = model.ErrPaymentAmountMismatch
			} else {
				var subtotalErr, totalErr error
				purchase, subtotalErr = decimal.NewFromString(*event.Data.Subtotal)
				amount, totalErr = decimal.NewFromString(*event.Data.Total)
				if subtotalErr != nil || totalErr != nil || !purchase.IsPositive() || !amount.IsPositive() || !purchase.Add(tax).Equal(amount) {
					parseErr = model.ErrPaymentAmountMismatch
				}
			}
		} else if !tax.IsZero() {
			// The SDK does not define whether legacy amount includes tax.
			// Authenticated GraphQL recovery supplies explicit subtotal/total.
			parseErr = model.ErrPaymentAmountMismatch
		}
		if tax.IsNegative() {
			parseErr = model.ErrPaymentAmountMismatch
		}
		if parseErr != nil {
			recordAdtyPaymentReview(model.PaymentProviderWaffoPancake, tradeNo, event.Data.OrderID, string(bodyBytes), model.ErrPaymentAmountMismatch)
			c.String(http.StatusBadRequest, "invalid amount")
			return
		}
		_, err = model.CompleteAdtyTopUp(tradeNo, model.AdtyPaymentFacts{
			Provider: model.PaymentProviderWaffoPancake, Method: model.PaymentMethodWaffoPancake,
			PaidAmount: amount, PurchaseAmount: purchase, PaidCurrency: event.Data.Currency, GatewayTradeId: event.Data.OrderID, TxHash: event.Data.OrderID,
			Environment: event.Mode, Source: model.AdtyPaymentSourceCallback, Payload: string(bodyBytes),
			ProductId: event.Data.OrderMetadata["adty_product"], MerchantId: event.StoreID,
		}, c.ClientIP())
	} else {
		err = model.RechargeWaffoPancake(tradeNo)
	}
	if err != nil {
		if errors.Is(err, model.ErrPaymentOrphanNotCredit) {
			recordAdtyPaymentReview(model.PaymentProviderWaffoPancake, tradeNo, event.Data.OrderID, string(bodyBytes), err)
			c.String(http.StatusOK, "OK")
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("Waffo Pancake top-up processing failed trade_no=%s event_id=%s order_id=%s client_ip=%s error=%q", tradeNo, event.ID, event.Data.OrderID, c.ClientIP(), err.Error()))
		c.String(http.StatusInternalServerError, "retry")
		return
	}

	logger.LogInfo(c.Request.Context(), common.LogText("Waffo Pancake top-up succeeded trade_no=%s event_id=%s order_id=%s client_ip=%s", tradeNo, event.ID, event.Data.OrderID, c.ClientIP()))
	c.String(http.StatusOK, "OK")
}
