package adty_setting

import "github.com/QuantumNous/new-api/setting/config"

// LimitSetting 是按用户的请求限制（任务 7.4），键名 adty_limit_setting.<json 标签>。
// 默认值 0 表示不限；单个用户可以在 adty_user_limits 里另设（-1 表示跟默认）。
type LimitSetting struct {
	Enabled            bool `json:"enabled"`
	DefaultRPM         int  `json:"default_rpm"`
	DefaultTPM         int  `json:"default_tpm"`
	DefaultConcurrency int  `json:"default_concurrency"`
}

var limitSetting = LimitSetting{}

func init() {
	config.GlobalConfig.Register("adty_limit_setting", &limitSetting)
}

// GetLimitSetting 返回当前的限制设置。
func GetLimitSetting() *LimitSetting {
	return &limitSetting
}
