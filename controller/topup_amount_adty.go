package controller

import (
	"errors"
	"math"
	"strings"

	"github.com/shopspring/decimal"
)

// topupGatewayChargeCovers 判断网关实付是否不少于下单时快照的应收。
// 税额和多付可以过。少付、币种不一致、没有快照，这里都返回 false。
// 空快照不能当成 0 元，也不能拿含分组倍率的 Money 当应收。
// 空快照的在途回调是否入账，由 decideTopupPayCallback 决定，不要把本函数的 false 直接当成丢单。
func topupGatewayChargeCovers(expectedAmount, expectedCurrency, paidCurrency string, paidMajor decimal.Decimal) bool {
	expectedAmount = strings.TrimSpace(expectedAmount)
	expectedCurrency = strings.TrimSpace(expectedCurrency)
	if expectedAmount == "" || expectedCurrency == "" || paidMajor.IsNegative() {
		return false
	}
	expected, ok := parseMajorMoney(expectedAmount)
	if !ok || !expected.IsPositive() {
		return false
	}
	if !sameMoneyCurrency(expectedCurrency, paidCurrency) {
		return false
	}
	return !paidMajor.Round(2).LessThan(expected.Round(2))
}

type topupCallbackAction int

const (
	topupCallbackCredit topupCallbackAction = iota
	topupCallbackAckNoCredit
	topupCallbackRetry
)

var errTopupCallbackRetry = errors.New("topup callback amount unverified")

func topupPaySnapshotPresent(amount, currency string) bool {
	return strings.TrimSpace(amount) != "" && strings.TrimSpace(currency) != ""
}

// decideTopupPayCallback 决定已验签回调是否入账。
// 有应收快照时，实付必须盖住快照；少付是永久不符，回成功避免网关无限重试，但不入账。
// 没有快照的在途旧单不能拿含分组倍率的 Money 当应收，也不能回成功后丢单。
// 实收为正就按原入账函数入账；金额解析不了则让网关重试。
func decideTopupPayCallback(snapshotAmount, snapshotCurrency string, paidOK bool, paid decimal.Decimal, covers bool) topupCallbackAction {
	if !topupPaySnapshotPresent(snapshotAmount, snapshotCurrency) {
		if paidOK && paid.IsPositive() {
			return topupCallbackCredit
		}
		return topupCallbackRetry
	}
	if paidOK && covers {
		return topupCallbackCredit
	}
	return topupCallbackAckNoCredit
}

func stripeExpectedCharge(unitAmount, quantity int64, currency string) (string, string, error) {
	if unitAmount <= 0 || quantity <= 0 || strings.TrimSpace(currency) == "" {
		return "", "", errors.New("stripe price amount unavailable")
	}
	if unitAmount > math.MaxInt64/quantity {
		return "", "", errors.New("stripe price amount overflow")
	}
	major, ok := minorUnitsToMajor(unitAmount*quantity, currency)
	if !ok || !major.IsPositive() {
		return "", "", errors.New("stripe price currency unsupported")
	}
	formatted, err := formatExpectedMajor(major, currency)
	if err != nil {
		return "", "", err
	}
	return formatted, normalizeMoneyCurrency(currency), nil
}

func creemExpectedCharge(price float64, currency string) (string, string, error) {
	if price < 0.01 || strings.TrimSpace(currency) == "" {
		return "", "", errors.New("creem product price is invalid")
	}
	formatted, err := formatExpectedMajor(decimal.NewFromFloat(price), currency)
	if err != nil {
		return "", "", err
	}
	parsed, ok := parseMajorMoney(formatted)
	if !ok || !parsed.IsPositive() {
		return "", "", errors.New("creem product price is invalid")
	}
	return formatted, normalizeMoneyCurrency(currency), nil
}

func waffoExpectedCharge(payMoney float64, currency string) (string, string, error) {
	if strings.TrimSpace(currency) == "" {
		return "", "", errors.New("waffo currency is required")
	}
	if _, ok := currencyMinorExponent(currency); !ok {
		return "", "", errors.New("waffo currency unsupported")
	}
	formatted := formatWaffoAmount(payMoney, currency)
	parsed, ok := parseMajorMoney(formatted)
	if !ok || !parsed.IsPositive() {
		return "", "", errors.New("waffo amount invalid")
	}
	return formatted, normalizeMoneyCurrency(currency), nil
}

func pancakeExpectedCharge(payMoney float64) (string, string, error) {
	formatted := formatWaffoPancakeAmount(payMoney)
	parsed, ok := parseMajorMoney(formatted)
	if !ok || !parsed.IsPositive() {
		return "", "", errors.New("waffo pancake amount invalid")
	}
	return formatted, "USD", nil
}

// waffoCallbackCovers 核对商户订单金额。同币种时再核用户实付，避免只回显下单金额。
// 用户币种不同是汇率换算，不能拿本币实付和美元应收硬比。
func waffoCallbackCovers(expectedAmount, expectedCurrency, orderAmount, orderCurrency, finalDealAmount, userCurrency string) bool {
	orderPaid, ok := parseMajorMoney(orderAmount)
	if !ok || !topupGatewayChargeCovers(expectedAmount, expectedCurrency, orderCurrency, orderPaid) {
		return false
	}
	if strings.TrimSpace(finalDealAmount) == "" || !sameMoneyCurrency(userCurrency, orderCurrency) {
		return true
	}
	deal, ok := parseMajorMoney(finalDealAmount)
	return ok && topupGatewayChargeCovers(expectedAmount, expectedCurrency, userCurrency, deal)
}

func formatExpectedMajor(amount decimal.Decimal, currency string) (string, error) {
	exp, ok := currencyMinorExponent(currency)
	if !ok {
		return "", errors.New("currency unsupported")
	}
	return amount.StringFixed(exp), nil
}
