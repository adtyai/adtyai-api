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
	PromoCodeId     int             `json:"promo_code_id"`   // 用了优惠码时是码的 id（任务 12.4）
	PromoBonusUSD   decimal.Decimal `json:"promo_bonus_usd"` // 优惠加赠，已含在 CreditQuota 里
	PromoFunder     string          `json:"-"`               // 优惠码出资方，下单时快照进扩展行
}

// AdtyBonusFor 取「不超过面额的最大档位」的加赠比例，低于最小档不加赠。加赠保留 8 位小数。
func AdtyBonusFor(face int64) (rate, bonus decimal.Decimal) {
	return adtyBonusFromRates(adty_setting.GetTopupSetting().BonusRates, face)
}

// AdtyTopupDisplayRates 是钱包页要展示的加赠档位。分站客户用站点档位，和报价一致；主站用后台档位。
func AdtyTopupDisplayRates(site *model.AdtySite) map[int]float64 {
	if site != nil {
		return AdtySiteBonusRates(site)
	}
	return adty_setting.GetTopupSetting().BonusRates
}

// AdtyBonusFromRates 按一组档位算加赠比例和金额。
func AdtyBonusFromRates(rates map[int]float64, face int64) (rate, bonus decimal.Decimal) {
	return adtyBonusFromRates(rates, face)
}

// adtyBonusFromRates 按一组档位算加赠：主站用 adty_topup_setting.bonus_rates，分站客户用站点的 BonusRates（P11）。
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
func AdtyQuoteFor(userGroup string, face int64, method string, promo AdtyPromoInput) (*AdtyQuote, error) {
	return AdtyQuoteForSite(nil, userGroup, face, method, promo)
}

// AdtyQuoteForSite 算一笔充值的报价；site 是客户所属的分站（主站传 nil），分站客户按站点的加赠档位算（11.5）。
// promo 里带了码时校验并算优惠加赠（任务 12.4），优惠只多送额度，应付不变。
// TOKENS 显示模式会改变金额含义，未确认支付合规时也不报价（controller 先拦，这里兜底）。
func AdtyQuoteForSite(site *model.AdtySite, userGroup string, face int64, method string, promo AdtyPromoInput) (*AdtyQuote, error) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return nil, common.NewMessage("Payment features are locked until the root administrator confirms the compliance terms.")
	}
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
	rate, bonus := AdtyBonusFor(face)
	if site != nil {
		rate, bonus = adtyBonusFromRates(AdtySiteBonusRates(site), face)
	}
	faceUSD := decimal.NewFromInt(face)
	promoCode, promoBonus, err := adtyPromoBonusFor(promo, faceUSD)
	if err != nil {
		return nil, err
	}
	if promoCode != nil && promoCode.Funder == model.AdtyPromoFunderSiteOwner {
		// 站长出资的码只给这个站正在服务的客户；加赠合计不能超过「主站同档 + max_site_bonus_extra」（11.5）。
		if site == nil || site.Id != promoCode.SiteId {
			return nil, ErrAdtyPromoInvalid
		}
		if !adtySiteBonusWithinCap(face, bonus.Add(promoBonus)) {
			return nil, ErrAdtySitePromoOverCap
		}
	}
	creditQuota := faceUSD.Add(bonus).Add(promoBonus).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor()
	if _, err := common.WalletQuotaFromDecimalStrict(creditQuota); err != nil || !creditQuota.IsPositive() {
		return nil, common.NewMessage("Top-up quota exceeds the range the system can represent")
	}
	groupRatio := decimal.NewFromFloat(common.GetTopupGroupRatio(userGroup))
	if !groupRatio.IsPositive() {
		groupRatio = decimal.NewFromInt(1)
	}
	quote := &AdtyQuote{
		FaceUSD:         faceUSD,
		BonusRate:       rate,
		BonusUSD:        bonus,
		CreditQuota:     creditQuota.IntPart(),
		TopupGroupRatio: groupRatio,
		PromoBonusUSD:   promoBonus,
	}
	if promoCode != nil {
		quote.PromoCodeId, quote.PromoFunder = promoCode.Id, promoCode.Funder
	}
	if IsAdtyUSDTMethod(method) {
		quote.PayCurrency = AdtyCurrencyUSDT
		quote.PayAmount = faceUSD.Mul(groupRatio).Round(8)
		return quote, nil
	}
	price := decimal.NewFromFloat(operation_setting.Price)
	if !price.IsPositive() {
		return nil, common.NewMessage("The top-up price is not configured")
	}
	quote.PayCurrency = AdtyCurrencyCNY
	quote.PriceCNYPerUSD = price
	quote.PayAmount = faceUSD.Mul(price).Mul(groupRatio).Round(2)
	return quote, nil
}
