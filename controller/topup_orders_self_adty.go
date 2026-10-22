package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

// ListSelfTopupOrders GET /api/topup/orders — owner only, no device or provider internals.
func ListSelfTopupOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	rows, total, err := model.ListTopupOrdersByUser(c.GetInt("id"), page, pageSize)
	if err != nil {
		adtyFail(c, http.StatusInternalServerError, "list_failed", "failed to list orders", nil)
		return
	}
	now := time.Now().Unix()
	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		items = append(items, userTopupOrderView(&rows[i], now))
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// ContinueTopupOrder POST /api/topup/order/:no/continue
// USDT reopens the stored cashier. Expired USDT is not recreated.
func ContinueTopupOrder(c *gin.Context) {
	order, ok := loadOwnTopupOrder(c)
	if !ok {
		return
	}
	now := time.Now().Unix()
	if model.TopupOrderDisplayStatus(order, now) == model.TopupDisplayExpired {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "订单已过期，请重新充值",
			"code":    "order_expired",
		})
		return
	}
	if !model.TopupOrderCanContinue(order, now) {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "这笔订单不能继续支付",
			"code":    "order_not_payable",
		})
		return
	}
	method := strings.ToLower(strings.TrimSpace(order.PaymentMethod))
	if method == "usdt" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"order": userTopupOrderView(order, now),
				"usdt":  storedUsdtCashier(order),
			},
		})
		return
	}
	payURL, params, err := replayStoredEpayCheckout(order)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "这笔订单不能继续支付",
			"code":    "order_not_payable",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"order":      userTopupOrderView(order, now),
			"pay_url":    payURL,
			"pay_params": params,
		},
	})
}

// replayStoredEpayCheckout returns the cashier saved at create time. It never opens a second gateway order.
func replayStoredEpayCheckout(order *model.TopupOrder) (string, map[string]string, error) {
	if order == nil || strings.TrimSpace(order.EpayPayURL) == "" || order.PayCNY <= 0 || order.EpayInstanceId <= 0 {
		return "", nil, errors.New("stored epay checkout missing")
	}
	params := map[string]string{}
	if raw := strings.TrimSpace(order.EpayPayParams); raw != "" {
		if err := common.UnmarshalJsonStr(raw, &params); err != nil {
			return "", nil, err
		}
	}
	return strings.TrimSpace(order.EpayPayURL), params, nil
}

func loadOwnTopupOrder(c *gin.Context) (*model.TopupOrder, bool) {
	order, err := model.GetTopupOrderByNo(c.Param("no"))
	if err != nil {
		if errors.Is(err, model.ErrTopupOrderNotFound) {
			adtyFail(c, http.StatusNotFound, "not_found", "order not found", nil)
			return nil, false
		}
		adtyFail(c, http.StatusInternalServerError, "bad_request", "failed to load order", nil)
		return nil, false
	}
	if order.UserId != c.GetInt("id") && c.GetInt("role") < common.RoleAdminUser {
		adtyFail(c, http.StatusForbidden, "forbidden", "forbidden", nil)
		return nil, false
	}
	return order, true
}

func userTopupOrderView(order *model.TopupOrder, now int64) gin.H {
	if order == nil {
		return gin.H{}
	}
	chain, _ := setting.LookupUsdtChain(order.UsdtChain)
	nextAction, nextHint := model.TopupOrderNextAction(order, now)
	canContinue := model.TopupOrderCanContinue(order, now)
	view := gin.H{
		"order_no":            order.OrderNo,
		"user_id":             order.UserId,
		"amount_usd":          order.AmountUSD,
		"paid_amount_usd":     order.PaidAmountUSD,
		"credit_amount_usd":   order.CreditAmountUSD,
		"bonus_amount_usd":    order.BonusAmountUSD,
		"refunded_amount":     order.RefundedAmount,
		"payment_method":      order.PaymentMethod,
		"usdt_chain":          order.UsdtChain,
		"usdt_network":        chain.Network,
		"status":              order.Status,
		"display_status":      model.TopupOrderDisplayStatus(order, now),
		"can_continue":        canContinue,
		"next_action":         nextAction,
		"next_action_hint":    nextHint,
		"create_time":         order.CreateTime,
		"complete_time":       order.CompleteTime,
		"usdt_expire_at":      order.UsdtExpireAt,
		"usdt_payment_amount": order.UsdtPaymentAmount,
		"pay_usdt":            order.PayUSDT,
		"refund_status":       order.RefundStatus,
	}
	// 窗口内才能继续付的订单才把地址和支付页给本人。过期、已支付、失败、人工单不再返回，避免再向旧地址转账。
	if canContinue && strings.EqualFold(strings.TrimSpace(order.PaymentMethod), "usdt") {
		view["usdt_pay_address"] = order.UsdtPayAddress
		view["usdt_payment_url"] = order.UsdtPaymentURL
	}
	return view
}

func storedUsdtCashier(order *model.TopupOrder) gin.H {
	chain, _ := setting.LookupUsdtChain(order.UsdtChain)
	amount := order.UsdtPaymentAmount
	if amount == "" {
		amount = strconv.FormatFloat(order.PayUSDT, 'f', -1, 64)
	}
	return gin.H{
		"payment_address":  order.UsdtPayAddress,
		"payment_amount":   amount,
		"pay_usdt":         order.PayUSDT,
		"payment_currency": "USDT",
		"payment_url":      order.UsdtPaymentURL,
		"expire_at":        order.UsdtExpireAt,
		"usdt_fx_rate":     order.UsdtFxRate,
		"timeout_minutes":  model.GetBepusdtOrderTimeoutMinutes(),
		"chain":            order.UsdtChain,
		"trade_type":       chain.TradeType,
		"network":          chain.Network,
		"chain_label":      chain.Label,
	}
}
