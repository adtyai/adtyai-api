package smartrouting

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func TestSmartWeightsKeepsHalfOpenBesideHealthyPeer(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	cfg := GetConfig()
	prev := cfg
	cfg.Mode = "active"
	cfg.ScoreRefreshSec = 1
	SetConfig(cfg)
	t.Cleanup(func() { SetConfig(prev) })
	healthy := &model.Channel{Id: 71, Name: "ok", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "g", Models: "m", Priority: common.GetPointer(int64(10))}
	probe := &model.Channel{Id: 72, Name: "probe", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "g", Models: "m", Priority: common.GetPointer(int64(10))}
	restore := model.InstallChannelCacheForTest([]*model.Channel{healthy, probe})
	t.Cleanup(restore)
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: probe.Id, Model: "m"}})
	noteBreakerStage(probe.Id, "half_2")
	t.Cleanup(func() { noteBreakerStage(probe.Id, "") })
	storeScores(map[string]*ScoreEntry{
		scoreKey(healthy.Id, "m"): {ChannelID: healthy.Id, Model: "m", Score: 80, RouteWeight: 10, State: "normal", UpdatedAt: time.Now()},
	})
	t.Cleanup(func() { storeScores(map[string]*ScoreEntry{}) })

	got := SmartWeights([]*model.Channel{healthy, probe}, "m", "g", model.RouteCaps{})
	if got.Skip || len(got.Channels) != 1 || got.Channels[0].Id != healthy.Id {
		t.Fatalf("healthy peer must still dispatch, skip=%v channels=%v", got.Skip, channelIDs(got.Channels))
	}
	if got.HalfOpen == nil || got.HalfOpen.Id != probe.Id {
		t.Fatalf("active band must keep the half-open probe beside healthy peers, got %#v", got.HalfOpen)
	}
}

func TestSimulateBandsKeepsAbilityBandAndHalfOpen(t *testing.T) {
	low := &model.Channel{Id: 22, Name: "low-channel-priority", Status: 1, Priority: common.GetPointer(int64(1))}
	highChannelPriority := &model.Channel{Id: 33, Name: "high-channel-priority", Status: 1, Priority: common.GetPointer(int64(5))}
	probe := &model.Channel{Id: 11, Name: "probe", Status: 1, Priority: common.GetPointer(int64(1))}
	prev := model.SmartWeightHook
	model.SmartWeightHook = func(channels []*model.Channel, modelName, group string, caps model.RouteCaps) model.SmartWeightResult {
		if len(channels) == 1 && channels[0].Id == probe.Id {
			return model.SmartWeightResult{Skip: true, HalfOpen: probe, Decision: model.RouteBandDecision{Mode: "active", Reason: "skip"}}
		}
		return model.SmartWeightResult{Channels: channels, Weights: []int{1, 1}, Decision: model.RouteBandDecision{Mode: "active", Reason: "score"}}
	}
	t.Cleanup(func() { model.SmartWeightHook = prev })

	out := SimulateBands("m", "g", []model.PriorityBand{
		{Priority: 20, Channels: []*model.Channel{probe}},
		{Priority: 10, Channels: []*model.Channel{low, highChannelPriority}},
	}, model.RouteCaps{})
	if out["priority"] != int64(10) {
		t.Fatalf("preview must use the ability band, priority=%v", out["priority"])
	}
	if _, ok := out["half_open"]; ok {
		t.Fatalf("preview must not advertise a probe the selector would not pick, got %#v", out["half_open"])
	}
	picked, _ := out["picked"].(int)
	if picked != low.Id && picked != highChannelPriority.Id {
		t.Fatalf("preview pick=%v", picked)
	}
	items, _ := out["items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("preview must not split an ability band by channel priority, items=%v", items)
	}
	seen := map[int]bool{}
	for _, item := range items {
		seen[item["channel_id"].(int)] = true
		if item["priority"] != int64(10) {
			t.Fatalf("item priority=%v", item["priority"])
		}
	}
	if !seen[low.Id] || !seen[highChannelPriority.Id] {
		t.Fatalf("items=%v", items)
	}
}

func channelIDs(channels []*model.Channel) []int {
	out := make([]int, 0, len(channels))
	for _, ch := range channels {
		if ch != nil {
			out = append(out, ch.Id)
		}
	}
	return out
}
