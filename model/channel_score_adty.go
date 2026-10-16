package model

// SmartWeightHook overlays route_weight on official priority bands.
// Registered by service/smartrouting.Init to avoid model→service import cycle.
var SmartWeightHook func(channels []*Channel, modelName, group string) ([]*Channel, []int)

// RegisterSmartWeightHook sets the smart-routing weight hook.
func RegisterSmartWeightHook(fn func(channels []*Channel, modelName, group string) ([]*Channel, []int)) {
	SmartWeightHook = fn
}

// ApplySmartWeights returns channels + effective weights for one priority band.
// When hook is nil or returns empty weights, falls back to GetWeight().
func ApplySmartWeights(channels []*Channel, modelName, group string) ([]*Channel, []int) {
	if len(channels) == 0 {
		return channels, nil
	}
	if SmartWeightHook != nil {
		out, weights := SmartWeightHook(channels, modelName, group)
		if len(out) > 0 && len(weights) == len(out) {
			return out, weights
		}
	}
	weights := make([]int, len(channels))
	for i, ch := range channels {
		weights[i] = ch.GetWeight()
	}
	return channels, weights
}
