package adty_setting

import (
	"strings"

	"github.com/shopspring/decimal"
)

// MethodBonusRule is one site's gift switch and rate for a payment method.
// Rate is a decimal fraction in [0, 1], not a percent and not an amount tier.
type MethodBonusRule struct {
	Enabled bool   `json:"enabled"`
	Rate    string `json:"rate"`
}

// MainMethodBonus is the main-site rule. An absent rule is off.
func MainMethodBonus(method string) (MethodBonusRule, bool) {
	if topupSetting.MethodBonus == nil {
		return MethodBonusRule{}, false
	}
	rule, ok := topupSetting.MethodBonus[method]
	return rule, ok
}

// SiteMethodBonusAllowed reports whether Root authorized this method for a site.
func SiteMethodBonusAllowed(siteID int, method string) bool {
	if topupSetting.SiteMethodAuth == nil {
		return false
	}
	for _, item := range topupSetting.SiteMethodAuth[siteID] {
		if item == method {
			return true
		}
	}
	return false
}

// SetMainMethodBonus stores the main-site switch and decimal rate for one method.
// Rate is a fraction in [0, 1]. Amount tiers are not accepted here.
func SetMainMethodBonus(method string, enabled bool, rate string) error {
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return err
	}
	parsed, err := parseBonusRate(rate, enabled)
	if err != nil {
		return err
	}
	if topupSetting.MethodBonus == nil {
		topupSetting.MethodBonus = map[string]MethodBonusRule{}
	}
	topupSetting.MethodBonus[key] = MethodBonusRule{Enabled: enabled, Rate: parsed}
	return nil
}

// AuthorizeSiteBonusMethod grants or withdraws a site's right to configure one method.
func AuthorizeSiteBonusMethod(siteID int, method string, allowed bool) error {
	if siteID <= 0 {
		return errBonusMethod("site is required")
	}
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return err
	}
	if topupSetting.SiteMethodAuth == nil {
		topupSetting.SiteMethodAuth = map[int][]string{}
	}
	current := topupSetting.SiteMethodAuth[siteID]
	next := make([]string, 0, len(current)+1)
	for _, item := range current {
		if item != key {
			next = append(next, item)
		}
	}
	if allowed {
		next = append(next, key)
	}
	topupSetting.SiteMethodAuth[siteID] = next
	return nil
}

// SetSiteMethodBonus saves a site rule only inside a Root-authorized method.
func SetSiteMethodBonus(siteID int, method string, enabled bool, rate string) error {
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return err
	}
	if !SiteMethodBonusAllowed(siteID, key) {
		return errBonusMethod("payment method is not authorized for this site")
	}
	parsed, err := parseBonusRate(rate, enabled)
	if err != nil {
		return err
	}
	if topupSetting.SiteMethodBonus == nil {
		topupSetting.SiteMethodBonus = map[int]map[string]MethodBonusRule{}
	}
	if topupSetting.SiteMethodBonus[siteID] == nil {
		topupSetting.SiteMethodBonus[siteID] = map[string]MethodBonusRule{}
	}
	topupSetting.SiteMethodBonus[siteID][key] = MethodBonusRule{Enabled: enabled, Rate: parsed}
	return nil
}

// AuthorizeSiteBonusMarginFill lets an authorized site cover a gift shortfall from margin.
func AuthorizeSiteBonusMarginFill(siteID int, allowed bool) {
	if topupSetting.SiteBonusMarginFill == nil {
		topupSetting.SiteBonusMarginFill = map[int]bool{}
	}
	topupSetting.SiteBonusMarginFill[siteID] = allowed
}

// SiteBonusMarginFillAllowed reports the Root authorization for margin fill.
func SiteBonusMarginFillAllowed(siteID int) bool {
	return topupSetting.SiteBonusMarginFill != nil && topupSetting.SiteBonusMarginFill[siteID]
}

func canonicalBonusMethod(method string) (string, error) {
	method = strings.ToLower(strings.TrimSpace(method))
	if strings.HasPrefix(method, "usdt") {
		return "usdt", nil
	}
	if method == "alipay" || method == "wxpay" {
		return method, nil
	}
	return "", errBonusMethod("unsupported payment method")
}

func parseBonusRate(rate string, enabled bool) (string, error) {
	if !enabled && strings.TrimSpace(rate) == "" {
		return "0", nil
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(rate))
	if err != nil || parsed.IsNegative() || parsed.GreaterThan(decimal.NewFromInt(1)) {
		return "", errBonusMethod("bonus rate must be a decimal fraction from 0 to 1")
	}
	return parsed.String(), nil
}

func errBonusMethod(text string) error {
	return &bonusMethodError{text}
}

type bonusMethodError struct{ text string }

func (e *bonusMethodError) Error() string { return e.text }

// SiteMethodBonus is the site-owner rule inside an authorized method.
func SiteMethodBonus(siteID int, method string) (MethodBonusRule, bool) {
	if topupSetting.SiteMethodBonus == nil {
		return MethodBonusRule{}, false
	}
	rules := topupSetting.SiteMethodBonus[siteID]
	rule, ok := rules[method]
	return rule, ok
}
