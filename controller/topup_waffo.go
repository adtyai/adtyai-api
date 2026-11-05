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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/thanhpk/randstr"
	waffo "github.com/waffo-com/waffo-go"
	"github.com/waffo-com/waffo-go/config"
	"github.com/waffo-com/waffo-go/core"
	"github.com/waffo-com/waffo-go/types/order"
)

func getWaffoSDK() (*waffo.Waffo, error) {
	env := config.Sandbox
	apiKey := setting.WaffoSandboxApiKey
	privateKey := setting.WaffoSandboxPrivateKey
	publicKey := setting.WaffoSandboxPublicCert
	if !setting.WaffoSandbox {
		env = config.Production
		apiKey = setting.WaffoApiKey
		privateKey = setting.WaffoPrivateKey
		publicKey = setting.WaffoPublicCert
	}
	builder := config.NewConfigBuilder().
		APIKey(apiKey).
		PrivateKey(privateKey).
		WaffoPublicKey(publicKey).
		Environment(env)
	if setting.WaffoMerchantId != "" {
		builder = builder.MerchantID(setting.WaffoMerchantId)
	}
	cfg, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return waffo.New(cfg), nil
}

// waffoCreateOrder 是向 Waffo 建单的出口。测试可以换成不访问 Waffo 的实现。
var waffoCreateOrder = func(ctx context.Context, sdk *waffo.Waffo, params *order.CreateOrderParams) (*core.ApiResponse[order.CreateOrderData], error) {
	return sdk.Order().Create(ctx, params, nil)
}

func getWaffoUserEmail(user *model.User) (string, bool) {
	if user == nil {
		return "", false
	}
	email := model.NormalizeEmail(user.Email)
	if email == "" || !strings.Contains(email, "@") {
		return "", false
	}
	return email, true
}

func getWaffoCurrency() string {
	if setting.WaffoCurrency != "" {
		return setting.WaffoCurrency
	}
	return "USD"
}

func buildWaffoTopUpGoodsInfo(amount int64) *order.GoodsInfo {
	appName := strings.TrimSpace(common.SystemName)
	if appName == "" {
		appName = "New API"
	}
	return &order.GoodsInfo{
		GoodsName: fmt.Sprintf("Recharge %d credits", amount),
		AppName:   appName,
	}
}

// zeroDecimalCurrencies 零小数位币种，金额不能带小数点
var zeroDecimalCurrencies = map[string]bool{
	"IDR": true, "JPY": true, "KRW": true, "VND": true,
}

func formatWaffoAmount(amount float64, currency string) string {
	if zeroDecimalCurrencies[currency] {
		return fmt.Sprintf("%.0f", amount)
	}
	return fmt.Sprintf("%.2f", amount)
}

// getWaffoPayMoney converts the user-facing amount to USD for Waffo payment.
// Waffo only accepts USD, so this function handles the conversion from different
// display types (USD/CNY/TOKENS) to the actual USD amount to charge.
func waffoPurchaseUnits(amount int64) (int64, bool) {
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

func getWaffoPayMoney(requestAmount int64, units int64, group string) float64 {
	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(requestAmount)]; ok && ds > 0 {
		discount = ds
	}
	return float64(units) * setting.WaffoUnitPrice * topupGroupRatio * discount
}

type WaffoPayRequest struct {
	Amount         int64  `json:"amount"`
	PayMethodIndex *int   `json:"pay_method_index"` // 服务端支付方式列表的索引，nil 表示由 Waffo 自动选择
	PayMethodType  string `json:"pay_method_type"`  // Deprecated: 兼容旧前端，优先使用 pay_method_index
	PayMethodName  string `json:"pay_method_name"`  // Deprecated: 兼容旧前端，优先使用 pay_method_index
}

func RequestWaffoAmount(c *gin.Context) {
	var req WaffoPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}

	waffoMinTopup := int64(setting.WaffoMinTopUp)
	if req.Amount < waffoMinTopup {
		paymentError(c, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": waffoMinTopup}))
		return
	}
	units, ok := waffoPurchaseUnits(req.Amount)
	if !ok {
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

	payMoney := getWaffoPayMoney(req.Amount, units, group)
	if payMoney <= 0.01 {
		paymentError(c, common.NewMessage("Payment amount is too low"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

// RequestWaffoPay 创建 Waffo 支付订单
func RequestWaffoPay(c *gin.Context) {
	if !setting.WaffoEnabled {
		paymentError(c, common.NewMessage("Waffo payment is not enabled"))
		return
	}

	var req WaffoPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	waffoMinTopup := int64(setting.WaffoMinTopUp)
	if req.Amount < waffoMinTopup {
		paymentError(c, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": waffoMinTopup}))
		return
	}
	units, ok := waffoPurchaseUnits(req.Amount)
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
	userEmail, emailOK := getWaffoUserEmail(user)
	if !emailOK {
		paymentError(c, common.NewMessage("Email is empty!"))
		return
	}

	// 从服务端配置查找支付方式，客户端只传索引或旧字段
	var resolvedPayMethodType, resolvedPayMethodName string
	methods := setting.GetWaffoPayMethods()
	if req.PayMethodIndex != nil {
		// 新协议：按索引查找
		idx := *req.PayMethodIndex
		if idx < 0 || idx >= len(methods) {
			logger.LogWarn(c.Request.Context(), common.LogText("Waffo invalid payment method index user_id=%d pay_method_index=%d method_count=%d", id, idx, len(methods)))
			paymentError(c, common.NewMessage("Unsupported payment method"))
			return
		}
		resolvedPayMethodType = methods[idx].PayMethodType
		resolvedPayMethodName = methods[idx].PayMethodName
	} else if req.PayMethodType != "" {
		// 兼容旧前端：验证客户端传的值在服务端列表中
		valid := false
		for _, m := range methods {
			if m.PayMethodType == req.PayMethodType && m.PayMethodName == req.PayMethodName {
				valid = true
				resolvedPayMethodType = m.PayMethodType
				resolvedPayMethodName = m.PayMethodName
				break
			}
		}
		if !valid {
			logger.LogWarn(c.Request.Context(), common.LogText("Waffo invalid payment method user_id=%d pay_method_type=%s pay_method_name=%q", id, req.PayMethodType, req.PayMethodName))
			paymentError(c, common.NewMessage("Unsupported payment method"))
			return
		}
	}
	// resolvedPayMethodType/Name 为空时，Waffo 自动选择支付方式

	group, _ := model.GetUserGroup(id, true)
	payMoney := getWaffoPayMoney(req.Amount, units, group)
	if payMoney < 0.01 {
		paymentError(c, common.NewMessage("Payment amount is too low"))
		return
	}

	// 生成唯一订单号，paymentRequestId 与 merchantOrderId 保持一致，简化追踪
	merchantOrderId := fmt.Sprintf("WAFFO-%d-%d-%s", id, time.Now().UnixMilli(), randstr.String(6))
	paymentRequestId := merchantOrderId

	// Amount 存购买单位。代币展示下必须是额度单位的整数倍，入账再乘 QuotaPerUnit，不再把小数截成 1。
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          units,
		Money:           payMoney,
		TradeNo:         merchantOrderId,
		PaymentMethod:   model.PaymentMethodWaffo,
		PaymentProvider: model.PaymentProviderWaffo,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo failed to create top-up order user_id=%d trade_no=%s amount=%d error=%q", id, merchantOrderId, req.Amount, err.Error()))
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}

	sdk, err := getWaffoSDK()
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo SDK initialization failed user_id=%d trade_no=%s error=%q", id, merchantOrderId, err.Error()))
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		paymentError(c, common.NewMessage("Payment configuration error"))
		return
	}

	callbackAddr := service.GetCallbackAddress()
	notifyUrl := callbackAddr + "/api/waffo/webhook"
	if setting.WaffoNotifyUrl != "" {
		notifyUrl = setting.WaffoNotifyUrl
	}
	returnUrl := paymentReturnPath("/wallet?show_history=true")
	if setting.WaffoReturnUrl != "" {
		returnUrl = setting.WaffoReturnUrl
	}

	currency := getWaffoCurrency()
	goodsInfo := buildWaffoTopUpGoodsInfo(req.Amount)
	createParams := &order.CreateOrderParams{
		PaymentRequestID: paymentRequestId,
		MerchantOrderID:  merchantOrderId,
		OrderAmount:      formatWaffoAmount(payMoney, currency),
		OrderCurrency:    currency,
		OrderDescription: goodsInfo.GoodsName,
		OrderRequestedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		NotifyURL:        notifyUrl,
		MerchantInfo: &order.MerchantInfo{
			MerchantID: setting.WaffoMerchantId,
		},
		UserInfo: &order.UserInfo{
			UserID:       strconv.Itoa(user.Id),
			UserEmail:    userEmail,
			UserTerminal: "WEB",
		},
		PaymentInfo: &order.PaymentInfo{
			ProductName:   "ONE_TIME_PAYMENT",
			PayMethodType: resolvedPayMethodType,
			PayMethodName: resolvedPayMethodName,
		},
		GoodsInfo:          goodsInfo,
		SuccessRedirectURL: returnUrl,
		FailedRedirectURL:  returnUrl,
	}
	resp, err := waffoCreateOrder(c.Request.Context(), sdk, createParams)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo failed to create order user_id=%d trade_no=%s error=%q", id, merchantOrderId, err.Error()))
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}
	if !resp.IsSuccess() {
		logger.LogWarn(c.Request.Context(), common.LogText("Waffo order creation rejected user_id=%d trade_no=%s code=%s message=%q", id, merchantOrderId, resp.Code, resp.Message))
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}

	orderData := resp.GetData()
	logger.LogInfo(c.Request.Context(), common.LogText("Waffo top-up order created user_id=%d trade_no=%s amount=%d money=%.2f pay_method_type=%s pay_method_name=%q", id, merchantOrderId, req.Amount, payMoney, resolvedPayMethodType, resolvedPayMethodName))

	paymentUrl := orderData.FetchRedirectURL()
	if paymentUrl == "" {
		paymentUrl = orderData.OrderAction
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url": paymentUrl,
			"order_id":    merchantOrderId,
		},
	})
}

// webhookPayloadWithSubInfo 是 PAYMENT_NOTIFICATION 回调的请求体（订阅字段已删，类型名沿用上游）
type webhookPayloadWithSubInfo struct {
	EventType string `json:"eventType"`
	Result    struct {
		core.PaymentNotificationResult
	} `json:"result"`
}

// WaffoWebhook 处理 Waffo 回调通知（支付/退款）
func WaffoWebhook(c *gin.Context) {
	if !isWaffoWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), common.LogText("Waffo webhook rejected reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo webhook failed to read request body path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	sdk, err := getWaffoSDK()
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo webhook SDK initialization failed path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	wh := sdk.Webhook()
	bodyStr := string(bodyBytes)
	signature := c.GetHeader("X-SIGNATURE")
	logger.LogInfo(c.Request.Context(), common.LogText("Waffo webhook received request path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))

	// 验证请求签名
	if !wh.VerifySignature(bodyStr, signature) {
		logger.LogWarn(c.Request.Context(), common.LogText("Waffo webhook signature verification failed path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	var event core.WebhookEvent
	if err := common.Unmarshal(bodyBytes, &event); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo webhook failed to parse path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		sendWaffoWebhookResponse(c, wh, false, "invalid payload")
		return
	}

	switch event.EventType {
	case core.EventPayment:
		var payload webhookPayloadWithSubInfo
		if err := common.Unmarshal(bodyBytes, &payload); err != nil {
			logger.LogError(c.Request.Context(), common.LogText("Waffo failed to parse payment callback payload event_type=%s client_ip=%s error=%q", event.EventType, c.ClientIP(), err.Error()))
			sendWaffoWebhookResponse(c, wh, false, "invalid payment payload")
			return
		}
		logger.LogInfo(c.Request.Context(), common.LogText("Waffo webhook verified and parsed event_type=%s merchant_order_id=%s order_status=%s client_ip=%s", event.EventType, payload.Result.MerchantOrderID, payload.Result.OrderStatus, c.ClientIP()))
		handleWaffoPayment(c, wh, &payload.Result.PaymentNotificationResult)
	case core.EventRefund:
		var notification core.RefundNotification
		if err := common.Unmarshal(bodyBytes, &notification); err != nil {
			logger.LogError(c.Request.Context(), common.LogText("Waffo failed to parse refund callback payload client_ip=%s error=%q", c.ClientIP(), err.Error()))
			sendWaffoWebhookResponse(c, wh, false, "invalid refund payload")
			return
		}
		handleWaffoRefund(c, wh, notification.Result, bodyStr)
	default:
		logger.LogInfo(c.Request.Context(), common.LogText("Waffo webhook ignored event event_type=%s client_ip=%s", event.EventType, c.ClientIP()))
		sendWaffoWebhookResponse(c, wh, true, "")
	}
}

// handleWaffoPayment 处理支付完成通知
func handleWaffoPayment(c *gin.Context, wh *core.WebhookHandler, result *core.PaymentNotificationResult) {
	if result.OrderStatus != core.OrderStatusPaySuccess {
		logger.LogInfo(c.Request.Context(), common.LogText("Waffo order not successful, top-up skipped trade_no=%s order_status=%s client_ip=%s", result.MerchantOrderID, result.OrderStatus, c.ClientIP()))
		// 只有关单是终态。支付中、待授权还可能变成成功，不能先标失败并让对方停止重试。
		applyWaffoNonSuccessStatus(c, result.MerchantOrderID, result.OrderStatus)
		sendWaffoWebhookResponse(c, wh, true, "")
		return
	}

	merchantOrderId := result.MerchantOrderID

	LockOrder(merchantOrderId)
	defer UnlockOrder(merchantOrderId)

	if err := model.RechargeWaffo(merchantOrderId, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo top-up processing failed trade_no=%s client_ip=%s error=%q", merchantOrderId, c.ClientIP(), err.Error()))
		sendWaffoWebhookResponse(c, wh, false, err.Error())
		return
	}

	logger.LogInfo(c.Request.Context(), common.LogText("Waffo top-up succeeded trade_no=%s client_ip=%s", merchantOrderId, c.ClientIP()))
	sendWaffoWebhookResponse(c, wh, true, "")
}

func handleWaffoRefund(c *gin.Context, wh *core.WebhookHandler, result *core.RefundNotificationResult, payload string) {
	if result == nil {
		sendWaffoWebhookResponse(c, wh, false, "invalid refund payload")
		return
	}
	switch result.RefundStatus {
	case core.RefundStatusInProgress, core.RefundStatusFailed:
		logger.LogInfo(c.Request.Context(), common.LogText("Waffo refund not completed, quota unchanged trade_no=%s refund_status=%s client_ip=%s", result.OrigPaymentRequestID, result.RefundStatus, c.ClientIP()))
		sendWaffoWebhookResponse(c, wh, true, "")
		return
	case core.RefundStatusFullyRefunded, core.RefundStatusPartiallyRefunded:
	default:
		logger.LogError(c.Request.Context(), common.LogText("Waffo refund status is not recognized trade_no=%s refund_status=%s client_ip=%s", result.OrigPaymentRequestID, result.RefundStatus, c.ClientIP()))
		sendWaffoWebhookResponse(c, wh, false, "unknown refund status")
		return
	}

	tradeNo := strings.TrimSpace(result.OrigPaymentRequestID)
	refundID := strings.TrimSpace(result.RefundRequestID)
	if refundID == "" {
		refundID = strings.TrimSpace(result.AcquiringRefundOrderID)
	}
	if tradeNo == "" || refundID == "" {
		recordAdtyPaymentReview(model.PaymentProviderWaffo, tradeNo, refundID, payload, errors.New("waffo refund is missing the original order or refund id"))
		sendWaffoWebhookResponse(c, wh, true, "")
		return
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	err := model.RefundWaffo(tradeNo, waffoRefundIdempotencyKey(refundID), result.RefundStatus == core.RefundStatusFullyRefunded, result.RefundAmount)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo refund failed trade_no=%s refund_request_id=%s client_ip=%s error=%q", tradeNo, refundID, c.ClientIP(), err.Error()))
		if waffoRefundPermanent(err) {
			recordAdtyPaymentReview(model.PaymentProviderWaffo, tradeNo, refundID, payload, err)
			sendWaffoWebhookResponse(c, wh, true, "")
			return
		}
		sendWaffoWebhookResponse(c, wh, false, err.Error())
		return
	}
	logger.LogInfo(c.Request.Context(), common.LogText("Waffo refund applied trade_no=%s refund_request_id=%s refund_status=%s client_ip=%s", tradeNo, refundID, result.RefundStatus, c.ClientIP()))
	sendWaffoWebhookResponse(c, wh, true, "")
}

func waffoRefundPermanent(err error) bool {
	return errors.Is(err, model.ErrTopUpNotFound) ||
		errors.Is(err, model.ErrPaymentMethodMismatch) ||
		errors.Is(err, model.ErrInvalidTopUpQuota) ||
		errors.Is(err, model.ErrPaymentAmountMismatch) ||
		errors.Is(err, model.ErrAdtyIdempotencyConflict)
}

func waffoRefundIdempotencyKey(refundID string) string {
	key := "waffo-refund:" + strings.TrimSpace(refundID)
	if len(key) <= 64 {
		return key
	}
	return "wr:" + common.Sha1([]byte(refundID))
}

func applyWaffoNonSuccessStatus(c *gin.Context, merchantOrderID string, orderStatus string) {
	if orderStatus != core.OrderStatusOrderClose {
		return
	}
	markWaffoOrderFailed(c, merchantOrderID)
}

func markWaffoOrderFailed(c *gin.Context, merchantOrderID string) {
	if merchantOrderID == "" {
		return
	}
	if err := model.UpdatePendingTopUpStatus(merchantOrderID, model.PaymentProviderWaffo, common.TopUpStatusFailed); err != nil &&
		!errors.Is(err, model.ErrTopUpNotFound) &&
		!errors.Is(err, model.ErrTopUpStatusInvalid) {
		logger.LogError(c.Request.Context(), common.LogText("Waffo failed to mark order as failed trade_no=%s error=%q", merchantOrderID, err.Error()))
	}
}

// sendWaffoWebhookResponse 发送签名响应
func sendWaffoWebhookResponse(c *gin.Context, wh *core.WebhookHandler, success bool, msg string) {
	var body, sig string
	if success {
		body, sig = wh.BuildSuccessResponse()
	} else {
		body, sig = wh.BuildFailedResponse(msg)
	}
	c.Header("X-SIGNATURE", sig)
	c.Data(http.StatusOK, "application/json", []byte(body))
}
