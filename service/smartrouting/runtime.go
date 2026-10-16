package smartrouting

import (
	"context"
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

func scoreLoop() {
	for {
		cfg := GetConfig()
		sec := cfg.ScoreRefreshSec
		if sec <= 0 {
			sec = 300
		}
		time.Sleep(time.Duration(sec) * time.Second)
		LoadConfigFromOptions()
		if !GetConfig().Enabled {
			continue
		}
		if !acquireScoreLock(sec) {
			continue
		}
		func() {
			defer releaseScoreLock()
			RefreshAllScores()
		}()
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

// SimulateSelection returns share preview for admin POST /simulate.
func SimulateSelection(modelName, group string, channels []*model.Channel) map[string]any {
	out, weights := SmartWeights(channels, modelName, group)
	sum := 0
	for _, w := range weights {
		sum += w
	}
	rows := make([]map[string]any, 0, len(out))
	for i, ch := range out {
		share := 0.0
		if sum > 0 {
			share = float64(weights[i]) / float64(sum)
		}
		se := GetScore(ch.Id, modelName)
		row := map[string]any{
			"channel_id":   ch.Id,
			"name":         ch.Name,
			"route_weight": weights[i],
			"share":        share,
			"official_w":   ch.GetWeight(),
		}
		if se != nil {
			row["score"] = se.Score
			row["state"] = se.State
		}
		if IsHalfOpenChannel(ch.Id) {
			row["halfopen"] = true
		}
		rows = append(rows, row)
	}
	return map[string]any{"model": modelName, "group": group, "candidates": rows}
}

// OfficialWeightsOnly for rollback to official.
func OfficialWeightsOnly(channels []*model.Channel) ([]*model.Channel, []int) {
	w := make([]int, len(channels))
	for i, ch := range channels {
		w[i] = ch.GetWeight()
	}
	return channels, w
}
