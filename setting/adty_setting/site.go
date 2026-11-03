package adty_setting

import "github.com/QuantumNous/new-api/setting/config"

// SiteSetting 是站点的公开配置（任务 8.3），键名 adty_site_setting.<json 标签>，GET /api/adty/site 原样返回。
// 键名不能以 Key、Secret、Token 结尾，否则 GET /api/option/ 会把它们过滤掉，后台表单读不回来。
// APIBaseURL 为空时前端回退到 status 里的 server_address。
type SiteSetting struct {
	APIBaseURL          string `json:"api_base_url"`
	SupportEmail        string `json:"support_email"`
	TelegramURL         string `json:"telegram_url"`
	SupportHours        string `json:"support_hours"`
	TawkPropertyID      string `json:"tawk_property_id"`
	TawkWidgetID        string `json:"tawk_widget_id"`
	RefundPolicyEnabled bool   `json:"refund_policy_enabled"` // 退款政策页和各处链接是否显示
}

var siteSetting = SiteSetting{}

func init() {
	config.GlobalConfig.Register("adty_site_setting", &siteSetting)
}

// GetSiteSetting 返回当前的站点配置。
func GetSiteSetting() *SiteSetting {
	return &siteSetting
}
