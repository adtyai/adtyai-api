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
	prevHook, prevSel, prevFb := SmartWeightHook, OnChannelSelectedHook, HalfOpenFallbackHook
	common.MemoryCacheEnabled = true
	channelsIDM = map[int]*Channel{}
	ids := []int{}
	for _, c := range chans {
		channelsIDM[c.Id] = c
		ids = append(ids, c.Id)
	}
	group2model2channels = map[string]map[string][]int{group: {modelName: ids}}
	SmartWeightHook, OnChannelSelectedHook, HalfOpenFallbackHook = nil, nil, nil
	t.Cleanup(func() {
		channelsIDM, group2model2channels = prevIDM, prevG
		common.MemoryCacheEnabled = prevMem
		SmartWeightHook, OnChannelSelectedHook, HalfOpenFallbackHook = prevHook, prevSel, prevFb
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
func TestRandomChannelHalfOpenFallback(t *testing.T) {
	withChannelCache(t, nil, "default", "m")
	half := &Channel{Id: 9, Status: common.ChannelStatusScoreBroken}
	HalfOpenFallbackHook = func(modelName, group string) *Channel { return half }
	ch, err := GetRandomSatisfiedChannel("default", "m", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, 9, ch.Id)

	// inflight 被拒时不返回
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
	SmartWeightHook = func(chs []*Channel, m, g string) ([]*Channel, []int) {
		called = true
		return []*Channel{chs[0], half}, []int{0, 1} // 只给半开权重，验证注入生效
	}
	ch, err := GetRandomSatisfiedChannel("default", "m", 0, nil)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, 3, ch.Id)
}
