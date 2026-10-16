package smartrouting

import (
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var (
	scoreMap atomic.Value // map[string]*ScoreEntry key=channelID|model
	scoreMu  sync.Mutex
)

func init() {
	scoreMap.Store(map[string]*ScoreEntry{})
}

func scoreKey(channelID int, modelName string) string {
	return common.Interface2String(channelID) + "|" + modelName
}

func GetScore(channelID int, modelName string) *ScoreEntry {
	m, _ := scoreMap.Load().(map[string]*ScoreEntry)
	return m[scoreKey(channelID, modelName)]
}

func GetAllScores() map[string]*ScoreEntry {
	m, _ := scoreMap.Load().(map[string]*ScoreEntry)
	out := make(map[string]*ScoreEntry, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func storeScores(m map[string]*ScoreEntry) {
	scoreMap.Store(m)
}

// ComputeConnScore dual-window EWMA connectivity 0-100.
func ComputeConnScore(channelID int, modelName string, cfg Config) (score float64, samples int, insufficient bool) {
	shortOK, shortFail, _, _, tokS, _, ufailS := ReadWindow(channelID, modelName, cfg.EWMAShortMin)
	longOK, longFail, _, _, tokL, _, ufailL := ReadWindow(channelID, modelName, cfg.EWMALongMin)
	shortOK, shortFail, _ = ApplySampleCap(shortOK, shortFail, tokS, cfg.SampleCapPerToken)
	longOK, longFail, _ = ApplySampleCap(longOK, longFail, tokL, cfg.SampleCapPerToken)
	shortFail = ApplyUserFailMeltCap(shortFail, ufailS, cfg.UserFailMeltCap)
	longFail = ApplyUserFailMeltCap(longFail, ufailL, cfg.UserFailMeltCap)
	shortTot := shortOK + shortFail
	longTot := longOK + longFail
	samples = int(longTot)
	if longTot < float64(cfg.MinSamples) {
		return 70, samples, true
	}
	var shortRate, longRate float64
	if shortTot >= 5 {
		shortRate = shortOK / shortTot
	} else {
		shortRate = longOK / longTot
	}
	longRate = longOK / longTot
	w := cfg.EWMAShortWeight
	rate := w*shortRate + (1-w)*longRate
	return 100 * rate, samples, false
}

// ComputeSpeedScore dual-window EWMA on successful latency only (P1-6); 70 if insufficient.
func ComputeSpeedScore(channelID int, modelName string, cfg Config, peerP50 map[string]float64) float64 {
	_, _, latSumS, latNS, _, _, _ := ReadWindow(channelID, modelName, cfg.EWMAShortMin)
	_, _, latSumL, latNL, _, _, _ := ReadWindow(channelID, modelName, cfg.EWMALongMin)
	if latNL < 5 {
		return 70
	}
	longAvg := latSumL / float64(latNL)
	shortAvg := longAvg
	if latNS >= 3 {
		shortAvg = latSumS / float64(latNS)
	}
	w := cfg.EWMAShortWeight
	avg := w*shortAvg + (1-w)*longAvg
	peer := peerP50[modelName]
	if peer <= 0 {
		peer = avg
	}
	s := 100 * peer / math.Max(avg, 1)
	return clamp(s, 10, 100)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ComputePriceScore §4.7: prefer live 30m unit; else ratio+fx; else 50.
// Non-token modalities skip price dimension when PriceScoreSkipNonToken (P1-5).
func ComputePriceScore(channelID int, modelName string, cfg Config, minUnit float64) (score float64, unit float64) {
	if cfg.PriceScoreSkipNonToken && IsNonTokenPricedModel(modelName, channelID) {
		return 50, 0
	}
	unit = LiveUnitCNY(channelID, modelName, cfg)
	if unit <= 0 {
		unit = ConfiguredUnitCNY(channelID, modelName)
	}
	if unit <= 0 {
		return 50, 0
	}
	if minUnit <= 0 {
		minUnit = unit
	}
	return clamp(100*minUnit/unit, 10, 100), unit
}

// LiveUnitCNY near-30min unit cost from ledger.
func LiveUnitCNY(channelID int, modelName string, cfg Config) float64 {
	since := time.Now().Add(-30 * time.Minute)
	type row struct {
		CostCNY float64
		Tokens  float64
		Source  string
	}
	var rows []row
	err := model.DB.Raw(`
SELECT cost_cny as cost_cny,
       (prompt_tokens + completion_tokens) as tokens,
       usage_source as source
FROM channel_cost_ledger
WHERE channel_id = ? AND model = ? AND created_at >= ? AND attempt_failed = ? AND cache_hit = ?
`, channelID, modelName, since, false, false).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return 0
	}
	var cost, toks, eff float64
	for _, r := range rows {
		w := 1.0
		if r.Source == "estimate" {
			w = cfg.EstimateWeight
		}
		cost += r.CostCNY * w
		toks += r.Tokens * w
		eff += w
	}
	if eff < cfg.MinPriceSamples || toks <= 0 {
		return 0
	}
	return cost / toks * 1e6
}

// ConfiguredUnitCNY from channel_cost_prices + model ref.
func ConfiguredUnitCNY(channelID int, modelName string) float64 {
	var p model.ChannelCostPrice
	if err := model.DB.Where("channel_id = ? AND model = ?", channelID, modelName).First(&p).Error; err != nil {
		// try wildcard model
		if err2 := model.DB.Where("channel_id = ? AND model = ?", channelID, "*").First(&p).Error; err2 != nil {
			return 0
		}
	}
	n := p.RechargeCreditsPerCNY
	if n <= 0 {
		return 0
	}
	if p.PriceMode == "absolute" {
		// approximate blended unit using completion ratio
		comp := ratio_setting.GetCompletionRatio(modelName)
		in, out := p.InPriceUSD, p.OutPriceUSD
		if in <= 0 && out <= 0 {
			return 0
		}
		// unit per mtok blended assuming 1:comp in:out share equal weight → use in as base
		usd := in
		if out > 0 && comp > 0 {
			usd = (in + out*comp) / (1 + comp)
		}
		if p.PriceCurrency == "CNY" {
			return usd
		}
		return usd / n
	}
	ratio := p.ChannelRatio
	if ratio <= 0 {
		// P1-9: unset ratio → treat as unconfigured → S_price=50 (unit 0)
		return 0
	}
	ref := p.ModelRefUSDPerMTok
	if ref <= 0 {
		if mp, ok := ratio_setting.GetModelPrice(modelName, false); ok && mp > 0 {
			ref = mp
		}
	}
	if ref <= 0 {
		return 0
	}
	return ref * ratio / n
}

// IsNonTokenPricedModel detects image/audio/video style models that must not use chat-token price score.
func IsNonTokenPricedModel(modelName string, channelID int) bool {
	m := strings.ToLower(modelName)
	for _, p := range []string{
		"dall-e", "tts-", "whisper", "suno", "midjourney", "mj_", "stable-diffusion",
		"flux", "veo", "runway", "luma", "sora", "gpt-image", "imagen",
	} {
		if strings.Contains(m, p) {
			return true
		}
	}
	if channelID <= 0 || model.DB == nil {
		return false
	}
	ch, err := model.CacheGetChannel(channelID)
	if err != nil || ch == nil {
		ch2, err2 := model.GetChannelById(channelID, true)
		if err2 != nil || ch2 == nil {
			return false
		}
		ch = ch2
	}
	switch ch.Type {
	case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus, constant.ChannelTypeSunoAPI:
		return true
	}
	return false
}

// RouteWeightFromScore score→route_weight; 70→100, quadratic.
func RouteWeightFromScore(score float64, state string) int {
	if score <= 0 {
		score = 70
	}
	rw := int(math.Round(100 * math.Pow(score/70, 2)))
	if rw < 1 {
		rw = 1
	}
	if state == "degraded" {
		rw = int(math.Round(float64(rw) * 0.5))
		if rw < 1 {
			rw = 1
		}
	}
	return rw
}

// RefreshAllScores recomputes scores for known channel×model pairs from abilities/weights table.
func RefreshAllScores() {
	cfg := GetConfig()
	if !cfg.Enabled {
		return
	}
	scoreMu.Lock()
	defer scoreMu.Unlock()

	type pair struct{ ChannelID int; Model string }
	var pairs []pair
	// from existing route weights + enabled channels' models
	_ = model.DB.Model(&model.ChannelRouteWeight{}).Select("channel_id, model").Find(&pairs).Error
	var channels []model.Channel
	_ = model.DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error
	seen := map[string]bool{}
	for _, p := range pairs {
		seen[scoreKey(p.ChannelID, p.Model)] = true
	}
	for _, ch := range channels {
		for _, m := range ch.GetModels() {
			k := scoreKey(ch.Id, m)
			if !seen[k] {
				pairs = append(pairs, pair{ch.Id, m})
				seen[k] = true
			}
		}
	}

	// peer latency for speed
	peerLat := map[string]float64{}
	peerN := map[string]int{}
	for _, p := range pairs {
		_, _, latSum, latN, _, _, _ := ReadWindow(p.ChannelID, p.Model, cfg.EWMALongMin)
		if latN > 0 {
			peerLat[p.Model] += latSum / float64(latN)
			peerN[p.Model]++
		}
	}
	peerP50 := map[string]float64{}
	for m, sum := range peerLat {
		if peerN[m] > 0 {
			peerP50[m] = sum / float64(peerN[m])
		}
	}

	// min unit per model for price score
	units := map[string]float64{}
	for _, p := range pairs {
		u := LiveUnitCNY(p.ChannelID, p.Model, cfg)
		if u <= 0 {
			u = ConfiguredUnitCNY(p.ChannelID, p.Model)
		}
		if u > 0 {
			if cur, ok := units[p.Model]; !ok || u < cur {
				units[p.Model] = u
			}
		}
	}

	next := map[string]*ScoreEntry{}
	prev, _ := scoreMap.Load().(map[string]*ScoreEntry)

	for _, p := range pairs {
		sConn, samples, insuff := ComputeConnScore(p.ChannelID, p.Model, cfg)
		sSpeed := ComputeSpeedScore(p.ChannelID, p.Model, cfg, peerP50)
		sPrice, _ := ComputePriceScore(p.ChannelID, p.Model, cfg, units[p.Model])
		sStab := 80.0 // simplified: outage tracking via breaker events would refine
		sAuth := 80.0
		if model.HasRouteTag(p.ChannelID, p.Model, "fake_model") {
			sAuth = 0
		}
		w := cfg.Weights
		sumW := w.Sum()
		score := (w.Conn*sConn + w.Price*sPrice + w.Speed*sSpeed + w.Stab*sStab + w.Auth*sAuth) / sumW
		state := "normal"
		if insuff {
			state = "new"
			score = 70
		} else if score < 40 {
			state = "degraded" // breaker may elevate to broken
		} else if score < 60 {
			state = "degraded"
		}
		rw := RouteWeightFromScore(score, state)
		if state == "new" {
			rw = 100 // floor applied later in SmartWeights
		}
		floorDecay := 0
		if old := prev[scoreKey(p.ChannelID, p.Model)]; old != nil {
			if old.Insufficient && !insuff {
				floorDecay = cfg.FloorDecayCycles
			} else if old.FloorDecay > 0 && !insuff {
				floorDecay = old.FloorDecay - 1
			}
		}
		ent := &ScoreEntry{
			ChannelID:    p.ChannelID,
			Model:        p.Model,
			Score:        score,
			SConn:        sConn,
			SPrice:       sPrice,
			SSpeed:       sSpeed,
			SStab:        sStab,
			SAuth:        sAuth,
			RouteWeight:  rw,
			State:        state,
			Samples:      samples,
			Insufficient: insuff,
			FloorDecay:   floorDecay,
			UpdatedAt:    time.Now(),
		}
		next[scoreKey(p.ChannelID, p.Model)] = ent
		_ = model.UpsertScoreSnapshot(model.ChannelScoreSnapshot{
			ChannelID: p.ChannelID, Model: p.Model, Score: score,
			SConn: sConn, SPrice: sPrice, SSpeed: sSpeed, SStab: sStab, SAuth: sAuth,
			Samples: samples, Insufficient: insuff,
		})
		_ = model.UpsertRouteWeight(p.ChannelID, p.Model, rw, score, state)
		EvaluateBreaker(p.ChannelID, p.Model, ent, cfg)
	}
	storeScores(next)
}
