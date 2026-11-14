package ratio_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

// unsetModelRatioDecimal is the self-use fallback previously returned as 37.5.
var unsetModelRatioDecimal = decimal.RequireFromString("37.5")

// DecimalFromFloat converts a float64 setting at the storage or default-literal
// boundary. After that conversion, financial arithmetic must use the decimal
// authority rather than the original float.
func DecimalFromFloat(value float64) decimal.Decimal {
	return decimal.NewFromFloat(value)
}

// GetGroupRatioDecimal reads the decimal group-ratio authority. Missing names
// still default to 1, matching GetGroupRatio.
func GetGroupRatioDecimal(name string) decimal.Decimal {
	if ratio, ok := groupRatioStore.getDecimal(name); ok {
		return ratio
	}
	common.SysLog(common.LogText("group ratio not found: %s", name))
	return decimal.NewFromInt(1)
}

// GetGroupGroupRatioDecimal reads the decimal group-group ratio authority.
func GetGroupGroupRatioDecimal(userGroup, usingGroup string) (decimal.Decimal, bool) {
	return groupGroupRatioStore.getDecimal(userGroup, usingGroup)
}

// GetModelPriceDecimal reads the decimal model-price authority.
func GetModelPriceDecimal(name string, printErr bool) (decimal.Decimal, bool) {
	name = FormatMatchingModelName(name)
	if price, ok := modelPriceStore.getDecimal(name); ok {
		return price, true
	}
	if printErr {
		common.SysError(common.LogText("model price not found: %s", name))
	}
	return decimal.Zero, false
}

// GetModelRatioDecimal reads the decimal model-ratio authority.
func GetModelRatioDecimal(name string) (decimal.Decimal, bool, string) {
	name = FormatMatchingModelName(name)
	if ratio, ok := modelRatioStore.getDecimal(name); ok {
		return ratio, true, name
	}
	return unsetModelRatioDecimal, operation_setting.SelfUseModeEnabled, name
}

// GetCompletionRatioDecimal follows the same configured vs hardcoded policy as
// GetCompletionRatio, using the decimal authority for saved values.
func GetCompletionRatioDecimal(name string) decimal.Decimal {
	name = FormatMatchingModelName(name)
	configured, ok := completionRatioStore.getDecimal(name)
	if strings.Contains(name, "/") && ok {
		return configured
	}
	hardCodedRatio, locked := getHardcodedCompletionModelRatio(name)
	if locked {
		return DecimalFromFloat(hardCodedRatio)
	}
	if ok {
		return configured
	}
	return DecimalFromFloat(hardCodedRatio)
}

// GetAudioRatioDecimal reads the decimal audio-ratio authority. Missing names
// still default to 1, matching GetAudioRatio.
func GetAudioRatioDecimal(name string) decimal.Decimal {
	name = FormatMatchingModelName(name)
	if ratio, ok := audioRatioStore.getDecimal(name); ok {
		return ratio
	}
	return decimal.NewFromInt(1)
}

// GetAudioCompletionRatioDecimal reads the decimal audio completion-ratio
// authority. Missing names still default to 1, matching GetAudioCompletionRatio.
func GetAudioCompletionRatioDecimal(name string) decimal.Decimal {
	name = FormatMatchingModelName(name)
	if ratio, ok := audioCompletionRatioStore.getDecimal(name); ok {
		return ratio
	}
	return decimal.NewFromInt(1)
}
