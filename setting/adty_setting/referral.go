package adty_setting

import "github.com/QuantumNous/new-api/setting/config"

// ReferralSetting 是推广返佣的设置（任务 6.1），键名 adty_referral_setting.<json 标签>。
// 比例是百分数，10 表示 10%（执行层决定 18）。提现汇率用字符串存：空串表示未设置，
// 提交提现时直接查库，不读这里（任务 6.4）。
// 和钱有关的三项没有内置值（约定 I）：nil 表示未设置，存成 "null"。0% 和没设是两回事。
type ReferralSetting struct {
	Enabled                  bool     `json:"enabled"`
	DefaultRate              *float64 `json:"default_rate"` // 没设时不计佣，也不能打开 enabled
	CookieTTLDays            int      `json:"cookie_ttl_days"`
	FreezeDays               int      `json:"freeze_days"`
	MinWithdrawUSD           *float64 `json:"min_withdraw_usd"`  // 没设时不能提现到 USDT 和收款码
	USDTWithdrawFee          *float64 `json:"usdt_withdraw_fee"` // 没设时不能提现到 USDT
	CancelWindowMinutes      int      `json:"cancel_window_minutes"`
	WithdrawCNYRate          string   `json:"withdraw_cny_rate"`
	WithdrawCNYRateUpdatedAt int64    `json:"withdraw_cny_rate_updated_at"`
	SelfInviteWindowDays     int      `json:"self_invite_window_days"`
	BatchRegisterThreshold   int      `json:"batch_register_threshold"`
	BatchRegisterWindowHours int      `json:"batch_register_window_hours"`
}

// 推广设置在 options 表里的键。
const (
	ReferralKeyPrefix                   = "adty_referral_setting."
	ReferralKeyWithdrawCNYRate          = "adty_referral_setting.withdraw_cny_rate"
	ReferralKeyWithdrawCNYRateUpdatedAt = "adty_referral_setting.withdraw_cny_rate_updated_at"
)

var referralSetting = ReferralSetting{
	CookieTTLDays:            30,
	FreezeDays:               7,
	CancelWindowMinutes:      30,
	SelfInviteWindowDays:     30,
	BatchRegisterThreshold:   3,
	BatchRegisterWindowHours: 24,
}

func init() {
	config.GlobalConfig.Register("adty_referral_setting", &referralSetting)
}

// GetReferralSetting 返回当前的推广设置。
func GetReferralSetting() *ReferralSetting {
	return &referralSetting
}
