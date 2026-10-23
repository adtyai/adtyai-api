package model

import (
	"github.com/QuantumNous/new-api/dto"
)

// RouteCaps is the per-request capability filter. Zero means "not required".
type RouteCaps struct {
	IsStream    bool
	NeedTools   bool
	NeedVision  bool
	NeedJSON    bool
	NeedContext int
	// StickyChannelID is a soft preference. Zero means no preference.
	StickyChannelID int
	Filters         []dto.ChannelFilter `json:"-"`
	// BandPriority is the walker's band key. Ability priority and channel priority can differ.
	BandPriority    int64 `json:"-"`
	HasBandPriority bool  `json:"-"`
	// PriorityOf returns that band key for a channel. Nil means Channel.GetPriority.
	PriorityOf func(channelID int) (int64, bool) `json:"-"`
	// DryRun selects without reserving inflight. Preview only.
	DryRun bool `json:"-"`
}

// BandIndexNotWalked marks a pick that did not come from the priority walk.
// The next walk still starts at the official retry index.
const BandIndexNotWalked = -1

// RouteExclusion is one channel removed from a priority band.
type RouteExclusion struct {
	ID  int    `json:"id"`
	Why string `json:"why"`
}

// RouteCandView is a dispatch candidate recorded for the admin log.
type RouteCandView struct {
	ID     int    `json:"id"`
	Weight int    `json:"w"`
	Note   string `json:"note,omitempty"`
}

// RouteBandDecision is the selection record for one request.
type RouteBandDecision struct {
	Priority   int64            `json:"prio,omitempty"`
	Picked     int              `json:"picked,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	Excluded   []RouteExclusion `json:"excluded,omitempty"`
	Candidates []RouteCandView  `json:"cands,omitempty"`
	HalfOpen   bool             `json:"halfopen,omitempty"`
	Advanced   bool             `json:"advanced,omitempty"`
	Mode       string           `json:"mode,omitempty"`
	Official   bool             `json:"official,omitempty"`
	WouldPick  int              `json:"would_pick,omitempty"`
	BandIndex  int              `json:"band,omitempty"`
	Group      string           `json:"group,omitempty"`
}

// SmartWeightResult is one priority band after smart routing.
// Skip means this band has no dispatchable channel and the caller may walk down.
type SmartWeightResult struct {
	Channels []*Channel
	Weights  []int
	Skip     bool
	HalfOpen *Channel
	Excluded []RouteExclusion
	Decision RouteBandDecision
}

// SmartWeightHook overlays route_weight on one official priority band.
// Registered by service/smartrouting.Init to avoid model→service import cycle.
var SmartWeightHook func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult

// OnChannelSelectedHook is called after a normal weighted pick.
// It must not reserve a half-open probe slot. Probe paths use OnHalfOpenProbeHook.
var OnChannelSelectedHook func(channelID int) bool

// OnHalfOpenProbeHook reserves one half-open probe slot. Return false when the quota is full.
var OnHalfOpenProbeHook func(channelID int) bool

// RegisterSmartWeightHook sets the smart-routing weight hook.
func RegisterSmartWeightHook(fn func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult) {
	SmartWeightHook = fn
}

// RegisterOnChannelSelectedHook sets the post-pick hook for a normal weighted pick.
func RegisterOnChannelSelectedHook(fn func(channelID int) bool) {
	OnChannelSelectedHook = fn
}

// RegisterOnHalfOpenProbeHook sets the half-open probe reservation hook.
func RegisterOnHalfOpenProbeHook(fn func(channelID int) bool) {
	OnHalfOpenProbeHook = fn
}

// ApplySmartWeights returns one band's dispatch set. A nil hook uses official weights.
func ApplySmartWeights(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
	if len(channels) == 0 {
		return SmartWeightResult{Skip: true, Decision: RouteBandDecision{Reason: "empty"}}
	}
	if SmartWeightHook != nil {
		return SmartWeightHook(channels, modelName, group, caps)
	}
	weights := make([]int, len(channels))
	cands := make([]RouteCandView, len(channels))
	for i, ch := range channels {
		weights[i] = ch.GetWeight()
		if ch != nil {
			cands[i] = RouteCandView{ID: ch.Id, Weight: weights[i], Note: "official"}
		}
	}
	return SmartWeightResult{
		Channels: channels,
		Weights:  weights,
		Decision: RouteBandDecision{Reason: "official", Official: true, Candidates: cands},
	}
}

// ConfirmChannelSelected runs OnChannelSelectedHook; true when selection is accepted.
func ConfirmChannelSelected(channelID int) bool {
	if OnChannelSelectedHook == nil {
		return true
	}
	return OnChannelSelectedHook(channelID)
}

// ConfirmHalfOpenProbe reserves a probe slot. A nil probe hook keeps the older selected-hook behavior.
func ConfirmHalfOpenProbe(channelID int) bool {
	if OnHalfOpenProbeHook != nil {
		return OnHalfOpenProbeHook(channelID)
	}
	return ConfirmChannelSelected(channelID)
}

// HalfOpenFallbackHook 审查 N-P1-2：候选为空（健康渠道全部熔断）时，返回一个冷却到期、可半开探测的渠道。
// accept 用于让调用方按请求过滤条件（ChannelSatisfiesFilters）筛掉不合格的候选。
var HalfOpenFallbackHook func(modelName, group string, caps RouteCaps, accept func(*Channel) bool) *Channel

// RegisterHalfOpenFallbackHook sets the empty-candidate half-open fallback.
func RegisterHalfOpenFallbackHook(fn func(modelName, group string, caps RouteCaps, accept func(*Channel) bool) *Channel) {
	HalfOpenFallbackHook = fn
}

// halfOpenFallback 选一个到期半开渠道并占用 inflight；没有则 nil。
// 审查 R-P1-1：分发中间件对每个请求都带过滤条件（FilterRequestPath 等），
// 回退候选必须通过 ChannelSatisfiesFilters 校验，而不是要求 filters 为空。
func halfOpenFallback(modelName, group string, caps RouteCaps, filters []dto.ChannelFilter) (*Channel, bool, error) {
	if HalfOpenFallbackHook == nil {
		return nil, false, nil
	}
	caps.Filters = filters
	accept := func(ch *Channel) bool {
		if ch == nil {
			return false
		}
		ok, _ := ChannelSatisfiesFilters(ch, modelName, filters)
		return ok
	}
	ch := HalfOpenFallbackHook(modelName, group, caps, accept)
	if ch == nil || !accept(ch) {
		return nil, false, nil
	}
	if caps.DryRun {
		return ch, false, nil
	}
	if !ConfirmHalfOpenProbe(ch.Id) {
		return nil, true, nil
	}
	return ch, false, nil
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
