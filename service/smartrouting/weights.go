package smartrouting

import (
	"math"
	"math/rand"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// SmartWeights overlays route_weight on one priority band.
// Does NOT mutate channels.weight. Injects half-open candidates when allowed (H2).
func SmartWeights(channels []*model.Channel, modelName, group string) ([]*model.Channel, []int) {
	cfg := GetConfig()
	n := len(channels)
	if n == 0 {
		return channels, nil
	}
	official := make([]int, n)
	for i, ch := range channels {
		official[i] = ch.GetWeight()
	}
	if !cfg.Enabled {
		return channels, official
	}
	// traffic gray
	if cfg.ScoreTrafficPct < 100 {
		if rand.Intn(100) >= cfg.ScoreTrafficPct {
			return channels, official
		}
	}

	// stale check: if no scores updated recently → official
	scores := GetAllScores()
	stale := true
	cutoff := time.Now().Add(-time.Duration(cfg.ScoreStaleSec) * time.Second)
	for _, s := range scores {
		if s != nil && s.UpdatedAt.After(cutoff) {
			stale = false
			break
		}
	}
	if stale && len(scores) > 0 {
		return channels, official
	}

	type cand struct {
		ch     *model.Channel
		rw     int
		score  float64
		state  string
		floor  bool
		half   bool
		stage  string
		locked bool
		note   string
	}
	var normal []cand
	var floors []cand
	var excluded []RouteExcluded

	for _, ch := range channels {
		if ch == nil {
			continue
		}
		// tags
		if model.HasRouteTag(ch.Id, modelName, "excluded") || model.HasRouteTag(ch.Id, modelName, "fake_model") {
			excluded = append(excluded, RouteExcluded{ID: ch.Id, Why: "excluded"})
			continue
		}
		if IsCostGuardExcluded(ch.Id) {
			excluded = append(excluded, RouteExcluded{ID: ch.Id, Why: "cost_guard"})
			continue
		}
		if InRateLimitCooldown(ch.Id) {
			excluded = append(excluded, RouteExcluded{ID: ch.Id, Why: "rpm_cooldown"})
			continue
		}
		if RPMExceeded(ch.Id) {
			excluded = append(excluded, RouteExcluded{ID: ch.Id, Why: "rpm_full"})
			continue
		}
		// fallback_only deferred
		fallbackOnly := model.HasRouteTag(ch.Id, modelName, "fallback_only")
		locked := model.HasRouteTag(ch.Id, modelName, "locked")
		se := GetScore(ch.Id, modelName)
		rw := ch.GetWeight()
		score := 70.0
		state := "normal"
		isFloor := false
		if se != nil {
			rw = se.RouteWeight
			score = se.Score
			state = se.State
			isFloor = se.Insufficient || se.FloorDecay > 0
			if se.FloorDecay > 0 && !se.Insufficient {
				// decaying floor: still mark for floor share fraction
				isFloor = true
			}
		}
		if locked {
			rw = 100
			state = "locked"
			isFloor = false
		}
		if fallbackOnly {
			// keep aside; added only if no other normal
			floors = append(floors, cand{ch: ch, rw: rw, score: score, state: "fallback_only", note: "fallback_only"})
			continue
		}
		c := cand{ch: ch, rw: rw, score: score, state: state, floor: isFloor, locked: locked}
		if isFloor && !locked {
			floors = append(floors, c)
		} else {
			normal = append(normal, c)
		}
	}

	// half-open injection (status=4 cooling done)
	halfList := ListHalfOpenReady()
	var halfCands []cand
	for _, h := range halfList {
		if h.Model != "" && h.Model != "*" && h.Model != modelName {
			continue
		}
		ch, err := model.CacheGetChannel(h.ChannelID)
		if err != nil || ch == nil {
			ch2, err2 := model.GetChannelById(h.ChannelID, true)
			if err2 != nil {
				continue
			}
			ch = ch2
		}
		// must belong to same priority band as current channels
		if n > 0 && ch.GetPriority() != channels[0].GetPriority() {
			continue
		}
		if !channelServesModel(ch, modelName) {
			continue
		}
		if !channelInGroup(ch, group) {
			continue
		}
		if HalfOpenInflightFull(h.ChannelID, cfg.HalfOpenMaxInflight) {
			continue
		}
		share := cfg.HalfOpenShareStage[0]
		if h.Stage == "half_10" {
			share = cfg.HalfOpenShareStage[1]
		}
		halfCands = append(halfCands, cand{
			ch: ch, rw: 1, score: 70, state: "halfopen", half: true, stage: h.Stage,
			note: "halfopen:" + h.Stage + ":" + common.Interface2String(share),
		})
	}

	// H2: if no normal/degraded/locked, only half-open → skip band (return empty → caller tries next priority)
	if len(normal) == 0 && len(floors) == 0 && len(halfCands) > 0 {
		return nil, nil
	}
	if len(normal) == 0 && len(floors) == 0 && len(halfCands) == 0 {
		return channels, official
	}

	// build weight list
	var out []*model.Channel
	var weights []int

	// compute normal weight sum
	sumRW := 0
	for _, c := range normal {
		if c.rw < 1 {
			c.rw = 1
		}
		sumRW += c.rw
	}
	if sumRW == 0 && len(normal) > 0 {
		sumRW = len(normal) * 100
		for i := range normal {
			normal[i].rw = 100
		}
	}

	// floor share
	floorN := 0
	for _, c := range floors {
		if c.state != "fallback_only" {
			floorN++
		}
	}
	floorShareEach := 0.0
	if floorN > 0 {
		totalFloor := cfg.FloorShare * float64(floorN)
		if totalFloor > cfg.FloorTotalMax {
			floorShareEach = cfg.FloorTotalMax / float64(floorN)
		} else {
			floorShareEach = cfg.FloorShare
		}
		// decay: FloorDecay remaining cycles → fraction FloorDecay/FloorDecayCycles
		// applied per-cand below
	}

	for _, c := range normal {
		out = append(out, c.ch)
		weights = append(weights, c.rw)
	}
	// redistribute: treat floor as share of total
	// effective: normal keeps sumRW representing (1 - floorTotal); floor get share
	floorTotal := floorShareEach * float64(floorN)
	if floorTotal > 0 && sumRW > 0 {
		// scale normal to (1-floorTotal)
		scale := 1.0 - floorTotal
		if scale < 0.1 {
			scale = 0.1
		}
		for i := range weights {
			weights[i] = int(math.Max(1, math.Round(float64(weights[i])*scale)))
		}
		base := float64(sumRW)
		for _, c := range floors {
			if c.state == "fallback_only" {
				continue
			}
			frac := floorShareEach
			if se := GetScore(c.ch.Id, modelName); se != nil && se.FloorDecay > 0 && cfg.FloorDecayCycles > 0 {
				frac = floorShareEach * float64(se.FloorDecay) / float64(cfg.FloorDecayCycles)
			}
			w := int(math.Max(1, math.Round(base*frac)))
			out = append(out, c.ch)
			weights = append(weights, w)
		}
	} else {
		for _, c := range floors {
			if c.state == "fallback_only" {
				continue
			}
			out = append(out, c.ch)
			weights = append(weights, int(math.Max(1, float64(c.rw))))
		}
	}

	// half-open share relative to normal sum
	normSum := 0
	for _, w := range weights {
		normSum += w
	}
	for _, c := range halfCands {
		share := cfg.HalfOpenShareStage[0]
		if c.stage == "half_10" {
			share = cfg.HalfOpenShareStage[1]
		}
		w := 1
		if normSum > 0 {
			w = int(math.Max(1, math.Round(float64(normSum)*share/(1-share))))
		}
		out = append(out, c.ch)
		weights = append(weights, w)
	}

	// if only fallback_only left
	if len(out) == 0 {
		for _, c := range floors {
			if c.state == "fallback_only" {
				out = append(out, c.ch)
				weights = append(weights, 100)
			}
		}
	}

	// ε-greedy exploration (default off)
	if cfg.EpsilonGreedyEnabled && len(out) > 1 && rand.Float64() < cfg.Epsilon {
		// uniform
		for i := range weights {
			weights[i] = 100
		}
	}

	_ = excluded // reserved for decision log attachment by caller
	return out, weights
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
			// trim spaces
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
