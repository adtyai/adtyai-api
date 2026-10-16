package smartrouting

import (
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// WriteCostLedger records one attempt cost line (§4.7).
func WriteCostLedger(ev AttemptEvent) {
	if ev.CacheHit {
		return
	}
	cfg := GetConfig()
	prompt, completion := ev.PromptTokens, ev.CompletionTokens
	usageSrc := ev.UsageSource
	if usageSrc == "" {
		if prompt+completion > 0 {
			usageSrc = "upstream"
		} else {
			usageSrc = "estimate"
		}
	}
	costUSD, costCNY, mode := estimateCost(ev.ChannelID, ev.Model, prompt, completion)
	_ = model.DB.Create(&model.ChannelCostLedger{
		RequestID:        ev.RequestID,
		ChannelID:        ev.ChannelID,
		Model:            ev.Model,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		CostUSD:          costUSD,
		CostCNY:          costCNY,
		PriceMode:        mode,
		UsageSource:      usageSrc,
		AttemptFailed:    !ev.Success,
		CacheHit:         ev.CacheHit,
	}).Error
	bumpDayCost(ev.ChannelID, costCNY, cfg)
}

func estimateCost(channelID int, modelName string, prompt, completion int) (usd, cny float64, mode string) {
	var p model.ChannelCostPrice
	found := model.DB.Where("channel_id = ? AND model = ?", channelID, modelName).First(&p).Error == nil
	if !found {
		found = model.DB.Where("channel_id = ? AND model = ?", channelID, "*").First(&p).Error == nil
	}
	comp := ratio_setting.GetCompletionRatio(modelName)
	if !found {
		return 0, 0, "none"
	}
	n := p.RechargeCreditsPerCNY
	if n <= 0 {
		n = 1
	}
	mode = p.PriceMode
	if mode == "" {
		mode = "ratio"
	}
	if mode == "absolute" {
		usd = (float64(prompt)*p.InPriceUSD + float64(completion)*p.OutPriceUSD) / 1e6
		if p.PriceCurrency == "CNY" {
			return usd, usd, mode
		}
		return usd, usd / n, mode
	}
	ratio := p.ChannelRatio
	if ratio <= 0 {
		// P1-9: unfilled ratio → no estimate (matches S_price=50 path)
		return 0, 0, "none"
	}
	ref := p.ModelRefUSDPerMTok
	if ref <= 0 {
		if mp, ok := ratio_setting.GetModelPrice(modelName, false); ok {
			ref = mp
		}
	}
	weighted := float64(prompt) + float64(completion)*comp
	usd = ref * ratio * weighted / 1e6
	cny = usd / n
	return usd, cny, "ratio"
}

func bumpDayCost(channelID int, costCNY float64, cfg Config) {
	if costCNY <= 0 {
		return
	}
	day := time.Now().Format("2006-01-02")
	var g model.ChannelCostGuard
	err := model.DB.Where("channel_id = ? AND day = ?", channelID, day).First(&g).Error
	if err != nil {
		g = model.ChannelCostGuard{ChannelID: channelID, Day: day, CostCNY: costCNY}
		_ = model.DB.Create(&g).Error
	} else {
		g.CostCNY += costCNY
		_ = model.DB.Save(&g).Error
	}
	// guard: > 7d avg * mult
	avg := avgDailyCost7d(channelID, day)
	if avg > 0 && g.CostCNY > avg*cfg.DailyCostGuardMult {
		g.Excluded = true
		_ = model.DB.Save(&g).Error
		if !model.HasRouteTag(channelID, "*", "excluded") {
			_ = model.DB.Create(&model.ChannelRouteTag{
				ChannelID: channelID,
				Model:     "*",
				Tag:       "excluded",
				Note:      "cost_guard",
			}).Error
		}
	}
}

func avgDailyCost7d(channelID int, today string) float64 {
	var rows []model.ChannelCostGuard
	_ = model.DB.Where("channel_id = ? AND day < ?", channelID, today).
		Order("day desc").Limit(7).Find(&rows).Error
	if len(rows) == 0 {
		return 0
	}
	var sum float64
	for _, r := range rows {
		sum += r.CostCNY
	}
	return sum / float64(len(rows))
}

// CostRankingsByMargin root-only profit ranking.
func CostRankingsByMargin(from, to time.Time) ([]map[string]any, error) {
	type agg struct {
		ChannelID int
		CostCNY   float64
	}
	var rows []agg
	err := model.DB.Model(&model.ChannelCostLedger{}).
		Select("channel_id, SUM(cost_cny) as cost_cny").
		Where("created_at >= ? AND created_at < ? AND attempt_failed = ?", from, to, false).
		Group("channel_id").
		Order("cost_cny desc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"channel_id": r.ChannelID,
			"cost_cny":   r.CostCNY,
			// user_charge / margin filled when joining logs; cost side here
		})
	}
	return out, nil
}
