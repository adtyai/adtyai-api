package smartrouting

import (
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// CheckPassiveAuth compares model name + token skew; may mark fake_model.
func CheckPassiveAuth(ev AttemptEvent) {
	if ev.CacheHit || model.HasRouteTag(ev.ChannelID, ev.Model, "fallback_only") {
		return
	}
	cfg := GetConfig()
	verdict := ""
	dev := 0.0
	if ev.RespModel != "" && ev.ReqModel != "" {
		if !modelFamilyMatch(ev.ReqModel, ev.RespModel) {
			verdict = "fail"
		}
	}
	if ev.PromptTokens > 0 && ev.UsageSource == "upstream" {
		// without local tokenizer here, skip if no local estimate provided
	}
	if ev.LocalPromptTokens > 0 && ev.PromptTokens > 0 {
		local := float64(ev.LocalPromptTokens)
		up := float64(ev.PromptTokens)
		dev = math.Abs(local-up) / math.Max(local, 1)
		if dev > cfg.TokenSkewFail {
			verdict = "fail"
		} else if dev > cfg.TokenSkewSuspect && verdict == "" {
			verdict = "suspect"
		}
	}
	if verdict == "" {
		return
	}
	_ = model.DB.Create(&model.ChannelAuthFlag{
		ChannelID:   ev.ChannelID,
		Model:       ev.Model,
		ReqModel:    ev.ReqModel,
		RespModel:   ev.RespModel,
		TokLocal:    ev.LocalPromptTokens,
		TokUpstream: ev.PromptTokens,
		DevPct:      dev,
		Verdict:     verdict,
	}).Error
	if verdict == "fail" {
		maybeMarkFake(ev.ChannelID, ev.Model)
	}
}

func modelFamilyMatch(req, resp string) bool {
	a := normalizeModelFamily(req)
	b := normalizeModelFamily(resp)
	return a == b || strings.HasPrefix(b, a) || strings.HasPrefix(a, b)
}

func normalizeModelFamily(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	// strip date suffix like -2024-08-06
	parts := strings.Split(m, "-")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 4 && isDigits(p) {
			break
		}
		out = append(out, p)
	}
	return strings.Join(out, "-")
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func maybeMarkFake(channelID int, modelName string) {
	since := time.Now().Add(-7 * 24 * time.Hour)
	var failN, total int64
	_ = model.DB.Model(&model.ChannelAuthFlag{}).
		Where("channel_id = ? AND model = ? AND created_at >= ? AND verdict = ? AND false_positive = ?",
			channelID, modelName, since, "fail", false).Count(&failN).Error
	_ = model.DB.Model(&model.ChannelAuthFlag{}).
		Where("channel_id = ? AND model = ? AND created_at >= ? AND false_positive = ?",
			channelID, modelName, since, false).Count(&total).Error
	if failN >= 3 && total > 0 && float64(failN)/float64(total) >= 0.05 {
		if !model.HasRouteTag(channelID, modelName, "fake_model") {
			_ = model.DB.Create(&model.ChannelRouteTag{
				ChannelID: channelID,
				Model:     modelName,
				Tag:       "fake_model",
				Note:      "passive_auth",
			}).Error
		}
	}
}
