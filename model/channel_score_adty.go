package model

import "github.com/QuantumNous/new-api/dto"

// SmartWeightHook overlays route_weight on official priority bands.
// Registered by service/smartrouting.Init to avoid model→service import cycle.
var SmartWeightHook func(channels []*Channel, modelName, group string) ([]*Channel, []int)

// OnChannelSelectedHook is called after a channel is picked for an outbound attempt.
// Return false to reject the pick (e.g. half-open in-flight full) so the caller can reselect.
var OnChannelSelectedHook func(channelID int) bool

// RegisterSmartWeightHook sets the smart-routing weight hook.
func RegisterSmartWeightHook(fn func(channels []*Channel, modelName, group string) ([]*Channel, []int)) {
	SmartWeightHook = fn
}

// RegisterOnChannelSelectedHook sets the post-pick hook (half-open inflight reserve).
func RegisterOnChannelSelectedHook(fn func(channelID int) bool) {
	OnChannelSelectedHook = fn
}

// ApplySmartWeights returns channels + effective weights for one priority band.
// When hook returns (nil, nil) that is H2 skip-band. Otherwise falls back to GetWeight().
func ApplySmartWeights(channels []*Channel, modelName, group string) ([]*Channel, []int) {
	if len(channels) == 0 {
		return channels, nil
	}
	if SmartWeightHook != nil {
		out, weights := SmartWeightHook(channels, modelName, group)
		// H2: only half-open left → skip this priority band
		if out == nil && weights == nil {
			return nil, nil
		}
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

// ConfirmChannelSelected runs OnChannelSelectedHook; true when selection is accepted.
func ConfirmChannelSelected(channelID int) bool {
	if OnChannelSelectedHook == nil {
		return true
	}
	return OnChannelSelectedHook(channelID)
}

// HalfOpenFallbackHook 审查 N-P1-2：候选为空（健康渠道全部熔断）时，返回一个冷却到期、可半开探测的渠道。
// accept 用于让调用方按请求过滤条件（ChannelSatisfiesFilters）筛掉不合格的候选。
var HalfOpenFallbackHook func(modelName, group string, accept func(*Channel) bool) *Channel

// RegisterHalfOpenFallbackHook sets the empty-candidate half-open fallback.
func RegisterHalfOpenFallbackHook(fn func(modelName, group string, accept func(*Channel) bool) *Channel) {
	HalfOpenFallbackHook = fn
}

// halfOpenFallback 选一个到期半开渠道并占用 inflight；没有则 nil。
// 审查 R-P1-1：分发中间件对每个请求都带过滤条件（FilterRequestPath 等），
// 回退候选必须通过 ChannelSatisfiesFilters 校验，而不是要求 filters 为空。
func halfOpenFallback(modelName, group string, filters []dto.ChannelFilter) *Channel {
	if HalfOpenFallbackHook == nil {
		return nil
	}
	accept := func(ch *Channel) bool {
		if ch == nil {
			return false
		}
		ok, _ := ChannelSatisfiesFilters(ch, modelName, filters)
		return ok
	}
	ch := HalfOpenFallbackHook(modelName, group, accept)
	if ch != nil && !accept(ch) {
		return nil
	}
	if ch == nil || !ConfirmChannelSelected(ch.Id) {
		return nil
	}
	return ch
}

// OnChannelManuallyEnabledHook 审查 N-P2-7：管理员手动启用渠道后同步重置熔断状态。
var OnChannelManuallyEnabledHook func(channelID int)

func RegisterOnChannelManuallyEnabledHook(fn func(channelID int)) { OnChannelManuallyEnabledHook = fn }

// NotifyChannelManuallyEnabled 调用重置挂钩（未注册时无操作）。
func NotifyChannelManuallyEnabled(channelID int) {
	if OnChannelManuallyEnabledHook != nil {
		OnChannelManuallyEnabledHook(channelID)
	}
}
