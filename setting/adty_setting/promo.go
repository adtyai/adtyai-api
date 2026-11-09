package adty_setting

import "github.com/QuantumNous/new-api/setting/config"

// PromoEnabledOptionKey retains the existing top-up flag across upgrades.
// The retired marketing settings are not registered or used by any task.
const PromoEnabledOptionKey = "adty_marketing_setting.promo_enabled"

type PromoSetting struct {
	PromoEnabled bool `json:"promo_enabled"`
}

var promoSetting PromoSetting

func init() {
	config.GlobalConfig.Register("adty_marketing_setting", &promoSetting)
}

func GetPromoSetting() *PromoSetting {
	return &promoSetting
}
