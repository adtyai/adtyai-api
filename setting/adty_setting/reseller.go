package adty_setting

import (
	"math"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// ResellerSetting 是站长分站的设置（P11），键名 adty_reseller_setting.<json 标签>。
// 和钱有关的项没有内置值（约定 I）：nil 表示未设置，存成 "null"；没设之前不能打开 enabled。
// 密钥按约定 D 用 Token、Secret 结尾的键名，GET /api/option/ 会过滤掉。
// 这些键只能通过分站管理的设置接口改（controller/adty_site_admin.go），通用的 /api/option/ 拒绝写入，
// 否则绕过下面的校验就能在没设 d 的情况下打开分站。
type ResellerSetting struct {
	Enabled            bool     `json:"enabled"`
	RootDomains        []string `json:"root_domains"`
	ReservedSubdomains []string `json:"reserved_subdomains"`
	UnknownHostMode    string   `json:"unknown_host_mode"` // main | reject

	CostFactor         *float64           `json:"cost_factor"`     // 全局拿货系数 d，0 < d ≤ 1
	MinCostFactor      *float64           `json:"min_cost_factor"` // d 的下限
	CostFactorTiers    []ResellerCostTier `json:"cost_factor_tiers"`
	MaxMarkup          *float64           `json:"max_markup"`           // k 的上限，1 ≤ k ≤ 100
	MaxSiteBonusExtra  *float64           `json:"max_site_bonus_extra"` // 分站加赠比主站同档最多多几个百分点
	TrialGrantMaxUSD   *float64           `json:"trial_grant_max_usd"`
	TrialGrantDailyUSD *float64           `json:"trial_grant_daily_usd"`

	MarkupIncreaseDelayHours int  `json:"markup_increase_delay_hours"`
	SettleHour               int  `json:"settle_hour"`
	FreezeDays               int  `json:"freeze_days"`
	CloseNoticeDays          int  `json:"close_notice_days"`
	SiteLoginOnly            bool `json:"site_login_only"`
	CustomDomainEnabled      bool `json:"custom_domain_enabled"`
	SenderDomainEnabled      bool `json:"sender_domain_enabled"`
	InviteCodeEnabled        bool `json:"invite_code_enabled"`
	SelfUseCheckEnabled      bool `json:"self_use_check_enabled"`
	MigrationCheckEnabled    bool `json:"migration_check_enabled"`
	FingerprintWindowDays    int  `json:"fingerprint_window_days"`
	CustomerLookupPerHour    int  `json:"customer_lookup_per_hour"`
	// 每个站的自定义域名：pending、active、failed 都算进数量；每小时按 created_at 计新建（11.8）。
	MaxCustomDomainsPerSite    int `json:"max_custom_domains_per_site"`
	CustomDomainCreatesPerHour int `json:"custom_domain_creates_per_hour"`

	// Visibility 是站长能看到的数据（11.7）的全局默认：customer_email 取 masked | full，其余取 on | off。
	Visibility map[string]string `json:"visibility"`

	// 自定义域名（11.8）。cloudflare_apiToken、turnstile_widget_secret 是密钥（键名后缀让 GET /api/option/ 过滤掉）。
	CloudflareApiToken    string                    `json:"cloudflare_apiToken"`
	CloudflareZoneId      string                    `json:"cloudflare_zone_id"`
	CloudflareAccountId   string                    `json:"cloudflare_account_id"`
	CnameTarget           string                    `json:"cname_target"`
	FallbackOrigin        string                    `json:"fallback_origin"` // 回源主机名
	TurnstileWidgets      []ResellerTurnstileWidget `json:"turnstile_widgets"`
	TurnstileWidgetSecret map[string]string         `json:"turnstile_widget_secret"` // widget id → secret
}

// ResellerCostTier 是按站点上月客户消费额分档的 d。
type ResellerCostTier struct {
	MinMonthlyUSD float64 `json:"min_monthly_usd"`
	CostFactor    float64 `json:"cost_factor"`
}

// ResellerTurnstileWidget 是登记自定义域名用的 Turnstile widget；secret 单独存在 TurnstileWidgetSecret。
type ResellerTurnstileWidget struct {
	Id      string `json:"id"`
	SiteKey string `json:"site_key"`
}

// 分站设置的模块名和密钥键名。
const (
	ResellerSettingModule     = "adty_reseller_setting"
	ResellerKeyCloudflareTok  = "cloudflare_apiToken"
	ResellerKeyWidgetSecret   = "turnstile_widget_secret"
	ResellerUnknownHostMain   = "main"
	ResellerUnknownHostReject = "reject"
)

// ResellerVisibilityKeys 是 visibility 的键和初始值（11.7）。
var ResellerVisibilityKeys = map[string]string{
	"customer_email":   "masked",
	"customer_balance": "on",
	"customer_topups":  "on",
	"customer_usage":   "on",
	"request_logs":     "on",
	"customer_tokens":  "on",
}

var resellerSetting = DefaultResellerSetting()

// DefaultResellerSetting 是初始值：非金额规则有值，金额项都是 nil。
func DefaultResellerSetting() ResellerSetting {
	visibility := make(map[string]string, len(ResellerVisibilityKeys))
	for key, value := range ResellerVisibilityKeys {
		visibility[key] = value
	}
	return ResellerSetting{
		RootDomains:                []string{},
		ReservedSubdomains:         []string{"www", "api", "admin", "pay", "mail", "news", "links", "status", "docs", "app"},
		UnknownHostMode:            ResellerUnknownHostMain,
		CostFactorTiers:            []ResellerCostTier{},
		MarkupIncreaseDelayHours:   24,
		SettleHour:                 4,
		FreezeDays:                 7,
		CloseNoticeDays:            7,
		SiteLoginOnly:              true,
		SelfUseCheckEnabled:        true,
		MigrationCheckEnabled:      true,
		FingerprintWindowDays:      30,
		CustomerLookupPerHour:      20,
		MaxCustomDomainsPerSite:    2,
		CustomDomainCreatesPerHour: 3,
		Visibility:                 visibility,
		TurnstileWidgets:           []ResellerTurnstileWidget{},
		TurnstileWidgetSecret:      map[string]string{},
	}
}

func init() {
	config.GlobalConfig.Register(ResellerSettingModule, &resellerSetting)
}

// GetResellerSetting 返回当前的分站设置。
func GetResellerSetting() *ResellerSetting {
	return &resellerSetting
}

// ResellerMoneyConfigured 说明开分站要的金额参数是否都设了（档位可以为空）。
func (s *ResellerSetting) ResellerMoneyConfigured() bool {
	return s.CostFactor != nil && s.MinCostFactor != nil && s.MaxMarkup != nil &&
		s.MaxSiteBonusExtra != nil && s.TrialGrantMaxUSD != nil && s.TrialGrantDailyUSD != nil
}

// Ready 说明分站功能可用：总开关开着，金额参数和根域名都已设置。
func (s *ResellerSetting) Ready() bool {
	return s.Enabled && s.ResellerMoneyConfigured() && len(s.RootDomains) > 0
}

// CloudflareConfigured 说明自定义域名要的三项都填了。
func (s *ResellerSetting) CloudflareConfigured() bool {
	return strings.TrimSpace(s.CloudflareApiToken) != "" && strings.TrimSpace(s.CloudflareZoneId) != "" && strings.TrimSpace(s.CnameTarget) != ""
}

var resellerDomainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// ResellerLabelPattern 是一个 DNS 标签：小写字母、数字、连字符，1–63 位，不以连字符开头或结尾。
var ResellerLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validFraction(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && *value > 0 && *value <= 1
}

// ValidateResellerSetting 检查一份要保存的分站设置。resendConfigured 是上游 ResendApiKey 有没有填。
func ValidateResellerSetting(s *ResellerSetting, resendConfigured bool) error {
	for _, domain := range s.RootDomains {
		if !resellerDomainPattern.MatchString(domain) {
			return common.NewMessage("Invalid root domain {{domain}}", map[string]any{"domain": domain})
		}
	}
	for _, label := range s.ReservedSubdomains {
		if !ResellerLabelPattern.MatchString(label) {
			return common.NewMessage("Invalid subdomain {{subdomain}}", map[string]any{"subdomain": label})
		}
	}
	if s.UnknownHostMode != ResellerUnknownHostMain && s.UnknownHostMode != ResellerUnknownHostReject {
		return common.NewMessage("Invalid parameters")
	}
	if s.CostFactor != nil && !validFraction(s.CostFactor) {
		return common.NewMessage("The cost factor must be greater than 0 and at most 1")
	}
	if s.MinCostFactor != nil && !validFraction(s.MinCostFactor) {
		return common.NewMessage("The cost factor must be greater than 0 and at most 1")
	}
	if s.CostFactor != nil && s.MinCostFactor != nil && *s.CostFactor < *s.MinCostFactor {
		return common.NewMessage("The cost factor cannot be below the minimum cost factor")
	}
	for _, tier := range s.CostFactorTiers {
		factor := tier.CostFactor
		if tier.MinMonthlyUSD < 0 || !validFraction(&factor) || (s.MinCostFactor != nil && factor < *s.MinCostFactor) {
			return common.NewMessage("Each tier needs a monthly amount of at least 0 and a cost factor between the minimum and 1")
		}
	}
	if s.MaxMarkup != nil && (math.IsNaN(*s.MaxMarkup) || *s.MaxMarkup < 1 || *s.MaxMarkup > 100) {
		return common.NewMessage("The markup limit must be between 1 and 100")
	}
	if s.MaxSiteBonusExtra != nil && (math.IsNaN(*s.MaxSiteBonusExtra) || *s.MaxSiteBonusExtra < 0 || *s.MaxSiteBonusExtra > 100) {
		return common.NewMessage("The extra bonus must be between 0 and 100")
	}
	for _, value := range []*float64{s.TrialGrantMaxUSD, s.TrialGrantDailyUSD} {
		if value != nil && (math.IsNaN(*value) || *value < 0) {
			return common.NewMessage("Trial credit limits cannot be negative")
		}
	}
	if s.MarkupIncreaseDelayHours < 0 || s.SettleHour < 0 || s.SettleHour > 23 || s.FreezeDays < 0 || s.CloseNoticeDays < 0 ||
		s.FingerprintWindowDays < 1 || s.CustomerLookupPerHour < 0 || s.MaxCustomDomainsPerSite < 0 || s.CustomDomainCreatesPerHour < 0 {
		return common.NewMessage("Invalid parameters")
	}
	for key, value := range s.Visibility {
		def, ok := ResellerVisibilityKeys[key]
		if !ok {
			return common.NewMessage("Invalid parameters")
		}
		allowed := value == "on" || value == "off"
		if def == "masked" {
			allowed = value == "masked" || value == "full"
		}
		if !allowed {
			return common.NewMessage("Invalid parameters")
		}
	}
	for _, widget := range s.TurnstileWidgets {
		if strings.TrimSpace(widget.Id) == "" || strings.TrimSpace(widget.SiteKey) == "" {
			return common.NewMessage("Each Turnstile widget needs an ID and a site key")
		}
	}
	if s.Enabled && !s.ResellerMoneyConfigured() {
		return common.NewMessage("Set the cost factor, its minimum, the markup limit, the extra bonus limit and the trial credit limits before turning reseller sites on")
	}
	if s.Enabled && len(s.RootDomains) == 0 {
		return common.NewMessage("Add at least one root domain before turning reseller sites on")
	}
	if s.CustomDomainEnabled && !s.CloudflareConfigured() {
		return common.NewMessage("Fill in the Cloudflare API token, zone ID and CNAME target before turning custom domains on")
	}
	if s.SenderDomainEnabled && !resendConfigured {
		return common.NewMessage("Fill in the Resend API key under email settings before turning sender domains on")
	}
	return nil
}
