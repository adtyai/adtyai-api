package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// AdtyBonusMethod is the canonical payment-method key for gift rules.
// USDT chains share one rule. Amount-tier rates are not a bonus authority.
func AdtyBonusMethod(method string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	if strings.HasPrefix(method, "usdt") {
		return "usdt"
	}
	switch method {
	case AdtyMethodAlipay, AdtyMethodWxpay:
		return method
	default:
		return ""
	}
}

// AdtyPaymentMethodBonus returns the snapshot rate and USD gift for one order.
// Rate is a decimal fraction: 0.025 means 2.5% of the USD principal.
// Amount-tier tables and paid promo codes do not affect this result.
func AdtyPaymentMethodBonus(site *model.AdtySite, method string, faceUSD decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	key := AdtyBonusMethod(method)
	if key == "" || !faceUSD.IsPositive() {
		return decimal.Zero, decimal.Zero, nil
	}
	rule, ok := adtyMethodBonusRule(site, key)
	if !ok || !rule.Enabled {
		return decimal.Zero, decimal.Zero, nil
	}
	rate, err := decimal.NewFromString(strings.TrimSpace(rule.Rate))
	if err != nil || rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.Zero, decimal.Zero, err
	}
	return rate, faceUSD.Mul(rate).Round(8), nil
}

func adtyMethodBonusRule(site *model.AdtySite, key string) (adty_setting.MethodBonusRule, bool) {
	if site != nil {
		if !adty_setting.SiteMethodBonusAllowed(site.Id, key) {
			return adty_setting.MethodBonusRule{}, false
		}
		return adty_setting.SiteMethodBonus(site.Id, key)
	}
	return adty_setting.MainMethodBonus(key)
}

func SetAdtyMethodBonusForTest(t *testing.T, method string, enabled bool, rate string) {
	t.Helper()
	setting := adty_setting.GetTopupSetting()
	old := cloneMethodBonus(setting.MethodBonus)
	if setting.MethodBonus == nil {
		setting.MethodBonus = map[string]adty_setting.MethodBonusRule{}
	}
	setting.MethodBonus[AdtyBonusMethod(method)] = adty_setting.MethodBonusRule{Enabled: enabled, Rate: rate}
	t.Cleanup(func() { setting.MethodBonus = old })
}

func ClearAdtyMethodBonusForTest(t *testing.T) {
	t.Helper()
	setting := adty_setting.GetTopupSetting()
	old := cloneMethodBonus(setting.MethodBonus)
	setting.MethodBonus = map[string]adty_setting.MethodBonusRule{}
	t.Cleanup(func() { setting.MethodBonus = old })
}

// SiteBonusEarningCovers refuses a promised site gift unless earning cash covers it,
// or Root authorized margin fill and the open margin cycle can cover the remainder.
func SiteBonusEarningCovers(siteID int, bonus decimal.Decimal) error {
	if siteID <= 0 || !bonus.IsPositive() || model.DB == nil {
		return nil
	}
	var earning model.AdtySiteWallet
	if err := model.DB.Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletEarnings).Limit(1).Find(&earning).Error; err != nil {
		return err
	}
	if earning.Id != 0 && earning.Balance.GreaterThanOrEqual(bonus) {
		return nil
	}
	short := bonus
	if earning.Id != 0 {
		short = bonus.Sub(earning.Balance)
	}
	if !adty_setting.SiteBonusMarginFillAllowed(siteID) {
		return common.NewMessage("Site gift reserve is not covered by the earning wallet")
	}
	var margin model.AdtySiteWallet
	if err := model.DB.Where("site_id = ? AND kind = ?", siteID, model.AdtySiteWalletMargin).Limit(1).Find(&margin).Error; err != nil {
		return err
	}
	allowed := AdtyMarginNewDraw(AdtyMarginDrawInput{
		TierUSD: margin.CycleTierUSD, Balance: margin.Balance, Used: margin.CycleUsedUSD,
	})
	if margin.Id == 0 || short.GreaterThan(allowed) {
		return common.NewMessage("Site gift reserve is not covered by the earning wallet or authorized margin")
	}
	return nil
}

// ReserveAdtySiteGiftTx freezes a site gift from the earning wallet, then from an authorized margin cycle.
func ReserveAdtySiteGiftTx(tx *gorm.DB, ext *model.AdtyTopupOrder) error {
	if tx == nil || ext == nil || ext.SiteId <= 0 || !ext.BonusUSD.IsPositive() {
		return nil
	}
	if err := ensureAdtySiteWalletsTx(tx, ext.SiteId); err != nil {
		return err
	}
	var earning model.AdtySiteWallet
	if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", ext.SiteId, model.AdtySiteWalletEarnings).First(&earning).Error; err != nil {
		return err
	}
	fromEarning := earning.Balance
	if fromEarning.GreaterThan(ext.BonusUSD) {
		fromEarning = ext.BonusUSD
	}
	if fromEarning.IsNegative() {
		fromEarning = decimal.Zero
	}
	rest := ext.BonusUSD.Sub(fromEarning)
	fromMargin := decimal.Zero
	if rest.IsPositive() {
		if !adty_setting.SiteBonusMarginFillAllowed(ext.SiteId) {
			return common.NewMessage("Site gift reserve is not covered by the earning wallet")
		}
		var margin model.AdtySiteWallet
		if err := model.AdtyLockForUpdate(tx).Where("site_id = ? AND kind = ?", ext.SiteId, model.AdtySiteWalletMargin).First(&margin).Error; err != nil {
			return err
		}
		allowed := AdtyMarginNewDraw(AdtyMarginDrawInput{TierUSD: margin.CycleTierUSD, Balance: margin.Balance, Used: margin.CycleUsedUSD})
		if rest.GreaterThan(allowed) {
			return common.NewMessage("Site gift reserve is not covered by the earning wallet or authorized margin")
		}
		fromMargin = rest
	}
	key := "gift:" + ext.TradeNo
	if fromEarning.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletEarnings, fromEarning.Neg(), AdtySiteWalletChange{
			Type: "gift_reserve", RefType: "topup", RefID: ext.TradeNo, IdempotencyKey: key + ":earn",
		}); err != nil {
			return err
		}
	}
	if fromMargin.IsPositive() {
		if _, err := applyAdtySiteWalletChangeTx(tx, ext.SiteId, model.AdtySiteWalletMargin, fromMargin.Neg(), AdtySiteWalletChange{
			Type: "gift_reserve", RefType: "topup", RefID: ext.TradeNo, IdempotencyKey: key + ":margin",
		}); err != nil {
			return err
		}
	}
	ext.GiftReserveEarning = fromEarning
	ext.GiftReserveMargin = fromMargin
	switch {
	case fromEarning.IsPositive() && fromMargin.IsPositive():
		ext.GiftReserveSource = "earning+margin"
	case fromMargin.IsPositive():
		ext.GiftReserveSource = "margin"
	default:
		ext.GiftReserveSource = "earning"
	}
	return nil
}

func cloneMethodBonus(in map[string]adty_setting.MethodBonusRule) map[string]adty_setting.MethodBonusRule {
	if in == nil {
		return nil
	}
	out := make(map[string]adty_setting.MethodBonusRule, len(in))
	for key, rule := range in {
		out[key] = rule
	}
	return out
}
