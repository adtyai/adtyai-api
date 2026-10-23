package smartrouting

import (
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// WriteCostLedger records one attempt cost line (§4.7).
func WriteCostLedger(ev AttemptEvent) {
	if ev.CacheHit || model.DB == nil {
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
	if costCNY <= 0 || model.DB == nil {
		return
	}
	day := time.Now().Format("2006-01-02")
	if err := model.AddChannelDayCost(channelID, day, costCNY); err != nil {
		return
	}
	var g model.ChannelCostGuard
	if err := model.DB.Where("channel_id = ? AND day = ?", channelID, day).First(&g).Error; err != nil {
		return
	}
	if g.Excluded {
		return
	}
	avg := avgDailyCost7d(channelID, day)
	if avg <= 0 || g.CostCNY <= avg*cfg.DailyCostGuardMult {
		return
	}
	if err := model.DB.Model(&model.ChannelCostGuard{}).Where("channel_id = ? AND day = ? AND excluded = ?", channelID, day, false).Update("excluded", true).Error; err != nil {
		return
	}
	model.RefreshRouteState()
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
