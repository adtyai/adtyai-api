package controller

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 从旧项目改写（任务 4.2）：按 USD 下单，站内支付窗显示地址和数量，回调以 POST JSON 为准。

type bepusdtPayRequest struct {
	PaymentScene string `json:"payment_scene"`
	Amount       int64  `json:"amount"`
	Chain        string `json:"chain"`
	PromoCode    string `json:"promo_code"` // 任务 12.4
}

// RequestBEpusdtPay 下单：报价、建单、调网关，把地址、数量、到期时间写回订单返回给前端。
func RequestBEpusdtPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req bepusdtPayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if !setting.BEpusdtConfigured() || !setting.BEpusdtChainEnabled(req.Chain) || !service.IsAdtyUSDTMethod(req.Chain) {
		common.ApiErrorT(c, "This payment method is not available")
		return
	}
	quote, err := service.AdtyQuoteForSite(service.AdtyBillingSiteForUser(c.GetInt("id")), c.GetString("group"), req.Amount, req.Chain, adtyPromoInput(c, req.PromoCode))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.ValidateTopUpQuotaCapacity(c.GetInt("id"), int(quote.CreditQuota)); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := service.AuthorizeAdtyPaymentScene(c.GetInt("id"), req.PaymentScene, model.PaymentProviderAdtyBEpusdt, req.Chain, quote); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := replaceOtherPendingAdtyOrders(c); err != nil {
		if errors.Is(err, service.ErrAdtyPaymentDetected) {
			common.ApiErrorT(c, "A transfer has been detected. Please wait")
			return
		}
		writeAdtyTopUpError(c, err)
		return
	}
	minutes := adty_setting.GetTopupSetting().USDTOrderMinutes
	if minutes <= 0 {
		minutes = 15
	}
	topUp, ext, err := createAdtyOrder(c, adtyOrderDraft{
		paymentScene: req.PaymentScene,
		prefix:       "EPU",
		provider:     model.PaymentProviderAdtyBEpusdt,
		method:       req.Chain,
		quote:        quote,
		expireAt:     time.Now().Unix() + int64(minutes*60),
	})
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	callback := strings.TrimRight(service.GetCallbackAddress(), "/")
	order, err := service.CreateBEpusdtOrder(service.BEpusdtCreateRequest{
		OrderID:     topUp.TradeNo,
		AmountUSD:   quote.PayAmount,
		TradeType:   req.Chain,
		NotifyURL:   callback + "/api/user/bepusdt/notify",
		RedirectURL: service.AdtyPayResultURL(topUp.TradeNo),
		Name:        common.SystemName,
		Timeout:     minutes * 60,
	})
	if err != nil {
		common.SysLog(common.LogText("bepusdt create order failed trade_no=%s: %s", topUp.TradeNo, err.Error()))
		_ = model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderAdtyBEpusdt, common.TopUpStatusFailed)
		common.ApiErrorT(c, "The payment gateway is unavailable. Please try again later")
		return
	}
	money, _ := order.ActualAmount.Float64()
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(map[string]any{
			"pay_address":      order.Address,
			"pay_amount":       order.ActualAmount,
			"expire_at":        order.ExpireAt,
			"gateway_trade_id": order.TradeId,
			"pay_url":          order.PaymentURL,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.TopUp{}).Where("id = ?", topUp.Id).Update("money", money).Error
	}); err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	view, err := model.GetUserAdtyTopUp(topUp.TradeNo, topUp.UserId)
	if err != nil {
		writeAdtyTopUpError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

// bepusdtPermanentError 是不会因为重试而好转的错误：进人工审核并回 ok，免得网关一直重发。
func bepusdtPermanentError(err error) bool {
	return errors.Is(err, model.ErrPaymentAmountMismatch) ||
		errors.Is(err, model.ErrTopUpNotFound) ||
		errors.Is(err, model.ErrPaymentMethodMismatch) ||
		errors.Is(err, model.ErrTopUpStatusInvalid) ||
		errors.Is(err, model.ErrPaymentOrphanNotCredit) ||
		errors.Is(err, model.ErrTopUpQuotaLimitExceeded) ||
		errors.Is(err, model.ErrInvalidTopUpQuota)
}

// BEpusdtTopUpNotify 处理 BEpusdt 的回调：验签、只认 status=2、入账。只看 HTTP 200 和 ok。
func BEpusdtTopUpNotify(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	values := map[string]any{}
	if err := common.Unmarshal(raw, &values); err != nil || !service.VerifyBEpusdtSignature(values) {
		common.SysLog(common.LogText("bepusdt notify rejected: bad signature or body"))
		c.String(http.StatusBadRequest, "fail")
		return
	}
	tradeNo := model.PaymentOrphanPayloadString(values, "order_id")
	status := model.PaymentOrphanPayloadString(values, "status")
	eventType := "payment.other"
	switch status {
	case "1":
		eventType = "payment.pending"
	case "2":
		eventType = "payment.succeeded"
	case "3":
		eventType = "payment.expired"
	}
	if err := model.RecordAdtyVerifiedCallback(tradeNo, model.PaymentProviderAdtyBEpusdt, eventType); err != nil {
		c.String(http.StatusInternalServerError, "fail")
		return
	}
	if status != "2" {
		// 待支付期间每分钟推一次 status=1，超时推 status=3，都不入账。
		c.String(http.StatusOK, "ok")
		return
	}
	paid, errPaid := service.AdtyDecimalFromAny(values["actual_amount"])
	fiat, errFiat := service.AdtyDecimalFromAny(values["amount"])
	facts := model.AdtyPaymentFacts{
		Provider:       model.PaymentProviderAdtyBEpusdt,
		PaidAmount:     paid,
		FiatAmount:     fiat,
		PaidCurrency:   "USDT",
		PayAddress:     model.PaymentOrphanPayloadString(values, "token"),
		GatewayTradeId: model.PaymentOrphanPayloadString(values, "trade_id"),
		TxHash:         model.PaymentOrphanPayloadString(values, "block_transaction_id"),
		Payload:        string(raw),
		Source:         model.AdtyPaymentSourceCallback,
	}
	if errPaid != nil || errFiat != nil {
		err = model.ErrPaymentAmountMismatch
	} else {
		LockOrder(tradeNo)
		_, err = model.CompleteAdtyTopUp(tradeNo, facts, adtyCallerIP(c))
		UnlockOrder(tradeNo)
	}
	if err == nil || errors.Is(err, model.ErrAdtyDuplicatePayment) {
		c.String(http.StatusOK, "ok")
		return
	}
	if bepusdtPermanentError(err) {
		recordAdtyPaymentReview(model.PaymentProviderAdtyBEpusdt, tradeNo, facts.GatewayTradeId, string(raw), err)
		c.String(http.StatusOK, "ok")
		return
	}
	common.SysError(common.LogText("bepusdt notify temporary failure trade_no=%s: %s", tradeNo, err.Error()))
	c.String(http.StatusInternalServerError, "fail")
}

// recordAdtyPaymentReview 把入账失败的回调记进人工审核（充值对账页处理）。
func recordAdtyPaymentReview(provider, tradeNo, sessionID, payload string, cause error) {
	if err := model.RecordPaymentOrphanEvent(&model.PaymentOrphanEvent{
		Provider:    provider,
		EventType:   "payment_callback",
		ReferenceID: tradeNo,
		SessionID:   sessionID,
		Reason:      "payment requires manual review after payment succeeded",
		Error:       cause.Error(),
		Payload:     payload,
	}); err != nil {
		common.SysError(common.LogText("record payment review failed trade_no=%s: %s", tradeNo, err.Error()))
	}
	common.SysLog(common.LogText("payment queued for review provider=%s trade_no=%s: %s", provider, tradeNo, cause.Error()))
}
