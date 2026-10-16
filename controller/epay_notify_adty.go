package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// BuildEpayClientForInstance 用该实例密钥构造客户端（禁止串用其它实例密钥）。
func BuildEpayClientForInstance(inst *model.EpayInstance) (*epay.Client, error) {
	if inst == nil {
		return nil, model.ErrEpayInstanceNotFound
	}
	secret, err := model.DecryptEpayInstanceSecret(inst)
	if err != nil {
		return nil, err
	}
	return epay.NewClient(&epay.Config{
		PartnerID: inst.PartnerId,
		Key:       secret,
	}, inst.PayAddress)
}

// StartEpayPurchaseForOrder 对已落库 pending 订单向主用实例拉起支付，返回 url+params。
func StartEpayPurchaseForOrder(order *model.TopupOrder, payMethod string) (payURL string, params map[string]string, inst *model.EpayInstance, err error) {
	inst, err = model.GetPrimaryEpayInstance()
	if err != nil {
		return "", nil, nil, model.ErrEpayPaymentUnavailable
	}
	ok, _ := model.IsEpayInstanceUsable(inst)
	if !ok {
		return "", nil, nil, model.ErrEpayPaymentUnavailable
	}
	client, err := BuildEpayClientForInstance(inst)
	if err != nil {
		_, _, _ = model.RecordEpayInstanceFailure(inst.Id, "client_init_failed")
		return "", nil, inst, err
	}
	callbackBase := inst.CallbackBase
	if callbackBase == "" {
		callbackBase = service.GetCallbackAddress()
	}
	notifyURL, err := url.Parse(fmt.Sprintf("%s/api/pay/epay/notify/%d", trimSlash(callbackBase), inst.Id))
	if err != nil {
		return "", nil, inst, err
	}
	returnURL, _ := url.Parse(callbackBase + "/")
	fx := model.CurrentTopupFxRate()
	payCNY := float64(order.AmountUSD) * fx
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           payMethod,
		ServiceTradeNo: order.OrderNo,
		Name:           fmt.Sprintf("ADTY%d", order.AmountUSD),
		Money:          strconv.FormatFloat(payCNY, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyURL,
		ReturnUrl:      returnURL,
	})
	if err != nil {
		_, _, _ = model.RecordEpayInstanceFailure(inst.Id, "purchase_failed")
		return "", nil, inst, err
	}
	_ = model.ResetEpayInstanceFailureStreak(inst.Id)
	return uri, params, inst, nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func collectEpayNotifyParams(c *gin.Context) map[string]string {
	params := make(map[string]string)
	_ = c.Request.ParseForm()
	for k, vs := range c.Request.Form {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	for k, vs := range c.Request.URL.Query() {
		if _, ok := params[k]; !ok && len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	return params
}

// AdtyEpayNotify GET|POST /api/pay/epay/notify/:instance_id — 只验该实例密钥，不靠登录态。
func AdtyEpayNotify(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("instance_id"), 10, 64)
	if err != nil || id <= 0 {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	inst, err := model.GetEpayInstanceById(id)
	if err != nil {
		logger.LogWarn(c.Request.Context(), common.LogText("adty epay notify unknown instance id=%d", id))
		c.String(http.StatusBadRequest, "fail")
		return
	}
	params := collectEpayNotifyParams(c)
	if len(params) == 0 {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	client, err := BuildEpayClientForInstance(inst)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("adty epay notify client err instance=%d: %v", id, err))
		c.String(http.StatusInternalServerError, "fail")
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || verifyInfo == nil || !verifyInfo.VerifyStatus {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty epay notify bad signature instance=%d err=%v", id, err))
		c.String(http.StatusForbidden, "fail")
		return
	}
	// 商户号校验
	if pid := params["pid"]; pid != "" && pid != inst.PartnerId {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty epay notify pid mismatch instance=%d got=%s want=%s", id, pid, inst.PartnerId))
		c.String(http.StatusForbidden, "fail")
		return
	}
	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		c.String(http.StatusOK, "success")
		return
	}
	orderNo := verifyInfo.ServiceTradeNo
	order, err := model.GetTopupOrderByNo(orderNo)
	if err != nil {
		logger.LogWarn(c.Request.Context(), common.LogText("adty epay notify order not found %s", orderNo))
		c.String(http.StatusBadRequest, "fail")
		return
	}
	// 必须已绑定实例且与回调实例一致（未绑定不得入账）
	if order.EpayInstanceId == 0 {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty epay notify order unbound instance order=%s", orderNo))
		c.String(http.StatusForbidden, "fail")
		return
	}
	if order.EpayInstanceId != inst.Id {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty epay notify instance mismatch order=%s order_inst=%d notify_inst=%d",
			orderNo, order.EpayInstanceId, inst.Id))
		c.String(http.StatusForbidden, "fail")
		return
	}
	// 金额校验：PayCNY 未快照不得跳过入账
	if order.PayCNY <= 0 {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty epay notify missing pay_cny snapshot order=%s", orderNo))
		c.String(http.StatusForbidden, "fail")
		return
	}
	// L4：按分（int64）比较，避免网关回 "10" / "10.0" 与快照 "10.00" 字符串不等而拒入账
	wantMoney := strconv.FormatFloat(order.PayCNY, 'f', 2, 64)
	if !epayMoneyEqual(verifyInfo.Money, order.PayCNY) {
		logger.LogWarn(c.Request.Context(), common.LogText(
			"adty epay notify money mismatch order=%s got=%s want=%s", orderNo, verifyInfo.Money, wantMoney))
		c.String(http.StatusForbidden, "fail")
		return
	}
	tradeNo := verifyInfo.TradeNo
	if tradeNo != "" {
		if err := model.AttachProviderTradeNo(orderNo, tradeNo); err != nil {
			if errors.Is(err, model.ErrEpayTradeNoDuplicate) {
				logger.LogWarn(c.Request.Context(), common.LogText(
					"adty epay notify trade_no duplicate %s", tradeNo))
				c.String(http.StatusConflict, "fail")
				return
			}
			c.String(http.StatusInternalServerError, "fail")
			return
		}
	}
	if _, err := model.CompleteTopupOrder(orderNo); err != nil {
		logger.LogError(c.Request.Context(), common.LogText(
			"adty epay notify complete failed order=%s err=%v", orderNo, err))
		c.String(http.StatusInternalServerError, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

type epayManualCompleteReq struct {
	OrderNo string `json:"order_no"`
	TradeNo string `json:"trade_no"`
	Confirm bool   `json:"confirm"`
}

// AdminEpayManualComplete POST /api/admin/epay/manual-complete
// 按易支付交易号查重；同交易号二次补单 409；须 confirm=true。
func AdminEpayManualComplete(c *gin.Context) {
	var req epayManualCompleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false, "message": "confirm=true required", "code": "confirm_required",
		})
		return
	}
	if req.OrderNo == "" || req.TradeNo == "" {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "order_no and trade_no required", nil)
		return
	}
	if existing, err := model.FindTopupOrderByProviderTradeNo(req.TradeNo); err == nil && existing != nil {
		if existing.OrderNo != req.OrderNo || existing.Status == model.TopupOrderStatusPaid {
			c.JSON(http.StatusConflict, gin.H{
				"success": false, "message": "trade_no already used", "code": "trade_no_duplicate",
			})
			return
		}
	} else if err != nil && !errors.Is(err, model.ErrTopupOrderNotFound) {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	if err := model.AttachProviderTradeNo(req.OrderNo, req.TradeNo); err != nil {
		if errors.Is(err, model.ErrEpayTradeNoDuplicate) {
			c.JSON(http.StatusConflict, gin.H{
				"success": false, "message": "trade_no already used", "code": "trade_no_duplicate",
			})
			return
		}
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	order, err := model.CompleteTopupOrder(req.OrderNo)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrTopupOrderNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		} else if errors.Is(err, model.ErrTopupOrderStatusInvalid) {
			status = http.StatusConflict
			code = "order_status_invalid"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

// epayParseMoneyCents 把金额字符串解析为分（四舍五入）。空串或非法返回 error。
func epayParseMoneyCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty money")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid money")
	}
	return int64(math.Round(f * 100)), nil
}

// epayMoneyEqual 回调金额与订单 PayCNY 是否数值相等（按分比较）。
func epayMoneyEqual(callbackMoney string, payCNY float64) bool {
	got, err := epayParseMoneyCents(callbackMoney)
	if err != nil {
		return false
	}
	want := int64(math.Round(payCNY * 100))
	return got == want
}
