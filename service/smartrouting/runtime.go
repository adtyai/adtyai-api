package smartrouting

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var startOnce sync.Once

// StartBackgroundLoops runs flush/score loops once.
func StartBackgroundLoops() {
	startOnce.Do(func() {
		Init()
		go scoreLoop()
	})
}

var refreshAllScores = RefreshAllScores

func scoreLoop() {
	scoreLoopUntil(nil)
}

func scoreLoopUntil(stop <-chan struct{}) {
	refreshScoresIfCollecting()
	for {
		if !waitScoreInterval(stop) {
			return
		}
		LoadConfigFromOptions()
		refreshScoresIfCollecting()
	}
}

var acquireScoreLockFn = acquireScoreLock
var loadPublishedScoresFn = loadPublishedScores

func refreshScoresIfCollecting() bool {
	if !Collects() {
		return false
	}
	cfg := GetConfig()
	sec := cfg.ScoreRefreshSec
	if sec <= 0 {
		sec = 300
	}
	if !acquireScoreLockFn(sec) {
		loadPublishedScoresFn()
		reloadBreakerStagesFn()
		return false
	}
	defer releaseScoreLock()
	refreshAllScores()
	return true
}

func waitScoreInterval(stop <-chan struct{}) bool {
	sec := GetConfig().ScoreRefreshSec
	if sec <= 0 {
		sec = 300
	}
	timer := time.NewTimer(time.Duration(sec) * time.Second)
	defer timer.Stop()
	select {
	case <-stop:
		return false
	case <-timer.C:
		return true
	}
}

func acquireScoreLock(ttlSec int) bool {
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		ok, err := common.RDB.SetNX(ctx, "sr:lock:score", "1", time.Duration(ttlSec)*time.Second).Result()
		return err == nil && ok
	}
	return true
}

func releaseScoreLock() {
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		common.RDB.Del(ctx, "sr:lock:score")
	}
}

// SimulateSelection previews the same priority walk and weight hook used by selection.
// It does not reserve inflight or half-open slots.
func SimulateSelection(modelName, group string, channels []*model.Channel, caps model.RouteCaps) map[string]any {
	bands := map[int64][]*model.Channel{}
	prios := make([]int64, 0)
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		p := ch.GetPriority()
		if _, ok := bands[p]; !ok {
			prios = append(prios, p)
		}
		bands[p] = append(bands[p], ch)
	}
	sort.Slice(prios, func(i, j int) bool { return prios[i] > prios[j] })
	var used int64
	var result model.SmartWeightResult
	found := false
	for _, p := range prios {
		result = model.ApplySmartWeights(bands[p], modelName, group, caps)
		result.Decision.Priority = p
		if result.Skip || len(result.Channels) == 0 {
			continue
		}
		used = p
		found = true
		break
	}
	items := make([]map[string]any, 0)
	if found {
		shares := smoothedSharePct(result.Weights)
		for i, ch := range result.Channels {
			if ch == nil {
				continue
			}
			item := map[string]any{
				"channel_id":   ch.Id,
				"channel_name": ch.Name,
				"name":         ch.Name,
				"route_weight": 0,
				"share_pct":    0.0,
				"share":        0.0,
				"official_w":   ch.GetWeight(),
				"priority":     used,
				"state":        result.Decision.Mode,
			}
			if i < len(result.Weights) {
				item["route_weight"] = result.Weights[i]
			}
			if i < len(shares) {
				item["share_pct"] = shares[i]
				item["share"] = shares[i] / 100
			}
			if se := GetScore(ch.Id, modelName); se != nil {
				item["score"] = se.Score
				item["state"] = se.State
			}
			items = append(items, item)
		}
	}
	out := map[string]any{
		"model":      modelName,
		"group":      group,
		"priority":   used,
		"mode":       result.Decision.Mode,
		"reason":     result.Decision.Reason,
		"would_pick": result.Decision.WouldPick,
		"excluded":   result.Excluded,
		"items":      items,
		"candidates": items,
	}
	return out
}

// SimulateBands dry-runs the live selector without reserving inflight.
func SimulateBands(modelName, group string, bands []model.PriorityBand, caps model.RouteCaps) map[string]any {
	preview := caps
	preview.DryRun = true
	ch, decision, err := model.SelectFromPriorityBands(bands, 0, modelName, group, preview)
	items := make([]map[string]any, 0)
	used := int64(0)
	mode := decision.Mode
	excluded := decision.Excluded
	would := decision.WouldPick
	if decision.BandIndex >= 0 && decision.BandIndex < len(bands) {
		band := bands[decision.BandIndex]
		used = band.Priority
		bandCaps := preview
		bandCaps.BandPriority = band.Priority
		bandCaps.HasBandPriority = true
		result := model.ApplySmartWeights(band.Channels, modelName, group, bandCaps)
		if result.Decision.Mode != "" {
			mode = result.Decision.Mode
		}
		if would == 0 {
			would = result.Decision.WouldPick
		}
		if len(excluded) == 0 {
			excluded = result.Excluded
		}
		shares := smoothedSharePct(result.Weights)
		for i, itemCh := range result.Channels {
			if itemCh == nil {
				continue
			}
			item := map[string]any{
				"channel_id":   itemCh.Id,
				"channel_name": itemCh.Name,
				"name":         itemCh.Name,
				"route_weight": 0,
				"share_pct":    0.0,
				"share":        0.0,
				"official_w":   itemCh.GetWeight(),
				"priority":     used,
				"state":        mode,
			}
			if i < len(result.Weights) {
				item["route_weight"] = result.Weights[i]
			}
			if i < len(shares) {
				item["share_pct"] = shares[i]
				item["share"] = shares[i] / 100
			}
			if se := GetScore(itemCh.Id, modelName); se != nil {
				item["score"] = se.Score
				item["state"] = se.State
			}
			items = append(items, item)
		}
	}
	reason := decision.Reason
	if ch == nil && reason == "" {
		reason = "empty"
	}
	if err != nil {
		reason = "error"
	}
	picked := 0
	if ch != nil {
		picked = ch.Id
	}
	out := map[string]any{
		"model":      modelName,
		"group":      group,
		"priority":   used,
		"mode":       mode,
		"reason":     reason,
		"picked":     picked,
		"would_pick": would,
		"excluded":   excluded,
		"items":      items,
		"candidates": items,
	}
	if decision.HalfOpen && ch != nil {
		out["half_open"] = map[string]any{
			"channel_id": ch.Id,
			"priority":   used,
			"band_index": decision.BandIndex,
		}
	}
	return out
}

func smoothedSharePct(weights []int) []float64 {
	n := len(weights)
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	sum := 0
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum == 0 {
		each := 100.0 / float64(n)
		for i := range out {
			out[i] = each
		}
		return out
	}
	for i, w := range weights {
		if w > 0 {
			out[i] = float64(w) / float64(sum) * 100
		}
	}
	return out
}

// OfficialWeightsOnly for rollback to official.
func OfficialWeightsOnly(channels []*model.Channel) ([]*model.Channel, []int) {
	w := make([]int, len(channels))
	for i, ch := range channels {
		w[i] = ch.GetWeight()
	}
	return channels, w
}
