package controller

import (
	"fmt"
	"net/url"
	"strings"
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
	tradeType, err := usdtTradeTypeForOrder(order)
	if err != nil {
		return nil, err
	}
	resp, err := service.CreateAdtyBepusdtOrder(service.AdtyBepusdtCreateOrderRequest{
		OrderID:     order.OrderNo,
		Amount:      float64(order.AmountUSD),
		Currency:    setting.GetBepusdtCurrency(),
		TradeType:   tradeType,
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
	expireAt := model.UsdtExpireAtFromBepusdtRemaining(resp.ExpirationTime, time.Now())
	if err := model.ApplyUsdtSnapshotToOrder(order, payUSDT, usdtFx, expireAt, resp.TransactionID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.PaymentAddress) == "" {
		return nil, fmt.Errorf("bepusdt cashier missing address")
	}
	if err := model.SaveUsdtCashierSnapshot(order, resp.PaymentAddress, resp.PaymentURL, resp.PaymentAmount); err != nil {
		return nil, err
	}
	chain, _ := setting.LookupUsdtChain(order.UsdtChain)
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
		"chain":            order.UsdtChain,
		"trade_type":       tradeType,
		"network":          chain.Network,
		"chain_label":      chain.Label,
	}
	return cashier, nil
}

func usdtTradeTypeForOrder(order *model.TopupOrder) (string, error) {
	chain := strings.TrimSpace(order.UsdtChain)
	if chain == "" {
		// 旧单没有链字段时才用全局兜底，新单必须先写入 usdt_chain。
		return setting.GetBepusdtTradeType(), nil
	}
	tradeType := setting.TradeTypeForUsdtChain(chain)
	if tradeType == "" {
		return "", fmt.Errorf("unknown usdt chain %q", chain)
	}
	return tradeType, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
