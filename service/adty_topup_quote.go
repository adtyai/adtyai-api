package service

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

// 支付商和支付方式（约定 C）。
const (
	AdtyProviderBEpusdt = "bepusdt"
	AdtyProviderEpay    = "adty_epay"

	AdtyMethodUSDTTRC20   = "usdt.trc20"
	AdtyMethodUSDTBEP20   = "usdt.bep20"
	AdtyMethodUSDTPolygon = "usdt.polygon"
	AdtyMethodUSDTXLayer  = "usdt.xlayer"
	AdtyMethodAlipay      = "alipay"
	AdtyMethodWxpay       = "wxpay"

	AdtyCurrencyUSDT = "USDT"
	AdtyCurrencyCNY  = "CNY"
)

// AdtyUSDTMethods 是支持的 USDT 链，值和 BEpusdt 的 trade_type 相同。
var AdtyUSDTMethods = []string{AdtyMethodUSDTTRC20, AdtyMethodUSDTBEP20, AdtyMethodUSDTPolygon, AdtyMethodUSDTXLayer}

// AdtyEpayMethods 是易支付的支付方式。
var AdtyEpayMethods = []string{AdtyMethodAlipay, AdtyMethodWxpay}

// IsAdtyUSDTMethod 说明这是不是一条 USDT 链。
func IsAdtyUSDTMethod(method string) bool {
	return slices.Contains(AdtyUSDTMethods, method)
}

// IsAdtyEpayMethod 说明这是不是易支付的方式。
func IsAdtyEpayMethod(method string) bool {
	return slices.Contains(AdtyEpayMethods, method)
}

// AdtyQuote 是一笔充值的报价：面额、加赠、入账额度和应付。
type AdtyQuote struct {
	FaceUSD         decimal.Decimal `json:"face_usd"`
	BonusRate       decimal.Decimal `json:"bonus_rate"` // 百分数，2.5 表示 2.5%
	BonusUSD        decimal.Decimal `json:"bonus_usd"`
	CreditQuota     int64           `json:"credit_quota"`
	PayAmount       decimal.Decimal `json:"pay_amount"`
	PayCurrency     string          `json:"pay_currency"`      // USDT | CNY
	PriceCNYPerUSD  decimal.Decimal `json:"price_cny_per_usd"` // 只有易支付用
	TopupGroupRatio decimal.Decimal `json:"topup_group_ratio"`
}

// AdtyBonusFor 取「不超过面额的最大档位」的加赠比例，低于最小档不加赠。加赠保留 8 位小数。
func AdtyBonusFor(face int64) (rate, bonus decimal.Decimal) {
	return adtyBonusFromRates(adty_setting.GetTopupSetting().BonusRates, face)
}

// adtyBonusFromRates 只给历史档位上限计算使用。新订单赠金不走这里。
func adtyBonusFromRates(rates map[int]float64, face int64) (rate, bonus decimal.Decimal) {
	best := int64(-1)
	for tier, tierRate := range rates {
		if int64(tier) <= face && int64(tier) > best && tierRate > 0 {
			best = int64(tier)
			rate = decimal.NewFromFloat(tierRate)
		}
	}
	if best < 0 {
		return decimal.Zero, decimal.Zero
	}
	bonus = decimal.NewFromInt(face).Mul(rate).Div(decimal.NewFromInt(100)).Round(8)
	return rate, bonus
}

// AdtyQuoteFor 给一个分组的主站用户算一笔充值的报价。
func AdtyQuoteFor(userGroup string, face int64, method string) (*AdtyQuote, error) {
	return AdtyQuoteForSite(nil, userGroup, face, method)
}

// AdtyQuoteForSite 算一笔充值的报价；site 是客户所属的分站（主站传 nil），分站客户按站点的加赠档位算（11.5）。
// TOKENS 显示模式会改变金额含义，这种模式下不报价。
func AdtyQuoteForSite(site *model.AdtySite, userGroup string, face int64, method string) (*AdtyQuote, error) {
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return nil, common.NewMessage("Top-up is unavailable while quota is displayed as tokens")
	}
	method = strings.TrimSpace(method)
	if !IsAdtyUSDTMethod(method) && !IsAdtyEpayMethod(method) {
		return nil, common.NewMessage("Unsupported payment method")
	}
	if face < int64(operation_setting.MinTopUp) || face <= 0 {
		return nil, common.NewMessage("Top-up amount cannot be less than {{amount}}", map[string]any{"amount": operation_setting.MinTopUp})
	}
	quote, err := adtyQuotaQuoteForSite(site, userGroup, decimal.NewFromInt(face), method)
	if err != nil {
		return nil, err
	}
	if IsAdtyUSDTMethod(method) {
		quote.PayCurrency = AdtyCurrencyUSDT
		quote.PayAmount = quote.FaceUSD.Mul(quote.TopupGroupRatio).Round(8)
		return quote, nil
	}
	price := decimal.NewFromFloat(operation_setting.Price)
	if !price.IsPositive() {
		return nil, common.NewMessage("The top-up price is not configured")
	}
	quote.PayCurrency = AdtyCurrencyCNY
	quote.PriceCNYPerUSD = price
	quote.PayAmount = quote.FaceUSD.Mul(price).Mul(quote.TopupGroupRatio).Round(2)
	return quote, nil
}

func adtyQuotaQuoteForSite(site *model.AdtySite, userGroup string, faceUSD decimal.Decimal, method string) (*AdtyQuote, error) {
	rate, bonus, err := AdtyPaymentMethodBonus(site, method, faceUSD)
	if err != nil {
		return nil, err
	}
	if site != nil && bonus.IsPositive() {
		// 策略上限（features.md「站长分站·加赠和试用额度」）：站点加赠
		// 不得超过 面额 ×（主站同档比例 + max_site_bonus_extra）÷ 100。
		// 这和下面的 SiteBonusEarningCovers 不是一回事：那个查站长的钱够不够垫这笔加赠，
		// 这个查规则允许站长给多少。预算够不等于允许给。
		if !adtySiteBonusWithinCap(faceUSD, bonus) {
			return nil, common.NewMessage("This site's bonus exceeds the cap of {{cap}} USD for a {{face}} USD top-up", map[string]any{
				"cap":  adtySiteBonusCapAmount(faceUSD).String(),
				"face": faceUSD.String(),
			})
		}
		if err := SiteBonusEarningCovers(site.Id, bonus); err != nil {
			return nil, err
		}
	}
	creditQuota := faceUSD.Add(bonus).Mul(common.QuotaPerUnitDecimal()).Floor()
	if _, err := common.WalletQuotaFromDecimalStrict(creditQuota); err != nil || !creditQuota.IsPositive() {
		return nil, common.NewMessage("Top-up quota exceeds the range the system can represent")
	}
	groupRatio := decimal.NewFromFloat(common.GetTopupGroupRatio(userGroup))
	if !groupRatio.IsPositive() {
		groupRatio = decimal.NewFromInt(1)
	}
	return &AdtyQuote{
		FaceUSD:         faceUSD,
		BonusRate:       rate,
		BonusUSD:        bonus,
		CreditQuota:     creditQuota.IntPart(),
		TopupGroupRatio: groupRatio,
	}, nil
}
