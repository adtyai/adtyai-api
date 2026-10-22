package controller

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type adtyTopupOrderReq struct {
	Amount        int64  `json:"amount"`         // 美元档位
	PaymentMethod string `json:"payment_method"` // alipay|wxpay|usdt
	UsdtChain     string `json:"usdt_chain"`     // trc20|bep20|polygon|xlayer，仅 usdt
}

// startEpayPurchaseForOrder is replaced in tests. Production uses StartEpayPurchaseForOrder.
var startEpayPurchaseForOrder = StartEpayPurchaseForOrder

// markCreatedTopupOrderFailed closes a checkout that never got a usable cashier snapshot.
func markCreatedTopupOrderFailed(order *model.TopupOrder) {
	if order == nil {
		return
	}
	_ = model.DB.Model(order).Update("status", model.TopupOrderStatusFailed).Error
	order.Status = model.TopupOrderStatusFailed
}

// GetTopupTiers GET /api/topup/tiers
func GetTopupTiers(c *gin.Context) {
	percents := model.GetTopupBonusTierPercents()
	tiers := make([]gin.H, 0, len(model.FixedTopupAmountsUSD))
	for i, amt := range model.FixedTopupAmountsUSD {
		rate := percents[i] / 100.0
		// 审查 P2-9：金额保留 2 位，消除浮点尾巴（0.44999999999999996 → 0.45）
		bonus := math.Round(float64(amt)*rate*100) / 100
		tiers = append(tiers, gin.H{
			"amount":        amt,
			"bonus_rate":    rate,
			"bonus_percent": percents[i],
			"bonus_amount":  bonus,
		})
	}
	epayOK, epayReason := model.HasUsablePrimaryEpay()
	if epayReason == "" {
		epayReason = "支付维护中"
	}
	usdtOK, usdtReason := service.IsAdtyUsdtAvailable()
	if usdtReason == "" {
		usdtReason = "支付维护中"
	}
	payMethods := []gin.H{
		{"type": "alipay", "name": "支付宝", "available": epayOK, "maintenance_reason": ternaryStr(epayOK, "", epayReason)},
		{"type": "wxpay", "name": "微信", "available": epayOK, "maintenance_reason": ternaryStr(epayOK, "", epayReason)},
		{"type": "usdt", "name": "USDT", "available": usdtOK, "maintenance_reason": ternaryStr(usdtOK, "", usdtReason)},
	}
	fxSnap := model.CurrentTopupFxSnapshot()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"tiers":                  tiers,
			"first_topup_bonus_rate": model.GetFirstTopupBonusRate(),
			"pay_methods":            payMethods,
			"usdt_chains":            setting.EnabledUsdtChains(),
			"fx_rate":                fxSnap.EffectiveRate,
			"fx_base_rate":           fxSnap.BaseRate,
			"fx_float":               fxSnap.FloatApplied,
			"fx_source":              fxSnap.Source,
		},
	})
}

func ternaryStr(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// GetFirstRechargeInfo GET /api/topup/first-recharge
// 总裁令：首充赠送已停；rate=0、eligible=false，供前端过渡清双标签/横幅。
func GetFirstRechargeInfo(c *gin.Context) {
	_ = c.GetInt("id")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"first_topup_bonus_rate": 0.0,
			"eligible":               false,
		},
	})
}

// CreateTopupOrder POST /api/topup/order — 固定档位；支付宝/微信走主用易支付拉起。
func CreateTopupOrder(c *gin.Context) {
	var req adtyTopupOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	if !model.IsFixedTopupAmount(req.Amount) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "amount must be one of the fixed tiers",
			"code":    "invalid_tier_amount",
		})
		return
	}
	method := strings.ToLower(strings.TrimSpace(req.PaymentMethod))
	// 审查 P1-2：只接受真实可拉起的支付方式，杜绝无支付的僵尸 pending 单。
	if method != "alipay" && method != "wxpay" && method != "usdt" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "payment_method must be one of alipay, wxpay, usdt",
			"code":    "invalid_payment_method",
		})
		return
	}
	needEpay := method == "alipay" || method == "wxpay"
	if needEpay {
		ok, reason := model.HasUsablePrimaryEpay()
		if !ok {
			if reason == "" {
				reason = "支付维护中"
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": reason,
				"code":    "payment_maintenance",
			})
			return
		}
	}
	var usdtChain string
	if method == "usdt" {
		id, chainErr := setting.RequireEnabledUsdtChain(req.UsdtChain)
		if chainErr != nil {
			if errors.Is(chainErr, setting.ErrUsdtChainDisabled) {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "该网络未开通",
					"code":    "usdt_chain_disabled",
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "请选择 USDT 网络",
				"code":    "invalid_usdt_chain",
			})
			return
		}
		usdtChain = id
		ok, reason := service.IsAdtyUsdtAvailable()
		if !ok {
			if reason == "" {
				reason = "支付维护中"
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": reason,
				"code":    "payment_maintenance",
			})
			return
		}
	}

	userId := c.GetInt("id")
	deviceFp := c.GetHeader("X-Device-FP")
	order, err := model.CreateTopupOrder(userId, req.Amount, method, deviceFp, c.ClientIP())
	if err != nil {
		if errors.Is(err, model.ErrInvalidTopupAmount) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error(), "code": "invalid_tier_amount"})
			return
		}
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}

	now := time.Now().Unix()
	resp := gin.H{"order": userTopupOrderView(order, now)}
	if needEpay {
		payURL, params, inst, err := startEpayPurchaseForOrder(order, method)
		if err != nil {
			// 审查 P1-2：拉起失败置 failed，避免永久 pending（与 USDT 分支一致）
			markCreatedTopupOrderFailed(order)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "支付维护中",
				"code":    "payment_maintenance",
				"data":    gin.H{"order": userTopupOrderView(order, now)},
			})
			return
		}
		paramsJSON, mErr := common.Marshal(params)
		if mErr != nil || strings.TrimSpace(payURL) == "" {
			// 网关单可能已创建，但本地没有收银台快照，不能继续显示待支付。
			markCreatedTopupOrderFailed(order)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to bind payment snapshot",
				"code":    "epay_snapshot_failed",
				"data":    gin.H{"order": userTopupOrderView(order, now)},
			})
			return
		}
		// 回写实例、汇率和收银台；失败则不得返回 pay_url。继续支付只重放这份快照。
		fx := order.FxRate
		payCNY := order.PayCNY
		bonusRateSnapshot := 0.0
		if order.AmountUSD > 0 {
			bonusRateSnapshot = order.BonusAmountUSD / float64(order.AmountUSD)
		}
		if err := model.DB.Model(order).Updates(map[string]any{
			"epay_instance_id":    inst.Id,
			"epay_pay_url":        payURL,
			"epay_pay_params":     string(paramsJSON),
			"fx_rate":             fx,
			"pay_cny":             payCNY,
			"payment_provider":    "epay",
			"bonus_rate_snapshot": bonusRateSnapshot,
		}).Error; err != nil {
			markCreatedTopupOrderFailed(order)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to bind payment snapshot",
				"code":    "epay_snapshot_failed",
				"data":    gin.H{"order": userTopupOrderView(order, now)},
			})
			return
		}
		order.EpayInstanceId = inst.Id
		order.EpayPayURL = payURL
		order.EpayPayParams = string(paramsJSON)
		order.FxRate = fx
		order.PayCNY = payCNY
		order.PaymentProvider = "epay"
		resp["order"] = userTopupOrderView(order, now)
		resp["pay_url"] = payURL
		resp["pay_params"] = params
	}
	if method == "usdt" {
		if err := model.SetTopupOrderUsdtChain(order, usdtChain); err != nil {
			_ = model.DB.Model(order).Update("status", model.TopupOrderStatusFailed).Error
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to bind usdt chain",
				"code":    "usdt_chain_bind_failed",
			})
			return
		}
		cashier, err := StartUsdtPurchaseForOrder(order)
		if err != nil {
			_ = model.DB.Model(order).Update("status", model.TopupOrderStatusFailed).Error
			order.Status = model.TopupOrderStatusFailed
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "支付维护中",
				"code":    "payment_maintenance",
				"data":    gin.H{"order": userTopupOrderView(order, now)},
			})
			return
		}
		resp["order"] = userTopupOrderView(order, now)
		resp["usdt"] = cashier
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// GetTopupOrder GET /api/topup/order/:no
func GetTopupOrder(c *gin.Context) {
	orderNo := c.Param("no")
	order, err := model.GetTopupOrderByNo(orderNo)
	if err != nil {
		if errors.Is(err, model.ErrTopupOrderNotFound) {
			adtyFail(c, http.StatusNotFound, "not_found", "order not found", nil)
			return
		}
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	userId := c.GetInt("id")
	role := c.GetInt("role")
	if order.UserId != userId && role < common.RoleAdminUser {
		adtyFail(c, http.StatusForbidden, "forbidden", "forbidden", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": userTopupOrderView(order, time.Now().Unix())})
}

// AdminCompleteAdtyTopup POST /api/admin/orders/:no/complete — 最小闭环补单/验收入账（不再要求 confirm）。
type adminCompleteTopupReq struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteAdtyTopup POST /api/admin/orders/:no/complete（仅 root）。
// 审查 P1-1：必须带网关交易号 trade_no；占位/未拉起支付的订单 409；不能给自己补单；流水记 operator_id。
func AdminCompleteAdtyTopup(c *gin.Context) {
	var req adminCompleteTopupReq
	_ = c.ShouldBindJSON(&req)
	adminManualCompleteTopup(c, c.Param("no"), req.TradeNo, "admin_order_complete")
}
