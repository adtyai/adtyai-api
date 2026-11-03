package adty_setting

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// TopupSetting 是充值加赠和订单有效期（任务 3.1）。键名是 adty_topup_setting.<json 标签>。
type TopupSetting struct {
	BonusRates       map[int]float64 `json:"bonus_rates"`        // 档位 USD → 加赠百分比，2.5 表示 2.5%
	USDTOrderMinutes int             `json:"usdt_order_minutes"` // USDT 订单有效期
	EpayOrderMinutes int             `json:"epay_order_minutes"` // 易支付订单有效期
}

// TopupSettingKeyBonusRates 是加赠比例的配置键，保存前要校验（controller/option.go）。
const TopupSettingKeyBonusRates = "adty_topup_setting.bonus_rates"

// 加赠没有内置档位（约定 I）：后台没设时不加赠，档位金额沿用上游的 payment_setting.amount_options。
var topupSetting = TopupSetting{
	BonusRates:       map[int]float64{},
	USDTOrderMinutes: 15,
	EpayOrderMinutes: 10,
}

func init() {
	config.GlobalConfig.Register("adty_topup_setting", &topupSetting)
}

// GetTopupSetting 返回当前的充值设置。
func GetTopupSetting() *TopupSetting {
	return &topupSetting
}

// ValidateBonusRates 检查 bonus_rates 的 JSON：键是正整数美元，值在 [0, 100]。
// 非法值不拦的话，config 会静默保留旧值，而 OptionMap 已经写成新字符串。
func ValidateBonusRates(value string) error {
	raw := map[string]float64{}
	if err := common.UnmarshalJsonStr(value, &raw); err != nil {
		return common.NewMessage("Bonus rates must be a JSON object of amount to percent")
	}
	for key, rate := range raw {
		amount, err := strconv.Atoi(key)
		if err != nil || amount <= 0 || fmt.Sprint(amount) != key {
			return common.NewMessage("Bonus tier {{tier}} must be a whole dollar amount", map[string]any{"tier": key})
		}
		if rate < 0 || rate > 100 {
			return common.NewMessage("Bonus rate for {{tier}} must be between 0 and 100", map[string]any{"tier": key})
		}
	}
	return nil
}
