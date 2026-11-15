package adty_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/shopspring/decimal"
)

// 这三个键必须与实际写入 options 表的键一致，否则重启后配置回退。
const (
	TopupSettingKeyMethodBonus     = "adty_topup_setting.method_bonus"
	TopupSettingKeySiteMethodAuth  = "adty_topup_setting.site_method_auth"
	TopupSettingKeySiteMethodBonus = "adty_topup_setting.site_method_bonus"
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

// BuildMainMethodBonus 校验一条主站规则，并返回写进
// adty_topup_setting.method_bonus 的完整 JSON。
//
// 它只算不写：内存里的开关必须由配置层在事务提交之后从落库值同步，
// 这样「写库失败」不会留下一个只在内存里生效、重启就消失的加赠比例。
func BuildMainMethodBonus(method string, enabled bool, rate string) (string, error) {
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return "", err
	}
	parsed, err := parseBonusRate(rate, enabled)
	if err != nil {
		return "", err
	}
	next := make(map[string]MethodBonusRule, len(topupSetting.MethodBonus)+1)
	for name, rule := range topupSetting.MethodBonus {
		next[name] = rule
	}
	next[key] = MethodBonusRule{Enabled: enabled, Rate: parsed}
	return marshalBonusSetting(next)
}

// BuildSiteMethodAuth 校验一次站点授权变更，并返回写进
// adty_topup_setting.site_method_auth 的完整 JSON。只算不写，理由同上。
func BuildSiteMethodAuth(siteID int, method string, allowed bool) (string, error) {
	if siteID <= 0 {
		return "", errBonusMethod("site is required")
	}
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return "", err
	}
	next := make(map[int][]string, len(topupSetting.SiteMethodAuth)+1)
	for id, methods := range topupSetting.SiteMethodAuth {
		next[id] = methods
	}
	current := next[siteID]
	updated := make([]string, 0, len(current)+1)
	for _, item := range current {
		if item != key {
			updated = append(updated, item)
		}
	}
	if allowed {
		updated = append(updated, key)
	}
	next[siteID] = updated
	return marshalBonusSetting(next)
}

// BuildSiteMethodBonus 校验一条站长规则（必须在 Root 授权的支付方式内），
// 并返回写进 adty_topup_setting.site_method_bonus 的完整 JSON。只算不写。
func BuildSiteMethodBonus(siteID int, method string, enabled bool, rate string) (string, error) {
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return "", err
	}
	if !SiteMethodBonusAllowed(siteID, key) {
		return "", errBonusMethod("payment method is not authorized for this site")
	}
	parsed, err := parseBonusRate(rate, enabled)
	if err != nil {
		return "", err
	}
	next := make(map[int]map[string]MethodBonusRule, len(topupSetting.SiteMethodBonus)+1)
	for id, rules := range topupSetting.SiteMethodBonus {
		copied := make(map[string]MethodBonusRule, len(rules)+1)
		for name, rule := range rules {
			copied[name] = rule
		}
		next[id] = copied
	}
	if next[siteID] == nil {
		next[siteID] = map[string]MethodBonusRule{}
	}
	next[siteID][key] = MethodBonusRule{Enabled: enabled, Rate: parsed}
	return marshalBonusSetting(next)
}

// ApplyMethodBonusOption 把一条已校验的加赠配置同步进内存。
//
// 生产路径是「先落库、提交后由配置层调用它」，测试用它预置内存配置，
// 于是不再存在「只改内存、不写库」的入口。
func ApplyMethodBonusOption(key, value string) error {
	field, ok := strings.CutPrefix(key, "adty_topup_setting.")
	if !ok || field == "" {
		return errBonusMethod("unknown bonus setting key")
	}
	return config.UpdateConfigFromMap(&topupSetting, map[string]string{field: value})
}

func marshalBonusSetting(value any) (string, error) {
	encoded, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
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
