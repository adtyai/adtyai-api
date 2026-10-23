package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func readAdtyBepusdtCallback(c *gin.Context) (map[string]interface{}, error) {
	params := make(map[string]interface{})
	ct := strings.ToLower(c.ContentType())
	if strings.Contains(ct, "application/json") || c.Request.Method == http.MethodPost {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			if err := json.Unmarshal(body, &params); err != nil {
				// fall through to form
				params = map[string]interface{}{}
			}
		}
	}
	_ = c.Request.ParseForm()
	for k, vs := range c.Request.Form {
		if len(vs) == 0 {
			continue
		}
		if _, exists := params[k]; !exists {
			params[k] = vs[0]
		}
	}
	for k, vs := range c.Request.URL.Query() {
		if len(vs) == 0 {
			continue
		}
		if _, exists := params[k]; !exists {
			params[k] = vs[0]
		}
	}
	if len(params) == 0 {
		return nil, errors.New("empty callback")
	}
	return params, nil
}

// handleAdtyUsdtNotify shared by new path and optional legacy alias.
func handleAdtyUsdtNotify(c *gin.Context) {
	params, err := readAdtyBepusdtCallback(c)
	if err != nil {
		logger.LogWarn(c.Request.Context(), common.LogText("adty usdt notify parse fail: %v", err))
		c.String(http.StatusBadRequest, "fail")
		return
	}
	if !service.AdtyVerifyBepusdtSignatureWithConfiguredToken(params) {
		logger.LogWarn(c.Request.Context(), common.LogText("adty usdt notify bad signature order=%s",
			service.AdtyBepusdtCallbackOrderNo(params)))
		c.String(http.StatusForbidden, "fail")
		return
	}
	orderNo := service.AdtyBepusdtCallbackOrderNo(params)
	if orderNo == "" {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	status := service.AdtyBepusdtCallbackStatus(params)
	if !service.AdtyIsBepusdtPaidStatus(status) {
		// Non-paid statuses are display-only. Do not turn a still-payable order into review.
		logger.LogInfo(c.Request.Context(), common.LogText(
			"adty usdt notify non-paid status kept for display only order=%s status=%s", orderNo, status))
		c.String(http.StatusOK, "ok")
		return
	}
	order, err := model.GetTopupOrderByNo(orderNo)
	if err != nil {
		logger.LogWarn(c.Request.Context(), common.LogText("adty usdt notify order not found %s", orderNo))
		c.String(http.StatusBadRequest, "fail")
		return
	}
	if order.PaymentProvider != "" && order.PaymentProvider != model.PaymentProviderBepusdt && order.PaymentProvider != "adtyai" {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty usdt notify provider mismatch order=%s provider=%s", orderNo, order.PaymentProvider))
		c.String(http.StatusForbidden, "fail")
		return
	}
	paidUSDT := service.AdtyBepusdtCallbackPaidUSDT(params)
	decision := model.DecideUsdtPaidCallback(order, paidUSDT)
	switch decision.Action {
	case model.UsdtPaidCallbackRetry:
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty usdt notify retry order=%s reason=%s", orderNo, decision.Reason))
		c.String(http.StatusBadRequest, "fail")
		return
	case model.UsdtPaidCallbackIdempotent:
		c.String(http.StatusOK, "ok")
		return
	case model.UsdtPaidCallbackManual:
		if order.Status == model.TopupOrderStatusNeedManual {
			c.String(http.StatusOK, "ok")
			return
		}
		marked, markErr := model.MarkTopupOrderNeedManual(orderNo, decision.Reason, paidUSDT)
		if markErr != nil || marked == nil {
			logger.LogError(c.Request.Context(), common.LogText(
				"adty usdt notify mark need_manual failed order=%s reason=%s err=%v", orderNo, decision.Reason, markErr))
			c.String(http.StatusInternalServerError, "fail")
			return
		}
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty usdt notify need_manual order=%s reason=%s amount=%v", orderNo, decision.Reason, paidUSDT))
		c.String(http.StatusOK, "ok")
		return
	case model.UsdtPaidCallbackCredit:
		tradeID := service.AdtyBepusdtCallbackTradeID(params)
		if tradeID != "" {
			if err := model.AttachProviderTradeNo(orderNo, tradeID); err != nil {
				if errors.Is(err, model.ErrEpayTradeNoDuplicate) {
					logger.LogWarn(c.Request.Context(), common.LogText(
						"adty usdt notify trade_id duplicate %s", tradeID))
					marked, markErr := model.MarkTopupOrderNeedManual(orderNo, "trade_duplicate", paidUSDT)
					if markErr != nil || marked == nil {
						logger.LogError(c.Request.Context(), common.LogText(
							"adty usdt notify mark need_manual failed order=%s reason=trade_duplicate err=%v", orderNo, markErr))
						c.String(http.StatusInternalServerError, "fail")
						return
					}
					c.String(http.StatusOK, "ok")
					return
				}
				c.String(http.StatusInternalServerError, "fail")
				return
			}
		}
		if order.PayUSDT > 0 && paidUSDT+1e-9 < order.PayUSDT {
			logger.LogWarn(c.Request.Context(), common.LogText(
				"adty usdt notify amount differs from cashier snapshot order=%s pay_usdt=%v actual_amount=%v",
				orderNo, order.PayUSDT, paidUSDT))
		}
		if _, err := model.CompleteTopupOrderFromUsdtCallback(orderNo); err != nil {
			logger.LogError(c.Request.Context(), common.LogText(
				"adty usdt notify complete failed order=%s err=%v", orderNo, err))
			c.String(http.StatusInternalServerError, "fail")
			return
		}
		c.String(http.StatusOK, "ok")
		return
	default:
		logger.LogError(c.Request.Context(), common.LogText(
			"adty usdt notify unknown decision order=%s action=%s", orderNo, decision.Action))
		c.String(http.StatusInternalServerError, "fail")
	}
}

// AdtyUsdtNotify GET|POST /api/pay/usdt/notify — 只验签，不靠登录态。
func AdtyUsdtNotify(c *gin.Context) {
	handleAdtyUsdtNotify(c)
}

// AdtyBepusdtLegacyNotify 不再入账。路由已删除，直接调用也固定 404。
func AdtyBepusdtLegacyNotify(c *gin.Context) {
	c.AbortWithStatus(http.StatusNotFound)
}
