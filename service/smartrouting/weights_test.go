package smartrouting

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func weightOf(n int) *uint {
	w := uint(n)
	return &w
}

func TestSmartWeightsOffUsesOfficialWeight(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "off"
	SetConfig(cfg)
	ch := &model.Channel{Id: 1, Weight: weightOf(10)}
	result := SmartWeights([]*model.Channel{ch}, "gpt-test", "default", model.RouteCaps{})
	if result.Skip || len(result.Channels) != 1 || len(result.Weights) != 1 || result.Weights[0] != 10 {
		t.Fatalf("off mode must keep official weight, got %+v", result)
	}
	if result.Decision.Reason != "off" {
		t.Fatalf("reason=%s", result.Decision.Reason)
	}
}

func TestSmartWeightsNil(t *testing.T) {
	result := SmartWeights(nil, "m", "g", model.RouteCaps{})
	if !result.Skip || result.Channels != nil {
		t.Fatalf("expected skip on nil input, got %+v", result)
	}
}

func TestSmartWeightsActiveDoesNotReturnExcluded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "active"
	SetConfig(cfg)
	model.ReplaceRouteStateForTest([]model.ChannelRouteTag{{
		ChannelID: 7, Model: "gpt-test", Tag: "excluded",
	}}, nil, nil)
	t.Cleanup(func() { model.ReplaceRouteStateForTest(nil, nil, nil) })
	kept := &model.Channel{Id: 8, Weight: weightOf(3)}
	dropped := &model.Channel{Id: 7, Weight: weightOf(9)}
	result := SmartWeights([]*model.Channel{dropped, kept}, "gpt-test", "default", model.RouteCaps{})
	if result.Skip || len(result.Channels) != 1 || result.Channels[0].Id != 8 {
		t.Fatalf("excluded channel must not be returned, got %+v", result)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Why != "excluded" {
		t.Fatalf("exclusion reason missing: %+v", result.Excluded)
	}
}

func TestSmartWeightsActiveEmptyBandSkips(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "active"
	SetConfig(cfg)
	model.ReplaceRouteStateForTest([]model.ChannelRouteTag{{
		ChannelID: 7, Model: "*", Tag: "excluded",
	}}, nil, nil)
	t.Cleanup(func() { model.ReplaceRouteStateForTest(nil, nil, nil) })
	result := SmartWeights([]*model.Channel{{Id: 7, Weight: weightOf(9)}}, "gpt-test", "default", model.RouteCaps{})
	if !result.Skip || len(result.Channels) != 0 {
		t.Fatalf("fully excluded band must skip, got %+v", result)
	}
}

func TestSmartWeightsObserveKeepsOfficialWeights(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = "observe"
	SetConfig(cfg)
	model.ReplaceRouteStateForTest([]model.ChannelRouteTag{{
		ChannelID: 7, Model: "gpt-test", Tag: "excluded",
	}}, nil, nil)
	t.Cleanup(func() { model.ReplaceRouteStateForTest(nil, nil, nil) })
	a := &model.Channel{Id: 7, Weight: weightOf(4)}
	b := &model.Channel{Id: 8, Weight: weightOf(6)}
	result := SmartWeights([]*model.Channel{a, b}, "gpt-test", "default", model.RouteCaps{})
	if result.Skip || len(result.Weights) != 2 || result.Weights[0] != 4 || result.Weights[1] != 6 {
		t.Fatalf("observe must dispatch official weights, got %+v", result)
	}
	if result.Decision.WouldPick != 8 {
		t.Fatalf("shadow pick=%d", result.Decision.WouldPick)
	}
}
