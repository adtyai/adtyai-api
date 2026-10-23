package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func withChannelCache(t *testing.T, chans []*Channel, group, modelName string) {
	t.Helper()
	prevIDM, prevG := channelsIDM, group2model2channels
	prevMem := common.MemoryCacheEnabled
	prevHook, prevSel, prevProbe, prevFb := SmartWeightHook, OnChannelSelectedHook, OnHalfOpenProbeHook, HalfOpenFallbackHook
	common.MemoryCacheEnabled = true
	channelsIDM = map[int]*Channel{}
	ids := []int{}
	for _, c := range chans {
		channelsIDM[c.Id] = c
		ids = append(ids, c.Id)
	}
	group2model2channels = map[string]map[string][]int{group: {modelName: ids}}
	SmartWeightHook, OnChannelSelectedHook, OnHalfOpenProbeHook, HalfOpenFallbackHook = nil, nil, nil, nil
	t.Cleanup(func() {
		channelsIDM, group2model2channels = prevIDM, prevG
		common.MemoryCacheEnabled = prevMem
		SmartWeightHook, OnChannelSelectedHook, OnHalfOpenProbeHook, HalfOpenFallbackHook = prevHook, prevSel, prevProbe, prevFb
	})
}

// 审查 N-P1-1：同优先级全部权重 0 时均匀分流，不能恒选第一个渠道。
func TestRandomChannelZeroWeightsUniform(t *testing.T) {
	zero := uint(0)
	var chans []*Channel
	for i := 1; i <= 4; i++ {
		chans = append(chans, &Channel{Id: i, Weight: &zero, Status: common.ChannelStatusEnabled})
	}
	withChannelCache(t, chans, "default", "m")
	hits := map[int]int{}
	const n = 20000
	for i := 0; i < n; i++ {
		ch, err := GetRandomSatisfiedChannel("default", "m", 0, nil)
		require.NoError(t, err)
		hits[ch.Id]++
	}
	for id := 1; id <= 4; id++ {
		share := float64(hits[id]) / n
		require.InDelta(t, 0.25, share, 0.05, "channel %d share %v (%v)", id, share, hits)
	}
}

// 审查 N-P1-2：候选为空（全部熔断）时回退到半开渠道；单渠道时也经过智能路由挂钩。
func TestEmptyCandidateHalfOpenDoesNotSkipHighestBand(t *testing.T) {
	probe := &Channel{Id: 7, Weight: bandWeight(1)}
	withChannelCache(t, nil, "g", "m")
	HalfOpenFallbackHook = func(modelName, group string, caps RouteCaps, accept func(*Channel) bool) *Channel {
		return probe
	}
	channel, decision, err := GetRandomSatisfiedChannelRouted("g", "m", 0, nil, RouteCaps{})
	if err != nil || channel == nil || channel.Id != 7 {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if decision.BandIndex >= 0 || decision.Picked != 7 || decision.Group != "g" || !decision.HalfOpen {
		t.Fatalf("empty-candidate probe must not be recorded as band 0, decision=%+v", decision)
	}
	if start := NextBandStart(decision, "g", 1); start != 1 {
		t.Fatalf("next walk starts at %d, want the official retry 1", start)
	}
}

func TestHalfOpenConfirmFailureIsNotGroupExhaustion(t *testing.T) {
	withChannelCache(t, nil, "g", "m")
	HalfOpenFallbackHook = func(modelName, group string, caps RouteCaps, accept func(*Channel) bool) *Channel {
		return &Channel{Id: 7}
	}
	OnChannelSelectedHook = func(channelID int) bool { return false }
	channel, decision, err := GetRandomSatisfiedChannelRouted("g", "m", 0, nil, RouteCaps{})
	if channel != nil || err != nil {
		t.Fatalf("confirm failure must be empty so auto-group can switch, channel=%v err=%v", channel, err)
	}
	if decision.Reason != "inflight_rejected" || decision.Picked != 0 || decision.Group != "g" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestRandomChannelHalfOpenFallback(t *testing.T) {
	withChannelCache(t, nil, "default", "m")
	half := &Channel{Id: 9, Status: common.ChannelStatusScoreBroken}
	HalfOpenFallbackHook = func(modelName, group string, caps RouteCaps, accept func(*Channel) bool) *Channel { return half }
	ch, err := GetRandomSatisfiedChannel("default", "m", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, 9, ch.Id)

	// inflight 被拒时该分组没有可派发渠道，调用方按空结果换组。
	OnChannelSelectedHook = func(int) bool { return false }
	ch, err = GetRandomSatisfiedChannel("default", "m", 0, nil)
	require.NoError(t, err)
	require.Nil(t, ch)
}

func TestRandomChannelSingleCandidateGoesThroughSmartHook(t *testing.T) {
	one := uint(1)
	healthy := &Channel{Id: 1, Weight: &one, Status: common.ChannelStatusEnabled}
	withChannelCache(t, []*Channel{healthy}, "default", "m")
	half := &Channel{Id: 3, Weight: &one, Status: common.ChannelStatusScoreBroken}
	called := false
	SmartWeightHook = func(chs []*Channel, m, g string, caps RouteCaps) SmartWeightResult {
		called = true
		return SmartWeightResult{Channels: []*Channel{chs[0], half}, Weights: []int{0, 1}}
	}
	ch, err := GetRandomSatisfiedChannel("default", "m", 0, nil)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, 3, ch.Id)
}
