package adty_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// MarketingSetting 是营销邮件的设置（P12），键名 adty_marketing_setting.<json 标签>。
// WebhookSecret 以 secret 结尾，GET /api/option/ 会过滤掉（约定 D）；后台通过营销设置接口只看「是否已填」。
// 营销总开关要求发件地址、底部发件方名称和联系方式、webhook 密钥、Resend API Key 都已填。
type MarketingSetting struct {
	Enabled                     bool   `json:"enabled"`
	FromAddress                 string `json:"from_address"` // 如 ADTY AI <news@news.example.com>
	ResendSegmentPrefix         string `json:"resend_segment_prefix"`
	ConsentDefaultNonEU         bool   `json:"consent_default_non_eu"` // 欧盟和拿不到国家时一律默认不勾
	ConsentVersion              string `json:"consent_version"`        // 同意文案的版本，改了文案就改它，随同意记录存下
	EUCountries                 string `json:"eu_countries"`           // 逗号分隔的 ISO 国家码，按 CF-IPCountry 判断
	FooterCompany               string `json:"footer_company"`
	FooterAddress               string `json:"footer_address"`
	OpenTracking                bool   `json:"open_tracking"`
	ClickTracking               bool   `json:"click_tracking"`
	SyncBatchSize               int    `json:"sync_batch_size"`
	SyncIntervalSeconds         int    `json:"sync_interval_seconds"`
	APIRatePerSecond            int    `json:"api_rate_per_second"` // 调 Resend 的速度上限（Resend 默认每个团队每秒 10 个请求）
	IncludeSiteCustomersDefault bool   `json:"include_site_customers_default"`
	WebhookSecret               string `json:"webhook_secret"`
	// PromoEnabled 是优惠码总开关（任务 12.4）。零值是关：报价和两个下单接口都不接受优惠码。
	PromoEnabled bool `json:"promo_enabled"`

	// 自动触发邮件（任务 12.3）。每条规则单独开关；标题、正文按内容语言存（JSON 对象），没写的语言用英文版，
	// 英文版没写时这条规则不能打开。同一用户同一规则只发一次。
	TriggerUnpaidEnabled      bool              `json:"trigger_unpaid_enabled"`      // 调用过但没充值
	TriggerUnpaidDays         int               `json:"trigger_unpaid_days"`         // 注册满 N 天
	TriggerUnpaidSubjects     map[string]string `json:"trigger_unpaid_subjects"`     //
	TriggerUnpaidBodies       map[string]string `json:"trigger_unpaid_bodies"`       // HTML
	TriggerLowBalanceEnabled  bool              `json:"trigger_low_balance_enabled"` // 付费用户余额快用完
	TriggerLowBalanceUSD      string            `json:"trigger_low_balance_usd"`     // 余额低于这个美元数
	TriggerLowBalanceSubjects map[string]string `json:"trigger_low_balance_subjects"`
	TriggerLowBalanceBodies   map[string]string `json:"trigger_low_balance_bodies"`
	TriggerWinbackEnabled     bool              `json:"trigger_winback_enabled"` // 付费用户召回
	TriggerWinbackDays        int               `json:"trigger_winback_days"`    // 最近 N 天没有调用
	TriggerWinbackSubjects    map[string]string `json:"trigger_winback_subjects"`
	TriggerWinbackBodies      map[string]string `json:"trigger_winback_bodies"`
	TriggerBatchSize          int               `json:"trigger_batch_size"` // 每条规则每小时最多发多少封
}

// 营销设置在 options 表里的键。
const (
	MarketingKeyPrefix        = "adty_marketing_setting."
	MarketingKeyWebhookSecret = "adty_marketing_setting.webhook_secret"
)

// 初始值：欧盟 27 国加欧洲经济区（冰岛、列支敦士登、挪威）和适用同类规则的英国、瑞士。
const defaultMarketingEUCountries = "AT,BE,BG,HR,CY,CZ,DK,EE,FI,FR,DE,GR,HU,IE,IT,LV,LT,LU,MT,NL,PL,PT,RO,SK,SI,ES,SE,IS,LI,NO,GB,CH"

var marketingSetting = MarketingSetting{
	ResendSegmentPrefix:  "adty",
	ConsentVersion:       "1",
	EUCountries:          defaultMarketingEUCountries,
	OpenTracking:         true,
	ClickTracking:        true,
	SyncBatchSize:        100,
	SyncIntervalSeconds:  60,
	APIRatePerSecond:     8,
	TriggerUnpaidDays:    3,
	TriggerLowBalanceUSD: "1",
	TriggerWinbackDays:   30,
	TriggerBatchSize:     200,
}

func init() {
	config.GlobalConfig.Register("adty_marketing_setting", &marketingSetting)
}

// GetMarketingSetting 返回当前的营销设置。
func GetMarketingSetting() *MarketingSetting {
	return &marketingSetting
}

// IsEUCountry：国家码在 eu_countries 里。空码、XX（未知）、T1（Tor）由调用方当作欧盟处理。
func (s *MarketingSetting) IsEUCountry(country string) bool {
	country = strings.ToUpper(strings.TrimSpace(country))
	for code := range strings.SplitSeq(s.EUCountries, ",") {
		if strings.ToUpper(strings.TrimSpace(code)) == country {
			return true
		}
	}
	return false
}
