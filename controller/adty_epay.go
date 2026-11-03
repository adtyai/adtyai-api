package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
)

// 易支付多通道下单和回调（任务 5.2）。

// 单个通道和一次下单的时限在后台设置（adty_epay_setting，任务 3.6），最多试 5 个通道。
// 测试可以用 SetAdtyEpayTimeoutsForTest 改成毫秒级，0 表示读设置。
var (
	adtyEpayChannelTimeoutOverride time.Duration
	adtyEpayTotalDeadlineOverride  time.Duration
	adtyEpayMaxAttempts            = 5
)

// adtyEpayTimeouts 返回单通道时限和总时限。
func adtyEpayTimeouts() (perChannel, total time.Duration) {
	setting := adty_setting.GetEpaySetting()
	perChannel = time.Duration(max(setting.ChannelTimeoutSeconds, 1)) * time.Second
	total = time.Duration(max(setting.TotalTimeoutSeconds, 1)) * time.Second
	if adtyEpayChannelTimeoutOverride > 0 {
		perChannel = adtyEpayChannelTimeoutOverride
	}
	if adtyEpayTotalDeadlineOverride > 0 {
		total = adtyEpayTotalDeadlineOverride
	}
	return perChannel, total
}

type adtyEpayPayRequest struct {
	Amount            int64  `json:"amount"`
	Method            string `json:"method"`
	Device            string `json:"device"`
	ExcludeChannelIds []int  `json:"exclude_channel_ids"`
	PromoCode         string `json:"promo_code"` // 任务 12.4
}

// RequestAdtyEpayPay 挑通道下单，API 下单失败、超时或返回不对就换下一个通道。
func RequestAdtyEpayPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req adtyEpayPayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || !service.IsAdtyEpayMethod(req.Method) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if req.Device != "mobile" {
		req.Device = "pc"
	}
	quote, err := service.AdtyQuoteForSite(service.AdtyBillingSiteForUser(c.GetInt("id")), c.GetString("group"), req.Amount, req.Method, adtyPromoInput(c, req.PromoCode))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.ValidateTopUpQuotaCapacity(c.GetInt("id"), int(quote.CreditQuota)); err != nil {
		common.ApiError(c, err)
		return
	}
	minutes := adty_setting.GetTopupSetting().EpayOrderMinutes
	if minutes <= 0 {
		minutes = 10
	}
	exclude := map[int]bool{}
	for _, id := range req.ExcludeChannelIds {
		exclude[id] = true
	}
	channelTimeout, totalTimeout := adtyEpayTimeouts()
	deadline := time.Now().Add(totalTimeout)
	callback := strings.TrimRight(service.GetCallbackAddress(), "/")
	for attempt := 0; attempt < adtyEpayMaxAttempts && time.Now().Before(deadline); attempt++ {
		channel, method, err := service.PickAdtyEpayChannel(req.Method, quote.PayAmount, req.Device, exclude, time.Now().Unix())
		if err != nil {
			break
		}
		exclude[channel.Id] = true
		topUp, _, err := createAdtyOrder(c, adtyOrderDraft{
			prefix:    "EPY",
			provider:  model.PaymentProviderAdtyEpay,
			method:    req.Method,
			quote:     quote,
			expireAt:  time.Now().Unix() + int64(minutes*60),
			channelID: channel.Id,
		})
		if err != nil {
			writeAdtyTopUpError(c, err)
			return
		}
		timeout := min(channelTimeout, time.Until(deadline))
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		result, err := service.CreateAdtyEpayOrder(ctx, channel, method, service.AdtyEpayCreateRequest{
			TradeNo:   topUp.TradeNo,
			Name:      "Top-up " + topUp.TradeNo,
			ClientIP:  c.ClientIP(),
			Device:    req.Device,
			MoneyCNY:  quote.PayAmount,
			NotifyURL: callback + "/api/user/epay/notify/" + strconv.Itoa(channel.Id),
			ReturnURL: service.AdtyPayResultURL(topUp.TradeNo), // 主站的支付结果页，按订单所属站点跳回（任务 11.3）
		})
		cancel()
		if err != nil {
			common.SysLog(common.LogText("epay channel %d order failed trade_no=%s: %s", channel.Id, topUp.TradeNo, err.Error()))
			_ = model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderAdtyEpay, common.TopUpStatusFailed)
			continue
		}
		if result.Mode != "submit" && result.Mode != method.ReturnType {
			_ = model.UpdateAdtyEpayMethodReturnType(method.Id, result.Mode)
		}
		if err := model.UpdateAdtyTopupOrderFields(topUp.TradeNo, map[string]any{
			"qr_code":          result.QRCode,
			"pay_url":          result.PayURL,
			"url_scheme":       result.URLScheme,
			"gateway_trade_id": result.GatewayTradeNo,
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
		return
	}
	common.ApiErrorT(c, "This payment method is not available")
}

// adtyEpayCallbackParams 读回调参数：GET 读 query，POST 读表单。
func adtyEpayCallbackParams(c *gin.Context) map[string]string {
	params := map[string]string{}
	if c.Request.Method == http.MethodPost {
		if err := c.Request.ParseForm(); err == nil {
			for key, values := range c.Request.PostForm {
				if len(values) > 0 {
					params[key] = values[0]
				}
			}
		}
		return params
	}
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	return params
}

// AdtyEpayNotify 处理 /api/user/epay/notify/:channelId：用这个通道自己的密钥验签，核对 pid、订单的通道和方式，
// 再入账。永久性错误进人工审核后也回 success，否则网关会一直重发；临时错误回 fail。
func AdtyEpayNotify(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("channelId"))
	if err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	channel, err := model.GetAdtyEpayChannel(channelID)
	if err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	params := adtyEpayCallbackParams(c)
	if !service.EpayVerify(params, channel.MerchantKey) || params["pid"] != channel.MerchantId {
		common.SysLog(common.LogText("epay notify rejected channel=%d: bad signature or pid", channelID))
		c.String(http.StatusOK, "fail")
		return
	}
	if params["trade_status"] != "TRADE_SUCCESS" {
		c.String(http.StatusOK, "success")
		return
	}
	tradeNo := params["out_trade_no"]
	if strings.HasPrefix(tradeNo, "TEST") {
		common.SysLog(common.LogText("epay test order callback channel=%d trade_no=%s", channelID, tradeNo))
		c.String(http.StatusOK, "success")
		return
	}
	payload := common.GetJsonString(params)
	err = completeAdtyEpayCallback(channel, tradeNo, params, payload, adtyCallerIP(c))
	if err == nil {
		c.String(http.StatusOK, "success")
		return
	}
	if bepusdtPermanentError(err) {
		recordAdtyPaymentReview(model.PaymentProviderAdtyEpay, tradeNo, params["trade_no"], payload, err)
		c.String(http.StatusOK, "success")
		return
	}
	common.SysError(common.LogText("epay notify temporary failure trade_no=%s: %s", tradeNo, err.Error()))
	c.String(http.StatusOK, "fail")
}

func completeAdtyEpayCallback(channel *model.AdtyEpayChannel, tradeNo string, params map[string]string, payload, callerIP string) error {
	ext, err := model.GetAdtyTopupOrder(tradeNo)
	if err != nil {
		return err
	}
	if ext == nil {
		return model.ErrTopUpNotFound
	}
	if ext.ChannelId != channel.Id {
		return errors.Join(model.ErrPaymentMethodMismatch, errors.New("order belongs to another channel"))
	}
	method := channel.MethodByTypeCode(params["type"])
	if method == nil || method.Method != ext.Method {
		return errors.Join(model.ErrPaymentMethodMismatch, errors.New("payment type does not match the order"))
	}
	money, err := service.AdtyDecimalFromAny(params["money"])
	if err != nil {
		return model.ErrPaymentAmountMismatch
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	_, err = model.CompleteAdtyTopUp(tradeNo, model.AdtyPaymentFacts{
		Provider:       model.PaymentProviderAdtyEpay,
		Method:         ext.Method,
		PaidAmount:     money,
		PaidCurrency:   "CNY",
		GatewayTradeId: params["trade_no"],
		TxHash:         params["trade_no"],
		Payload:        payload,
		Source:         model.AdtyPaymentSourceCallback,
	}, callerIP)
	return err
}

// SetAdtyEpayTimeoutsForTest 缩短换通道的超时，结束时还原。只给其他包的测试用。
func SetAdtyEpayTimeoutsForTest(t interface{ Cleanup(func()) }, perChannel, total time.Duration) {
	oldChannel, oldTotal := adtyEpayChannelTimeoutOverride, adtyEpayTotalDeadlineOverride
	adtyEpayChannelTimeoutOverride, adtyEpayTotalDeadlineOverride = perChannel, total
	t.Cleanup(func() { adtyEpayChannelTimeoutOverride, adtyEpayTotalDeadlineOverride = oldChannel, oldTotal })
}
