package model

import "testing"

func TestHigherHalfOpenDoesNotBeatHealthyBand(t *testing.T) {
	high := &Channel{Id: 11, Name: "probe", Status: 1}
	low := &Channel{Id: 22, Name: "healthy", Status: 1}
	prevProbe := OnHalfOpenProbeHook
	prevWeight := SmartWeightHook
	probed := 0
	OnHalfOpenProbeHook = func(id int) bool {
		probed = id
		return true
	}
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		if len(channels) == 1 && channels[0].Id == high.Id {
			return SmartWeightResult{Skip: true, HalfOpen: high, Decision: RouteBandDecision{Reason: "skip"}}
		}
		return SmartWeightResult{Channels: channels, Weights: []int{1}, Decision: RouteBandDecision{Reason: "picked"}}
	}
	t.Cleanup(func() {
		OnHalfOpenProbeHook = prevProbe
		SmartWeightHook = prevWeight
	})

	ch, decision, err := SelectFromPriorityBands([]PriorityBand{
		{Priority: 10, Channels: []*Channel{high}},
		{Priority: 1, Channels: []*Channel{low}},
	}, 0, "m", "g", RouteCaps{})
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil || ch.Id != low.Id || decision.HalfOpen {
		t.Fatalf("a later healthy band must win over a higher half-open, id=%v half=%v reason=%s", channelID(ch), decision.HalfOpen, decision.Reason)
	}
	if probed != 0 {
		t.Fatalf("healthy band must not reserve the higher probe, id=%d", probed)
	}
}

func TestHealthyBandWinsWhenProbeSlotIsFull(t *testing.T) {
	probe := &Channel{Id: 11, Name: "probe", Status: 1}
	healthy := &Channel{Id: 22, Name: "healthy", Status: 1}
	prevProbe := OnHalfOpenProbeHook
	prevWeight := SmartWeightHook
	OnHalfOpenProbeHook = func(int) bool { return false }
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		return SmartWeightResult{
			Channels: []*Channel{healthy},
			Weights:  []int{1},
			HalfOpen: probe,
			Decision: RouteBandDecision{Reason: "score"},
		}
	}
	t.Cleanup(func() {
		OnHalfOpenProbeHook = prevProbe
		SmartWeightHook = prevWeight
	})

	ch, decision, err := SelectFromPriorityBands([]PriorityBand{
		{Priority: 10, Channels: []*Channel{probe, healthy}},
	}, 0, "m", "g", RouteCaps{})
	if err != nil || ch == nil || ch.Id != healthy.Id || decision.HalfOpen {
		t.Fatalf("full probe slot must fall back to the healthy peer, id=%v half=%v err=%v", channelID(ch), decision.HalfOpen, err)
	}
}

func TestSameBandHealthyPeerWinsOverProbe(t *testing.T) {
	probe := &Channel{Id: 11, Name: "probe", Status: 1}
	healthy := &Channel{Id: 22, Name: "healthy", Status: 1}
	prevProbe := OnHalfOpenProbeHook
	prevWeight := SmartWeightHook
	OnHalfOpenProbeHook = func(int) bool { return true }
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		return SmartWeightResult{
			Channels: []*Channel{healthy},
			Weights:  []int{1},
			HalfOpen: probe,
			Decision: RouteBandDecision{Reason: "score"},
		}
	}
	t.Cleanup(func() {
		OnHalfOpenProbeHook = prevProbe
		SmartWeightHook = prevWeight
	})

	ch, decision, err := SelectFromPriorityBands([]PriorityBand{
		{Priority: 10, Channels: []*Channel{probe, healthy}},
	}, 0, "m", "g", RouteCaps{})
	if err != nil || ch == nil || ch.Id != healthy.Id || decision.HalfOpen {
		t.Fatalf("same-band healthy peer must win over a probe, id=%v reason=%s err=%v", channelID(ch), decision.Reason, err)
	}
}

func TestExhaustedBandUsesHalfOpen(t *testing.T) {
	probe := &Channel{Id: 11, Name: "probe", Status: 1}
	prevProbe := OnHalfOpenProbeHook
	prevWeight := SmartWeightHook
	OnHalfOpenProbeHook = func(id int) bool { return id == probe.Id }
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		return SmartWeightResult{Skip: true, HalfOpen: probe, Decision: RouteBandDecision{Reason: "skip"}}
	}
	t.Cleanup(func() {
		OnHalfOpenProbeHook = prevProbe
		SmartWeightHook = prevWeight
	})
	ch, decision, err := SelectFromPriorityBands([]PriorityBand{{Priority: 10, Channels: []*Channel{probe}}}, 0, "m", "g", RouteCaps{})
	if err != nil || ch == nil || ch.Id != probe.Id || decision.Reason != "halfopen" {
		t.Fatalf("exhausted band must use the half-open probe, id=%v reason=%s err=%v", channelID(ch), decision.Reason, err)
	}
}

func channelID(ch *Channel) int {
	if ch == nil {
		return 0
	}
	return ch.Id
}
