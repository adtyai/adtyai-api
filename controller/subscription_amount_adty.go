package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
)

// subscriptionChargeCoversPlan 判断网关实付是否不少于套餐标价。
// 税额可以多付。少付、币种不一致、金额解析不了，都不放行。
// 标价为 0 的套餐没有少付空间。
func subscriptionChargeCoversPlan(orderMoney float64, planCurrency, paidCurrency string, paidMajor decimal.Decimal) bool {
	if orderMoney < 0 || paidMajor.IsNegative() {
		return false
	}
	if orderMoney == 0 {
		return true
	}
	if !sameMoneyCurrency(planCurrency, paidCurrency) {
		return false
	}
	expected := decimal.NewFromFloat(orderMoney).Round(2)
	return !paidMajor.Round(2).LessThan(expected)
}

func sameMoneyCurrency(planCurrency, paidCurrency string) bool {
	paid := normalizeMoneyCurrency(paidCurrency)
	if paid == "" {
		return false
	}
	plan := normalizeMoneyCurrency(planCurrency)
	if plan == "" {
		plan = "USD"
	}
	return plan == paid
}

func normalizeMoneyCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "RMB" {
		return "CNY"
	}
	return currency
}

// currencyMinorExponent 返回该币种的最小单位位数。不认识的币种不换算，避免把日元当成分。
func currencyMinorExponent(currency string) (int32, bool) {
	switch normalizeMoneyCurrency(currency) {
	case "BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW", "MGA", "PYG", "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF":
		return 0, true
	case "BHD", "JOD", "KWD", "OMR", "TND":
		return 3, true
	case "USD", "EUR", "CNY", "GBP", "AUD", "CAD", "HKD", "SGD", "TWD", "INR", "THB", "MYR", "PHP", "IDR", "NZD", "CHF", "SEK", "NOK", "DKK", "PLN", "CZK", "HUF", "BRL", "MXN", "ZAR", "AED", "SAR", "TRY", "RUB":
		return 2, true
	default:
		return 0, false
	}
}

func minorUnitsToMajor(minor int64, currency string) (decimal.Decimal, bool) {
	exp, ok := currencyMinorExponent(currency)
	if !ok || minor < 0 {
		return decimal.Zero, false
	}
	return decimal.NewFromInt(minor).Shift(-exp), true
}

func parseMajorMoney(raw string) (decimal.Decimal, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return decimal.Zero, false
	}
	amount, err := decimal.NewFromString(raw)
	if err != nil || amount.IsNegative() {
		return decimal.Zero, false
	}
	return amount, true
}

func stripeEventPaidMajor(event stripe.Event) (string, decimal.Decimal, bool) {
	currency := event.GetObjectValue("currency")
	raw := strings.TrimSpace(event.GetObjectValue("amount_total"))
	if raw == "" {
		return currency, decimal.Zero, false
	}
	minor, err := decimal.NewFromString(raw)
	if err != nil || minor.IsNegative() || !minor.Equal(minor.Truncate(0)) {
		return currency, decimal.Zero, false
	}
	major, ok := minorUnitsToMajor(minor.IntPart(), currency)
	return currency, major, ok
}

// subscriptionOrderChargeCovers 读取套餐币种后核对实付。套餐查不到是暂时失败，调用方应让网关重试。
func subscriptionOrderChargeCovers(order *model.SubscriptionOrder, paidCurrency string, paidMajor decimal.Decimal) (bool, error) {
	if order == nil {
		return false, model.ErrSubscriptionOrderNotFound
	}
	if order.Status == common.TopUpStatusSuccess {
		return true, nil
	}
	plan, err := model.GetSubscriptionPlanById(order.PlanId)
	if err != nil {
		return false, err
	}
	return subscriptionChargeCoversPlan(order.Money, plan.Currency, paidCurrency, paidMajor), nil
}
