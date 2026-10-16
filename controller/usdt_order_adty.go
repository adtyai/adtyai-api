package controller

import (
	"fmt"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

// StartUsdtPurchaseForOrder calls BEpusdt create-transaction and snapshots pay_usdt / fx / expire.
func StartUsdtPurchaseForOrder(order *model.TopupOrder) (cashier gin.H, err error) {
	callbackBase := service.GetCallbackAddress()
	if callbackBase == "" {
		callbackBase = "http://localhost"
	}
	notifyURL := fmt.Sprintf("%s/api/pay/usdt/notify", trimSlash(callbackBase))
	returnURL := trimSlash(callbackBase) + "/"
	if _, err := url.Parse(notifyURL); err != nil {
		return nil, err
	}
	resp, err := service.CreateAdtyBepusdtOrder(service.AdtyBepusdtCreateOrderRequest{
		OrderID:     order.OrderNo,
		Amount:      float64(order.AmountUSD),
		Currency:    setting.GetBepusdtCurrency(),
		TradeType:   setting.GetBepusdtTradeType(),
		NotifyURL:   notifyURL,
		RedirectURL: returnURL,
		Name:        fmt.Sprintf("ADTY%d", order.AmountUSD),
	})
	if err != nil {
		return nil, err
	}
	payUSDT, err := model.ParsePayUSDTString(resp.PaymentAmount)
	if err != nil || payUSDT <= 0 {
		return nil, fmt.Errorf("invalid bepusdt payment_amount %q", resp.PaymentAmount)
	}
	usdtFx := payUSDT / float64(order.AmountUSD)
	expireAt := resp.ExpirationTime
	if expireAt <= 0 {
		expireAt = model.DefaultUsdtExpireAt(time.Now())
	}
	if err := model.ApplyUsdtSnapshotToOrder(order, payUSDT, usdtFx, expireAt, resp.TransactionID); err != nil {
		return nil, err
	}
	cashier = gin.H{
		"payment_address":  resp.PaymentAddress,
		"payment_amount":   resp.PaymentAmount,
		"pay_usdt":         payUSDT,
		"payment_currency": firstNonEmpty(resp.PaymentCurrency, "USDT"),
		"payment_url":      resp.PaymentURL,
		"transaction_id":   resp.TransactionID,
		"expire_at":        expireAt,
		"usdt_fx_rate":     usdtFx,
		"timeout_minutes":  model.GetBepusdtOrderTimeoutMinutes(),
	}
	return cashier, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
