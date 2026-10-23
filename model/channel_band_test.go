package model

import (
	"testing"
)

func bandWeight(n int) *uint {
	w := uint(n)
	return &w
}

func bandPriority(n int64) *int64 {
	p := n
	return &p
}

func TestSelectFromPriorityBandsAdvancesInsideSelector(t *testing.T) {
	prev := SmartWeightHook
	t.Cleanup(func() { SmartWeightHook = prev })
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		if len(channels) == 1 && channels[0].Id == 1 {
			return SmartWeightResult{Skip: true, Decision: RouteBandDecision{Reason: "skip"}}
		}
		return SmartWeightResult{Channels: channels, Weights: []int{1}, Decision: RouteBandDecision{Reason: "score"}}
	}
	high := &Channel{Id: 1, Priority: bandPriority(10), Weight: bandWeight(1)}
	low := &Channel{Id: 2, Priority: bandPriority(1), Weight: bandWeight(1)}
	ch, decision, err := SelectFromPriorityBands([]PriorityBand{
		{Priority: 10, Channels: []*Channel{high}},
		{Priority: 1, Channels: []*Channel{low}},
	}, 0, "m", "default", RouteCaps{})
	if err != nil || ch == nil || ch.Id != 2 || !decision.Advanced {
		t.Fatalf("internal walk should pick the lower band without a retry bump, ch=%v decision=%+v err=%v", ch, decision, err)
	}
	if decision.Group != "default" || decision.BandIndex != 1 {
		t.Fatalf("picked band must be recorded, got %+v", decision)
	}
	if NextBandStart(decision, "default", 0) != 0 {
		t.Fatal("same-group retry must use the official retry index")
	}
	if NextBandStart(decision, "other", 2) != 2 {
		t.Fatal("a group change must use the official retry")
	}
	if NextBandStart(RouteBandDecision{}, "default", 2) != 2 {
		t.Fatal("an empty decision must use the official retry")
	}
	exhausted := RouteBandDecision{Reason: "exhausted", Group: "default"}
	if NextBandStart(exhausted, "default", 2) != 2 {
		t.Fatal("a decision with no pick must use the official retry")
	}
}

func TestSelectFromPriorityBandsPassesBandPriority(t *testing.T) {
	prev := SmartWeightHook
	t.Cleanup(func() { SmartWeightHook = prev })
	var seen RouteCaps
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		seen = caps
		return SmartWeightResult{Skip: true, Decision: RouteBandDecision{Reason: "skip"}}
	}
	_, _, err := SelectFromPriorityBands([]PriorityBand{{
		Priority: 10,
		Channels: []*Channel{{Id: 1, Weight: bandWeight(1)}},
	}}, 0, "m", "g", RouteCaps{})
	if err != nil {
		t.Fatal(err)
	}
	if !seen.HasBandPriority || seen.BandPriority != 10 {
		t.Fatalf("weight hook must see the walked band priority, got %+v", seen)
	}
}

func TestRememberedHalfOpenConfirmFailureIsInflight(t *testing.T) {
	prevHook, prevSelected := SmartWeightHook, OnChannelSelectedHook
	t.Cleanup(func() {
		SmartWeightHook = prevHook
		OnChannelSelectedHook = prevSelected
	})
	probe := &Channel{Id: 9}
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		return SmartWeightResult{Skip: true, HalfOpen: probe, Decision: RouteBandDecision{Reason: "skip"}}
	}
	OnChannelSelectedHook = func(channelID int) bool { return false }
	channel, decision, err := SelectFromPriorityBands([]PriorityBand{{
		Priority: 10,
		Channels: []*Channel{{Id: 1, Weight: bandWeight(1)}},
	}}, 0, "m", "g", RouteCaps{})
	if channel != nil || err != nil {
		t.Fatalf("confirm failure must be an empty result so auto-group can continue, channel=%v err=%v", channel, err)
	}
	if decision.Reason != "inflight_rejected" || decision.Picked != 0 || decision.Group != "g" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestSelectFromPriorityBandsInflightWalksToLowerBand(t *testing.T) {
	prevHook, prevSel := SmartWeightHook, OnChannelSelectedHook
	t.Cleanup(func() {
		SmartWeightHook, OnChannelSelectedHook = prevHook, prevSel
	})
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		return SmartWeightResult{Channels: channels, Weights: []int{1}}
	}
	OnChannelSelectedHook = func(id int) bool { return id != 1 }
	high := &Channel{Id: 1, Priority: bandPriority(10), Weight: bandWeight(1)}
	low := &Channel{Id: 2, Priority: bandPriority(1), Weight: bandWeight(1)}
	ch, decision, err := SelectFromPriorityBands([]PriorityBand{
		{Priority: 10, Channels: []*Channel{high}},
		{Priority: 1, Channels: []*Channel{low}},
	}, 0, "m", "default", RouteCaps{})
	if err != nil || ch == nil || ch.Id != 2 || !decision.Advanced || decision.Reason == "inflight_rejected" {
		t.Fatalf("inflight on a higher band must walk down in the same attempt, ch=%v decision=%+v err=%v", ch, decision, err)
	}
}

func TestSelectFromPriorityBandsAllInflightIsEmpty(t *testing.T) {
	prevHook, prevSel := SmartWeightHook, OnChannelSelectedHook
	t.Cleanup(func() {
		SmartWeightHook, OnChannelSelectedHook = prevHook, prevSel
	})
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		return SmartWeightResult{Channels: channels, Weights: []int{1}}
	}
	OnChannelSelectedHook = func(int) bool { return false }
	high := &Channel{Id: 1, Priority: bandPriority(10), Weight: bandWeight(1)}
	low := &Channel{Id: 2, Priority: bandPriority(1), Weight: bandWeight(1)}
	ch, decision, err := SelectFromPriorityBands([]PriorityBand{
		{Priority: 10, Channels: []*Channel{high}},
		{Priority: 1, Channels: []*Channel{low}},
	}, 0, "m", "default", RouteCaps{})
	if ch != nil || err != nil || decision.Reason != "inflight_rejected" || decision.Picked != 0 || decision.Group != "default" {
		t.Fatalf("every band inflight-full must be an empty rejection, ch=%v decision=%+v err=%v", ch, decision, err)
	}
}

func TestSelectFromPriorityBandsHalfOpenKeepsSourceBand(t *testing.T) {
	prev := SmartWeightHook
	t.Cleanup(func() { SmartWeightHook = prev })
	probe := &Channel{Id: 9, Priority: bandPriority(5), Weight: bandWeight(1)}
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		if len(channels) == 1 && channels[0] != nil && channels[0].Id == 2 {
			return SmartWeightResult{Skip: true, HalfOpen: probe, Decision: RouteBandDecision{Reason: "skip"}}
		}
		return SmartWeightResult{Skip: true, Decision: RouteBandDecision{Reason: "skip"}}
	}
	bands := []PriorityBand{
		{Priority: 10, Channels: []*Channel{{Id: 1, Priority: bandPriority(10), Weight: bandWeight(1)}}},
		{Priority: 5, Channels: []*Channel{{Id: 2, Priority: bandPriority(5), Weight: bandWeight(1)}}},
		{Priority: 1, Channels: []*Channel{{Id: 3, Priority: bandPriority(1), Weight: bandWeight(1)}}},
	}
	ch, decision, err := SelectFromPriorityBands(bands, 0, "m", "g", RouteCaps{})
	if err != nil || ch == nil || ch.Id != 9 || !decision.HalfOpen || decision.BandIndex != 1 || decision.Group != "g" {
		t.Fatalf("half-open must keep the source band, ch=%v decision=%+v err=%v", ch, decision, err)
	}
	if NextBandStart(decision, "g", 0) != 0 {
		t.Fatalf("next retry must use the official retry index, got %d", NextBandStart(decision, "g", 0))
	}
}

func TestDryRunHalfOpenDoesNotReserve(t *testing.T) {
	prev := SmartWeightHook
	prevProbe := OnHalfOpenProbeHook
	t.Cleanup(func() {
		SmartWeightHook = prev
		OnHalfOpenProbeHook = prevProbe
	})
	probe := &Channel{Id: 9}
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		return SmartWeightResult{Skip: true, HalfOpen: probe, Decision: RouteBandDecision{Reason: "skip"}}
	}
	OnHalfOpenProbeHook = func(int) bool {
		t.Fatal("preview must not reserve a probe")
		return false
	}
	ch, decision, err := SelectFromPriorityBands([]PriorityBand{{
		Priority: 10,
		Channels: []*Channel{{Id: 1}},
	}}, 0, "m", "g", RouteCaps{DryRun: true})
	if err != nil || ch == nil || ch.Id != 9 || !decision.HalfOpen {
		t.Fatalf("dry-run must return the probe without reserving, ch=%v decision=%+v err=%v", ch, decision, err)
	}
}
