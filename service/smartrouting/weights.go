package smartrouting

import (
	"math/rand"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// SmartWeights overlays one priority band.
// off keeps official weights. observe records a shadow pick but still dispatches official weights.
// active never returns an excluded channel. An empty active band sets Skip so the selector walks down.
func SmartWeights(channels []*model.Channel, modelName, group string, caps model.RouteCaps) model.SmartWeightResult {
	n := len(channels)
	if n == 0 {
		return model.SmartWeightResult{Skip: true, Decision: model.RouteBandDecision{Reason: "empty"}}
	}
	mode := routingMode()
	official := officialWeights(channels)
	officialCands := candViews(channels, official, "official")
	if mode == "off" {
		return model.SmartWeightResult{
			Channels: channels,
			Weights:  official,
			Decision: model.RouteBandDecision{Mode: "off", Official: true, Reason: "off", Candidates: officialCands},
		}
	}

	cfg := GetConfig()
	active := filterBand(channels, modelName, group, caps, cfg)
	if mode == "observe" {
		would := argmaxID(active.channels, active.weights)
		return model.SmartWeightResult{
			Channels: channels,
			Weights:  official,
			Excluded: active.excluded,
			Decision: model.RouteBandDecision{
				Mode:       "observe",
				Official:   true,
				Reason:     "observe",
				WouldPick:  would,
				Excluded:   active.excluded,
				Candidates: officialCands,
			},
		}
	}

	if active.stale {
		if len(active.channels) == 0 {
			return model.SmartWeightResult{
				Skip:     true,
				HalfOpen: active.half,
				Excluded: active.excluded,
				Decision: model.RouteBandDecision{Mode: "active", Reason: "stale_empty", Excluded: active.excluded},
			}
		}
		active.weights = decorateBandWeights(active.channels, active.weights, modelName, caps)
		return model.SmartWeightResult{
			Channels: active.channels,
			Weights:  active.weights,
			HalfOpen: active.half,
			Excluded: active.excluded,
			Decision: model.RouteBandDecision{
				Mode: "active", Official: true, Reason: "stale",
				Excluded: active.excluded, Candidates: candViews(active.channels, active.weights, "stale"),
			},
		}
	}
	if len(active.channels) == 0 {
		return model.SmartWeightResult{
			Skip:     true,
			HalfOpen: active.half,
			Excluded: active.excluded,
			Decision: model.RouteBandDecision{Mode: "active", Reason: "skip", Excluded: active.excluded},
		}
	}
	active.weights = decorateBandWeights(active.channels, active.weights, modelName, caps)
	return model.SmartWeightResult{
		Channels: active.channels,
		Weights:  active.weights,
		HalfOpen: active.half,
		Excluded: active.excluded,
		Decision: model.RouteBandDecision{
			Mode: "active", Reason: "score",
			Excluded: active.excluded, Candidates: candViews(active.channels, active.weights, ""),
		},
	}
}

type bandFilter struct {
	channels []*model.Channel
	weights  []int
	excluded []model.RouteExclusion
	half     *model.Channel
	stale    bool
}

func filterBand(channels []*model.Channel, modelName, group string, caps model.RouteCaps, cfg Config) bandFilter {
	var out bandFilter
	scores := GetAllScores()
	out.stale = scoresStale(scores, cfg)
	var normal []*model.Channel
	var normalW []int
	var floors []*model.Channel
	var floorW []int
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		if why := hardExclude(ch, modelName, caps); why != "" {
			out.excluded = append(out.excluded, model.RouteExclusion{ID: ch.Id, Why: why})
			continue
		}
		if breakerOpen(ch.Id) {
			out.excluded = append(out.excluded, model.RouteExclusion{ID: ch.Id, Why: "breaker"})
			if out.half == nil && halfOpenReady(ch.Id) && halfOpenAllowed(ch, modelName, group, caps, cfg) {
				out.half = ch
			}
			continue
		}
		w, note := routeWeight(ch, modelName, out.stale)
		if model.CachedHasRouteTag(ch.Id, modelName, "fallback_only") {
			floors = append(floors, ch)
			floorW = append(floorW, w)
			_ = note
			continue
		}
		normal = append(normal, ch)
		normalW = append(normalW, w)
	}
	if len(normal) == 0 && len(floors) > 0 {
		out.channels = floors
		out.weights = floorW
		return out
	}
	if len(floors) > 0 && len(normal) > 0 {
		for i, ch := range floors {
			out.excluded = append(out.excluded, model.RouteExclusion{ID: ch.Id, Why: "fallback_only"})
			_ = floorW[i]
		}
	}
	out.channels = normal
	out.weights = normalW
	if out.half == nil {
		out.half = halfOpenFromReady(channels, modelName, group, caps, cfg)
	}
	return out
}

func routeWeight(ch *model.Channel, modelName string, stale bool) (int, string) {
	official := ch.GetWeight()
	if official < 1 {
		official = 1
	}
	if stale {
		return official, "stale"
	}
	se := GetScore(ch.Id, modelName)
	if se == nil || se.Insufficient || se.State == "insufficient" || se.State == "new" {
		return official, "insufficient"
	}
	if se.RouteWeight < 1 {
		return official, "official"
	}
	return se.RouteWeight, se.State
}

func hardExclude(ch *model.Channel, modelName string, caps model.RouteCaps) string {
	if ch == nil {
		return "excluded"
	}
	if model.CachedHasRouteTag(ch.Id, modelName, "excluded") || model.CachedHasRouteTag(ch.Id, modelName, "fake_model") {
		return "excluded"
	}
	if model.CachedCostGuardExcluded(ch.Id) {
		return "cost_guard"
	}
	if InRateLimitCooldown(ch.Id, modelName) {
		return "rpm_cooldown"
	}
	if rpmExceededCached(ch.Id) {
		return "rpm_full"
	}
	return capabilityRejectCached(ch.Id, caps)
}

func halfOpenReady(channelID int) bool {
	for _, h := range ListHalfOpenReady() {
		if h.ChannelID == channelID {
			return true
		}
	}
	return false
}

func halfOpenStatusAllowed(ch *model.Channel) bool {
	return ch != nil && (ch.Status == common.ChannelStatusEnabled || ch.Status == common.ChannelStatusScoreBroken)
}

func channelBandKey(ch *model.Channel, caps model.RouteCaps) (int64, bool) {
	if ch == nil {
		return 0, false
	}
	if caps.PriorityOf != nil {
		return caps.PriorityOf(ch.Id)
	}
	return ch.GetPriority(), true
}

func halfOpenAllowed(ch *model.Channel, modelName, group string, caps model.RouteCaps, cfg Config) bool {
	if !halfOpenStatusAllowed(ch) {
		return false
	}
	// Streaming does not reject a half-open probe. PreferNonStream is not a selection gate.
	if hardExclude(ch, modelName, caps) != "" {
		return false
	}
	if !channelServesModel(ch, modelName) || !channelInGroup(ch, group) {
		return false
	}
	if HalfOpenInflightFull(ch.Id, cfg.HalfOpenMaxInflight) {
		return false
	}
	return true
}

func halfOpenBandTarget(band []*model.Channel, caps model.RouteCaps) (int64, bool) {
	if caps.HasBandPriority {
		return caps.BandPriority, true
	}
	for _, ch := range band {
		if ch == nil {
			continue
		}
		return ch.GetPriority(), true
	}
	return 0, false
}

func halfOpenFromReady(band []*model.Channel, modelName, group string, caps model.RouteCaps, cfg Config) *model.Channel {
	want, ok := halfOpenBandTarget(band, caps)
	if !ok {
		return nil
	}
	for _, h := range ListHalfOpenReady() {
		if h.Model != "" && h.Model != "*" && h.Model != modelName {
			continue
		}
		ch, ok := model.CacheGetChannelMemory(h.ChannelID)
		if !ok {
			continue
		}
		key, matched := channelBandKey(ch, caps)
		if !matched || key != want {
			continue
		}
		if ok, _ := model.ChannelSatisfiesFilters(ch, modelName, caps.Filters); !ok {
			continue
		}
		if halfOpenAllowed(ch, modelName, group, caps, cfg) {
			return ch
		}
	}
	return nil
}

func rpmExceededCached(channelID int) bool {
	cap, ok := model.CachedCapability(channelID)
	if !ok || cap.RPMHint <= 0 {
		return false
	}
	return rpmCount(channelID) >= cap.RPMHint
}

func capabilityRejectCached(channelID int, caps model.RouteCaps) string {
	cap, ok := model.CachedCapability(channelID)
	if !ok {
		return ""
	}
	if caps.NeedTools && cap.SupportsTools != nil && !*cap.SupportsTools {
		return "capability:tools"
	}
	if caps.NeedVision && cap.SupportsVision != nil && !*cap.SupportsVision {
		return "capability:vision"
	}
	if caps.NeedJSON && cap.SupportsJSONSchema != nil && !*cap.SupportsJSONSchema {
		return "capability:json_schema"
	}
	if caps.NeedContext > 0 && cap.MaxContext > 0 && caps.NeedContext > cap.MaxContext {
		return "capability:max_context"
	}
	return ""
}

func officialWeights(channels []*model.Channel) []int {
	out := make([]int, len(channels))
	for i, ch := range channels {
		if ch != nil {
			out[i] = ch.GetWeight()
		}
	}
	return out
}

func candViews(channels []*model.Channel, weights []int, note string) []model.RouteCandView {
	out := make([]model.RouteCandView, 0, len(channels))
	for i, ch := range channels {
		if ch == nil {
			continue
		}
		w := 0
		if i < len(weights) {
			w = weights[i]
		}
		out = append(out, model.RouteCandView{ID: ch.Id, Weight: w, Note: note})
	}
	return out
}

func argmaxID(channels []*model.Channel, weights []int) int {
	bestID := 0
	best := -1
	for i, ch := range channels {
		if ch == nil {
			continue
		}
		w := 0
		if i < len(weights) {
			w = weights[i]
		}
		if w > best {
			best = w
			bestID = ch.Id
		}
	}
	return bestID
}

func scoresStale(scores map[string]*ScoreEntry, cfg Config) bool {
	if len(scores) == 0 || cfg.ScoreStaleSec <= 0 {
		return false
	}
	cutoff := time.Now().Add(-time.Duration(cfg.ScoreStaleSec) * time.Second)
	for _, s := range scores {
		if s != nil && s.UpdatedAt.After(cutoff) {
			return false
		}
	}
	return true
}

func routingMode() string {
	cfg := GetConfig()
	switch cfg.Mode {
	case "off", "observe", "active":
		return cfg.Mode
	}
	if cfg.Enabled {
		return "active"
	}
	return "off"
}

func channelServesModel(ch *model.Channel, modelName string) bool {
	if ch == nil {
		return false
	}
	for _, m := range ch.GetModels() {
		if m == modelName {
			return true
		}
	}
	return false
}

func channelInGroup(ch *model.Channel, group string) bool {
	if ch == nil || group == "" {
		return true
	}
	for _, g := range splitCSV(ch.Group) {
		if g == group {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			for len(part) > 0 && part[0] == ' ' {
				part = part[1:]
			}
			for len(part) > 0 && part[len(part)-1] == ' ' {
				part = part[:len(part)-1]
			}
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

// PickWeighted selects index by weights (same smoothing as official when needed).
func PickWeighted(weights []int) int {
	if len(weights) == 0 {
		return -1
	}
	sum := 0
	for _, w := range weights {
		sum += w
	}
	smoothingFactor := 1
	smoothingAdjustment := 0
	if sum == 0 {
		sum = len(weights) * 100
		smoothingAdjustment = 100
	} else if sum/len(weights) < 10 {
		smoothingFactor = 100
	}
	total := sum * smoothingFactor
	r := rand.Intn(total)
	for i, w := range weights {
		r -= w*smoothingFactor + smoothingAdjustment
		if r < 0 {
			return i
		}
	}
	return len(weights) - 1
}

func decorateBandWeights(channels []*model.Channel, weights []int, modelName string, caps model.RouteCaps) []int {
	if len(channels) == 0 || len(weights) != len(channels) {
		return weights
	}
	out := append([]int(nil), weights...)
	out = applyFloorShare(channels, out, modelName)
	return out
}

func applyFloorShare(channels []*model.Channel, weights []int, modelName string) []int {
	cfg := GetConfig()
	if cfg.FloorShare <= 0 {
		return weights
	}
	var idx []int
	for i, ch := range channels {
		if ch == nil {
			continue
		}
		se := GetScore(ch.Id, modelName)
		if se != nil && se.FloorDecay > 0 {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return weights
	}
	per := cfg.FloorShare
	total := per * float64(len(idx))
	if cfg.FloorTotalMax > 0 && total > cfg.FloorTotalMax {
		per = cfg.FloorTotalMax / float64(len(idx))
	}
	sum := 0
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum < 1 {
		sum = 1
	}
	minW := int(per * float64(sum))
	if minW < 1 {
		minW = 1
	}
	for _, i := range idx {
		if weights[i] < minW {
			weights[i] = minW
		}
	}
	return weights
}
