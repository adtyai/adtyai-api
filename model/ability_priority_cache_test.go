package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestCacheBandsUseAbilityPriority(t *testing.T) {
	high := &Channel{Id: 501, Name: "ability-high", Status: common.ChannelStatusEnabled, Models: "m", Group: "g", Priority: common.GetPointer(int64(1)), Weight: common.GetPointer(uint(1))}
	low := &Channel{Id: 502, Name: "ability-low", Status: common.ChannelStatusEnabled, Models: "m", Group: "g", Priority: common.GetPointer(int64(1)), Weight: common.GetPointer(uint(1))}
	restore := InstallChannelCacheForTest([]*Channel{high, low})
	t.Cleanup(restore)
	SetGroupModelChannelsForTest("g", "m", []int{high.Id, low.Id})
	SetAbilityPriorityForTest("g", "m", high.Id, 10)
	SetAbilityPriorityForTest("g", "m", low.Id, 1)
	var seen []int64
	prev := SmartWeightHook
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		seen = append(seen, caps.BandPriority)
		if len(seen) == 1 {
			return SmartWeightResult{Skip: true, Decision: RouteBandDecision{Reason: "skip"}}
		}
		return SmartWeightResult{Channels: channels, Weights: []int{1}, Decision: RouteBandDecision{Reason: "picked"}}
	}
	t.Cleanup(func() { SmartWeightHook = prev })

	ch, _, err := GetRandomSatisfiedChannelRouted("g", "m", 0, nil, RouteCaps{})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) < 2 || seen[0] != 10 || seen[1] != 1 {
		t.Fatalf("cache path must band by ability priority, seen=%v", seen)
	}
	if ch == nil || ch.Id != low.Id {
		t.Fatalf("second band should be the ability-low channel, got %#v", ch)
	}
}
