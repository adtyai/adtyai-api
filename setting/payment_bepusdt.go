package setting

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// BEpusdtSetting 是 USDT 网关的配置（任务 4.1），键名 adty_bepusdt.<json 标签>。
// secret 以 secret 结尾，GET /api/option/ 会把它过滤掉。
type BEpusdtSetting struct {
	Enabled    bool     `json:"enabled"`
	BaseURL    string   `json:"base_url"`
	Secret     string   `json:"secret"`
	TradeTypes []string `json:"trade_types"` // usdt.trc20、usdt.bep20、usdt.polygon、usdt.xlayer
}

var bepusdtSetting = BEpusdtSetting{
	TradeTypes: []string{"usdt.trc20"},
}

func init() {
	config.GlobalConfig.Register("adty_bepusdt", &bepusdtSetting)
}

// GetBEpusdtSetting 返回当前的 BEpusdt 配置。
func GetBEpusdtSetting() *BEpusdtSetting {
	return &bepusdtSetting
}

// BEpusdtConfigured 说明网关地址和密钥都已填写并且启用。
func BEpusdtConfigured() bool {
	return bepusdtSetting.Enabled &&
		strings.TrimSpace(bepusdtSetting.BaseURL) != "" &&
		strings.TrimSpace(bepusdtSetting.Secret) != ""
}

// BEpusdtChainEnabled 说明这条链已启用。
func BEpusdtChainEnabled(tradeType string) bool {
	return slices.Contains(bepusdtSetting.TradeTypes, tradeType)
}
