package ratio_setting

import "github.com/shopspring/decimal"

// DecimalFromFloat converts a float64 setting at the storage boundary.
// Financial arithmetic must use the returned decimal, not the original float.
func DecimalFromFloat(value float64) decimal.Decimal {
	return decimal.NewFromFloat(value)
}

// GetGroupRatioDecimal is GetGroupRatio converted once at the setting boundary.
func GetGroupRatioDecimal(name string) decimal.Decimal {
	return DecimalFromFloat(GetGroupRatio(name))
}

// GetGroupGroupRatioDecimal is GetGroupGroupRatio converted once at the boundary.
func GetGroupGroupRatioDecimal(userGroup, usingGroup string) (decimal.Decimal, bool) {
	ratio, ok := GetGroupGroupRatio(userGroup, usingGroup)
	if !ok {
		return decimal.Zero, false
	}
	return DecimalFromFloat(ratio), true
}

// GetModelPriceDecimal is GetModelPrice converted once at the setting boundary.
func GetModelPriceDecimal(name string, printErr bool) (decimal.Decimal, bool) {
	price, ok := GetModelPrice(name, printErr)
	if !ok {
		return decimal.Zero, false
	}
	return DecimalFromFloat(price), true
}

// GetModelRatioDecimal is GetModelRatio converted once at the setting boundary.
func GetModelRatioDecimal(name string) (decimal.Decimal, bool, string) {
	ratio, ok, match := GetModelRatio(name)
	return DecimalFromFloat(ratio), ok, match
}

// GetCompletionRatioDecimal is GetCompletionRatio converted once at the boundary.
func GetCompletionRatioDecimal(name string) decimal.Decimal {
	return DecimalFromFloat(GetCompletionRatio(name))
}

// GetAudioRatioDecimal is GetAudioRatio converted once at the setting boundary.
func GetAudioRatioDecimal(name string) decimal.Decimal {
	return DecimalFromFloat(GetAudioRatio(name))
}

// GetAudioCompletionRatioDecimal is GetAudioCompletionRatio converted once at the boundary.
func GetAudioCompletionRatioDecimal(name string) decimal.Decimal {
	return DecimalFromFloat(GetAudioCompletionRatio(name))
}
