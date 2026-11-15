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

// MergeMainMethodBonus 校验一条主站规则，并把它合并进 current。
//
// current 必须是同一个事务里加行锁从 options 表读到的值，不能是本进程的内存
// 快照：这里写回去的是整张表，基准一旦陈旧，另一节点刚保存的规则就会被静默
// 抹掉（ISS-R4-0099）。它只算不写，内存仍由配置层在事务提交之后按落库值同步。
func MergeMainMethodBonus(current string, method string, enabled bool, rate string) (string, error) {
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return "", err
	}
	parsed, err := parseBonusRate(rate, enabled)
	if err != nil {
		return "", err
	}
	var next map[string]MethodBonusRule
	if err := unmarshalBonusSetting(current, &next); err != nil {
		return "", err
	}
	if next == nil {
		next = map[string]MethodBonusRule{}
	}
	next[key] = MethodBonusRule{Enabled: enabled, Rate: parsed}
	return marshalBonusSetting(next)
}

// MergeSiteMethodAuth 校验一次站点授权变更，并把它合并进 current。理由同上：
// 基准取库里加锁读到的当前值，不取内存快照。
func MergeSiteMethodAuth(current string, siteID int, method string, allowed bool) (string, error) {
	if siteID <= 0 {
		return "", errBonusMethod("site is required")
	}
	key, err := canonicalBonusMethod(method)
	if err != nil {
		return "", err
	}
	var next map[int][]string
	if err := unmarshalBonusSetting(current, &next); err != nil {
		return "", err
	}
	if next == nil {
		next = map[int][]string{}
	}
	authorized := next[siteID]
	updated := make([]string, 0, len(authorized)+1)
	for _, item := range authorized {
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

// MergeSiteMethodBonus 校验一条站长规则（必须在 Root 授权的支付方式内），
// 并把它合并进 current。理由同上。
//
// 「Root 是否授权了这个支付方式」读的仍是配置层的内存：那是授权，不是被覆盖的
// 写基准，与 ISS-R4-0099 的陈旧快照丢写是两件事。
func MergeSiteMethodBonus(current string, siteID int, method string, enabled bool, rate string) (string, error) {
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
	var next map[int]map[string]MethodBonusRule
	if err := unmarshalBonusSetting(current, &next); err != nil {
		return "", err
	}
	if next == nil {
		next = map[int]map[string]MethodBonusRule{}
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

// unmarshalBonusSetting 解析 options 里读到的一串配置；空串表示这一行还没有。
func unmarshalBonusSetting(current string, value any) error {
	if strings.TrimSpace(current) == "" {
		return nil
	}
	return common.UnmarshalJsonStr(current, value)
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
