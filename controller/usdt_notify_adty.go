package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

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
	now := time.Now().Unix()
	if err := model.EvaluateUsdtCallbackCredit(order, paidUSDT, now); err != nil {
		reason := err.Error()
		if errors.Is(err, model.ErrUsdtUnderpay) {
			reason = "underpay"
		} else if errors.Is(err, model.ErrUsdtOrderExpired) {
			reason = "timeout"
		}
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty usdt notify no auto-credit order=%s reason=%s paid=%.6f want=%.6f",
			orderNo, reason, paidUSDT, order.PayUSDT))
		_, _ = model.MarkTopupOrderNeedManual(orderNo, reason)
		// Acknowledge callback so gateway stops retrying; funds need manual补单.
		c.String(http.StatusOK, "ok")
		return
	}
	tradeID := service.AdtyBepusdtCallbackTradeID(params)
	if tradeID != "" {
		if err := model.AttachProviderTradeNo(orderNo, tradeID); err != nil {
			if errors.Is(err, model.ErrEpayTradeNoDuplicate) {
				logger.LogWarn(c.Request.Context(), common.LogText(
					"adty usdt notify trade_id duplicate %s", tradeID))
				c.String(http.StatusConflict, "fail")
				return
			}
			c.String(http.StatusInternalServerError, "fail")
			return
		}
	}
	if _, err := model.CompleteTopupOrder(orderNo); err != nil {
		logger.LogError(c.Request.Context(), common.LogText(
			"adty usdt notify complete failed order=%s err=%v", orderNo, err))
		c.String(http.StatusInternalServerError, "fail")
		return
	}
	c.String(http.StatusOK, "ok")
}

// AdtyUsdtNotify GET|POST /api/pay/usdt/notify — 只验签，不靠登录态。
func AdtyUsdtNotify(c *gin.Context) {
	handleAdtyUsdtNotify(c)
}

// AdtyBepusdtLegacyNotify GET|POST /api/user/bepusdt/notify — 默认关闭；开启时与新路径同一处理。
func AdtyBepusdtLegacyNotify(c *gin.Context) {
	if !model.IsBepusdtLegacyNotifyEnabled() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	handleAdtyUsdtNotify(c)
}
