package smartrouting

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
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
	// 审查 R-P2-7：按批次一次性查出已存在的 log_id / (request_id, channel_id)，批量写入，避免 N+1。
	const batch = 500
	for i := 0; i < len(logs); i += batch {
		chunk := logs[i:min(i+batch, len(logs))]
		logIDs := make([]int, 0, len(chunk))
		reqIDs := make([]string, 0, len(chunk))
		for _, lg := range chunk {
			logIDs = append(logIDs, lg.Id)
			if lg.RequestId != "" {
				reqIDs = append(reqIDs, lg.RequestId)
			}
		}
		var existing []model.ChannelCostLedger
		q := model.DB.Model(&model.ChannelCostLedger{}).Select("log_id", "request_id", "channel_id")
		if len(reqIDs) > 0 {
			q = q.Where("log_id IN ? OR request_id IN ?", logIDs, reqIDs)
		} else {
			q = q.Where("log_id IN ?", logIDs)
		}
		if err := q.Find(&existing).Error; err != nil {
			return scanned, inserted, err
		}
		seenLog := make(map[int]struct{}, len(existing))
		seenReq := make(map[string]struct{}, len(existing))
		for _, e := range existing {
			seenLog[int(e.LogID)] = struct{}{}
			if e.RequestID != "" {
				seenReq[e.RequestID+"|"+strconv.Itoa(e.ChannelID)] = struct{}{}
			}
		}
		rows := make([]model.ChannelCostLedger, 0, len(chunk))
		for _, lg := range chunk {
			scanned++
			if _, ok := seenLog[lg.Id]; ok {
				continue
			}
			if lg.RequestId != "" {
				if _, ok := seenReq[lg.RequestId+"|"+strconv.Itoa(lg.ChannelId)]; ok {
					continue
				}
			}
			usd, cny, mode := estimateCost(lg.ChannelId, lg.ModelName, lg.PromptTokens, lg.CompletionTokens)
			rows = append(rows, model.ChannelCostLedger{
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
			})
		}
		if len(rows) > 0 {
			if err := model.DB.CreateInBatches(&rows, 100).Error; err != nil {
				return scanned, inserted, err
			}
			inserted += len(rows)
		}
	}
	return scanned, inserted, nil
}

// BackfillJob 审查 R-P2-7：成本回填改为后台任务，HTTP 请求只负责启动并返回 job id。
type BackfillJob struct {
	ID         string `json:"job_id"` // 审查 R3-P2-1：字符串，避免 JS 丢精度
	Status     string `json:"status"` // running|done|failed
	From       int64  `json:"from"`
	To         int64  `json:"to"`
	Scanned    int    `json:"scanned"`
	Inserted   int    `json:"inserted"`
	Error      string `json:"error,omitempty"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at,omitempty"`
}

var (
	backfillMu  sync.Mutex
	backfillJob *BackfillJob
)

// StartBackfillJob 启动后台回填；已有任务在跑时返回 (当前任务, false)。
func StartBackfillJob(from, to time.Time, limit int) (BackfillJob, bool) {
	backfillMu.Lock()
	if backfillJob != nil && backfillJob.Status == "running" {
		cur := *backfillJob
		backfillMu.Unlock()
		return cur, false
	}
	job := &BackfillJob{ID: strconv.FormatInt(time.Now().UnixNano(), 10), Status: "running", From: from.Unix(), To: to.Unix(), StartedAt: time.Now().Unix()}
	backfillJob = job
	snap := *job
	backfillMu.Unlock()
	go func() {
		var scanned, inserted int
		var err error
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
			backfillMu.Lock()
			job.Scanned, job.Inserted, job.FinishedAt = scanned, inserted, time.Now().Unix()
			if err != nil {
				job.Status, job.Error = "failed", err.Error()
			} else {
				job.Status = "done"
			}
			backfillMu.Unlock()
		}()
		scanned, inserted, err = BackfillCostFromLogs(from, to, limit)
	}()
	return snap, true
}

// GetBackfillJob 返回最近一次回填任务（没有则 nil）。
func GetBackfillJob() *BackfillJob {
	backfillMu.Lock()
	defer backfillMu.Unlock()
	if backfillJob == nil {
		return nil
	}
	cp := *backfillJob
	return &cp
}
