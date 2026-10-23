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

func mergePublishedScores(snaps []model.ChannelScoreSnapshot, weights []model.ChannelRouteWeight) map[string]*ScoreEntry {
	if len(snaps) == 0 {
		return nil
	}
	weightsByKey := make(map[string]model.ChannelRouteWeight, len(weights))
	for _, w := range weights {
		weightsByKey[scoreKey(w.ChannelID, w.Model)] = w
	}
	next := make(map[string]*ScoreEntry, len(snaps))
	for _, s := range snaps {
		w := weightsByKey[scoreKey(s.ChannelID, s.Model)]
		state := w.State
		if state == "" {
			state = "normal"
		}
		next[scoreKey(s.ChannelID, s.Model)] = &ScoreEntry{
			ChannelID:    s.ChannelID,
			Model:        s.Model,
			Score:        s.Score,
			SConn:        s.SConn,
			SPrice:       s.SPrice,
			SSpeed:       s.SSpeed,
			SStab:        s.SStab,
			SAuth:        s.SAuth,
			RouteWeight:  w.RouteWeight,
			State:        state,
			Samples:      s.Samples,
			Insufficient: s.Insufficient,
			UpdatedAt:    s.UpdatedAt,
		}
	}
	return next
}

func applyPublishedScores(snaps []model.ChannelScoreSnapshot, weights []model.ChannelRouteWeight) map[string]*ScoreEntry {
	if len(snaps) == 0 {
		return GetAllScores()
	}
	merged := mergePublishedScores(snaps, weights)
	storeScores(merged)
	return merged
}

func loadPublishedScores() {
	if model.DB == nil {
		return
	}
	var snaps []model.ChannelScoreSnapshot
	if err := model.DB.Find(&snaps).Error; err != nil {
		common.SysError(common.LogText("load published scores: %v", err))
		return
	}
	var weights []model.ChannelRouteWeight
	if err := model.DB.Find(&weights).Error; err != nil {
		common.SysError(common.LogText("load published weights: %v", err))
		return
	}
	applyPublishedScores(snaps, weights)
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
		return 0, samples, true
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

// ComputeStabScore is success-rate consistency across the short and long windows.
// Either window below five samples is unknown, so the dimension is omitted.
func ComputeStabScore(channelID int, modelName string, cfg Config) (float64, bool) {
	shortMin, longMin := cfg.EWMAShortMin, cfg.EWMALongMin
	if shortMin <= 0 {
		shortMin = 5
	}
	if longMin <= 0 {
		longMin = 60
	}
	shortOK, shortFail, _, _, tokS, _, ufailS := ReadWindow(channelID, modelName, shortMin)
	longOK, longFail, _, _, tokL, _, ufailL := ReadWindow(channelID, modelName, longMin)
	shortOK, shortFail, _ = ApplySampleCap(shortOK, shortFail, tokS, cfg.SampleCapPerToken)
	longOK, longFail, _ = ApplySampleCap(longOK, longFail, tokL, cfg.SampleCapPerToken)
	shortFail = ApplyUserFailMeltCap(shortFail, ufailS, cfg.UserFailMeltCap)
	longFail = ApplyUserFailMeltCap(longFail, ufailL, cfg.UserFailMeltCap)
	shortTot := shortOK + shortFail
	longTot := longOK + longFail
	if shortTot < 5 || longTot < 5 {
		return 0, false
	}
	shortRate := shortOK / shortTot
	longRate := longOK / longTot
	diff := shortRate - longRate
	if diff < 0 {
		diff = -diff
	}
	score := 100 * (1 - diff)
	if score < 0 {
		return 0, true
	}
	if score > 100 {
		return 100, true
	}
	return score, true
}

// ComputeSpeedScore dual-window EWMA on successful latency only (P1-6); 0 if insufficient.
func ComputeSpeedScore(channelID int, modelName string, cfg Config, peerP50 map[string]float64) float64 {
	_, _, latSumS, latNS, _, _, _ := ReadWindow(channelID, modelName, cfg.EWMAShortMin)
	_, _, latSumL, latNL, _, _, _ := ReadWindow(channelID, modelName, cfg.EWMALongMin)
	if latNL < 5 {
		return 0
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
		return 0, 0
	}
	unit = LiveUnitCNY(channelID, modelName, cfg)
	if unit <= 0 {
		unit = ConfiguredUnitCNY(channelID, modelName)
	}
	if unit <= 0 {
		return 0, 0
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
	if channelID <= 0 {
		return false
	}
	// Score refresh must not load a channel from the database. A cache miss stays token-priced.
	ch, ok := model.CacheGetChannelMemory(channelID)
	if !ok || ch == nil {
		return false
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
		return 1
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

// weightedKnown averages only dimensions that were actually measured.
// Unknown price is omitted, so a missing price cannot win a cheap ranking.
func weightedKnown(w WeightParts, conn, price float64, priceKnown bool, speed float64, speedKnown bool, stab float64, stabKnown bool, auth float64, authKnown bool) float64 {
	type part struct {
		w  float64
		s  float64
		ok bool
	}
	parts := []part{
		{w.Conn, conn, true},
		{w.Price, price, priceKnown},
		{w.Speed, speed, speedKnown},
		{w.Stab, stab, stabKnown},
		{w.Auth, auth, authKnown},
	}
	var sumW, sum float64
	for _, p := range parts {
		if !p.ok || p.w <= 0 {
			continue
		}
		sumW += p.w
		sum += p.w * p.s
	}
	if sumW <= 0 {
		return 0
	}
	return sum / sumW
}

// RefreshAllScores recomputes scores for known channel×model pairs from abilities/weights table.
func RefreshAllScores() {
	cfg := GetConfig()
	if !Collects() {
		return
	}
	model.RefreshRouteState()
	scoreMu.Lock()
	defer scoreMu.Unlock()

	type pair struct {
		ChannelID int
		Model     string
	}
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
	chWeight := map[int]int{}
	for _, ch := range channels {
		w := ch.GetWeight()
		if w < 1 {
			w = 1
		}
		chWeight[ch.Id] = w
	}

	for _, p := range pairs {
		sConn, samples, insuff := ComputeConnScore(p.ChannelID, p.Model, cfg)
		sSpeed := ComputeSpeedScore(p.ChannelID, p.Model, cfg, peerP50)
		sPrice, unit := ComputePriceScore(p.ChannelID, p.Model, cfg, units[p.Model])
		priceKnown := unit > 0 && sPrice > 0
		speedKnown := sSpeed > 0
		sStab, stabKnown := ComputeStabScore(p.ChannelID, p.Model, cfg)
		var sAuth float64
		authKnown := false
		if model.CachedHasRouteTag(p.ChannelID, p.Model, "fake_model") {
			sAuth = 0
			authKnown = true
		}
		w := cfg.Weights
		state := "normal"
		official := chWeight[p.ChannelID]
		if official < 1 {
			official = 1
		}
		var score float64
		var rw int
		if insuff {
			state = "insufficient"
			rw = official
			sConn, sPrice, sSpeed, sStab, sAuth = 0, 0, 0, 0, 0
			priceKnown = false
			speedKnown = false
			stabKnown = false
		} else {
			score = weightedKnown(w, sConn, sPrice, priceKnown, sSpeed, speedKnown, sStab, stabKnown, sAuth, authKnown)
			if score < 60 {
				state = "degraded"
			}
			rw = RouteWeightFromScore(score, state)
			if rw < 1 {
				rw = 1
			}
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
