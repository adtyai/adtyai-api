package smartrouting

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// BackfillCostFromLogs 审查 N-P1-5：从本地消费日志回填 channel_cost_ledger。
// 已有成本行（同 log_id 或同 request_id）跳过，幂等；单次最多处理 limit 条日志。
// 返回 (扫描日志数, 新写入行数)。
func BackfillCostFromLogs(from, to time.Time, limit int) (scanned, inserted int, err error) {
	if !to.After(from) {
		return 0, 0, errors.New("to must be after from")
	}
	if limit <= 0 || limit > 50000 {
		limit = 50000
	}
	var logs []model.Log
	if err := model.LOG_DB.Where("type = ? AND created_at >= ? AND created_at < ? AND channel_id > 0", model.LogTypeConsume, from.Unix(), to.Unix()).
		Order("id asc").Limit(limit).Find(&logs).Error; err != nil {
		return 0, 0, err
	}
	for _, lg := range logs {
		scanned++
		var n int64
		q := model.DB.Model(&model.ChannelCostLedger{}).Where("log_id = ?", lg.Id)
		if lg.RequestId != "" {
			q = model.DB.Model(&model.ChannelCostLedger{}).Where("log_id = ? OR (request_id = ? AND channel_id = ?)", lg.Id, lg.RequestId, lg.ChannelId)
		}
		if err := q.Count(&n).Error; err != nil {
			return scanned, inserted, err
		}
		if n > 0 {
			continue
		}
		usd, cny, mode := estimateCost(lg.ChannelId, lg.ModelName, lg.PromptTokens, lg.CompletionTokens)
		row := model.ChannelCostLedger{
			RequestID:        lg.RequestId,
			LogID:            lg.Id,
			ChannelID:        lg.ChannelId,
			Model:            lg.ModelName,
			PromptTokens:     lg.PromptTokens,
			CompletionTokens: lg.CompletionTokens,
			CostUSD:          usd,
			CostCNY:          cny,
			PriceMode:        mode,
			UsageSource:      "backfill",
			CreatedAt:        time.Unix(lg.CreatedAt, 0),
		}
		if err := model.DB.Create(&row).Error; err != nil {
			return scanned, inserted, err
		}
		inserted++
	}
	return scanned, inserted, nil
}
