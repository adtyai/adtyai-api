package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// IsKnownOptionKey 审查 R-P2-5：通用 option PUT 只接受已知 key——
// OptionMap 中已有的 key、请求策略/Passkey/模型定价等专用 key，以及已注册分层配置前缀（xxx_setting.*）。
func IsKnownOptionKey(key string) bool {
	if key == "" {
		return false
	}
	common.OptionMapRWMutex.RLock()
	_, ok := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if ok {
		return true
	}
	if IsRequestPolicyOption(key) || IsPasskeyDomainOption(key) || IsModelPricingOption(key) || key == operation_setting.ToolPriceOptionKey {
		return true
	}
	if parts := strings.SplitN(key, ".", 2); len(parts) == 2 && parts[1] != "" {
		return config.GlobalConfig.Get(parts[0]) != nil
	}
	return false
}
