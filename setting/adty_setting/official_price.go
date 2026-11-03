package adty_setting

import "github.com/QuantumNous/new-api/setting/config"

// OfficialPriceSetting 是官方价同步的设置（任务 8.4），键名 adty_official_price_setting.<json 标签>。
// AutoMapProviders：没映射的模型按 ID 完全相同，依次在这些 models.dev provider 里找。
type OfficialPriceSetting struct {
	AutoMapProviders []string `json:"auto_map_providers"`
}

var officialPriceSetting = OfficialPriceSetting{
	AutoMapProviders: []string{"anthropic", "openai", "google", "deepseek", "xai", "mistral"},
}

func init() {
	config.GlobalConfig.Register("adty_official_price_setting", &officialPriceSetting)
}

// GetOfficialPriceSetting 返回当前的官方价设置。
func GetOfficialPriceSetting() *OfficialPriceSetting {
	return &officialPriceSetting
}
