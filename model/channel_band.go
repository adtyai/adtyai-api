package model

import (
	"math/rand"
)

// PriorityBand is one official priority group. Callers pass bands highest-first.
type PriorityBand struct {
	Priority int64
	Channels []*Channel
}

// SelectFromPriorityBands walks from the official retry index down to the first band that can dispatch.
// Walking down does not change the caller's retry counter.
// A half-open channel is returned only after every band has no normal candidate.
// DryRun returns that same pick without reserving inflight.
func SelectFromPriorityBands(bands []PriorityBand, start int, modelName, group string, caps RouteCaps) (*Channel, RouteBandDecision, error) {
	if len(bands) == 0 {
		return nil, RouteBandDecision{Reason: "empty", Group: group}, nil
	}
	if start < 0 {
		start = 0
	}
	if start >= len(bands) {
		start = len(bands) - 1
	}
	var remembered *Channel
	rememberedIndex := 0
	var last RouteBandDecision
	rejected := false
	for i := start; i < len(bands); i++ {
		band := bands[i]
		bandCaps := caps
		bandCaps.BandPriority = band.Priority
		bandCaps.HasBandPriority = true
		result := ApplySmartWeights(band.Channels, modelName, group, bandCaps)
		result.Decision.Priority = band.Priority
		if result.HalfOpen != nil && remembered == nil {
			remembered = result.HalfOpen
			rememberedIndex = i
		}
		if result.Skip || len(result.Channels) == 0 {
			last = result.Decision
			if last.Reason == "" {
				last.Reason = "skip"
			}
			last.Group = group
			continue
		}
		ch := weightedPick(result.Channels, result.Weights, !caps.DryRun)
		if ch == nil {
			last = result.Decision
			last.Reason = "inflight_rejected"
			last.BandIndex = i
			last.Group = group
			rejected = true
			continue
		}
		decision := result.Decision
		decision.Priority = band.Priority
		decision.Picked = ch.Id
		decision.BandIndex = i
		decision.Group = group
		decision.Advanced = i > start
		if decision.Reason == "" {
			decision.Reason = "picked"
		}
		return ch, decision, nil
	}
	if remembered != nil {
		if !caps.DryRun && !ConfirmHalfOpenProbe(remembered.Id) {
			last.Picked = 0
			last.HalfOpen = false
			last.Reason = "inflight_rejected"
			last.Group = group
			return nil, last, nil
		}
		last.Picked = remembered.Id
		last.HalfOpen = true
		last.BandIndex = rememberedIndex
		last.Group = group
		last.Advanced = rememberedIndex > start
		last.Reason = "halfopen"
		return remembered, last, nil
	}
	if rejected {
		last.Picked = 0
		last.HalfOpen = false
		last.Reason = "inflight_rejected"
		last.Group = group
		return nil, last, nil
	}
	last.Group = group
	if last.Reason == "" {
		last.Reason = "exhausted"
	}
	return nil, last, nil
}

// NextBandStart returns the official retry index.
// Internal descent does not consume RetryParam, so a later retry does not start at BandIndex+1.
func NextBandStart(prev RouteBandDecision, group string, officialRetry int) int {
	if officialRetry < 0 {
		return 0
	}
	return officialRetry
}

// weightedPick keeps the official smoothing rules. smoothingAdjustment stays 0.
func weightedPick(channels []*Channel, weights []int, reserve bool) *Channel {
	if len(channels) == 0 {
		return nil
	}
	ws := append([]int(nil), weights...)
	if len(ws) != len(channels) {
		ws = make([]int, len(channels))
		for i, ch := range channels {
			if ch != nil {
				ws[i] = ch.GetWeight()
			}
		}
	}
	sumWeight := 0
	for _, w := range ws {
		if w > 0 {
			sumWeight += w
		}
	}
	smoothingFactor := 1
	if sumWeight == 0 {
		for i := range ws {
			ws[i] = 100
		}
	} else if sumWeight/len(channels) < 10 {
		smoothingFactor = 100
	}
	remaining := append([]int(nil), ws...)
	for attempt := 0; attempt < len(channels); attempt++ {
		sum := 0
		for _, w := range remaining {
			if w > 0 {
				sum += w
			}
		}
		total := sum * smoothingFactor
		if total <= 0 {
			break
		}
		rw := rand.Intn(total)
		picked := false
		for i, channel := range channels {
			w := remaining[i]
			if w <= 0 || channel == nil {
				continue
			}
			rw -= w * smoothingFactor
			if rw < 0 {
				picked = true
				if !reserve || ConfirmChannelSelected(channel.Id) {
					return channel
				}
				remaining[i] = 0
				break
			}
		}
		if !picked {
			break
		}
	}
	return nil
}
