package model

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	HealthTargetEpay     = "epay"
	HealthTargetBepusdt  = "bepusdt"
	HealthTargetChatwoot = "chatwoot"
	HealthTargetPostgres = "postgres"
	HealthTargetRedis    = "redis"
	HealthTargetChannel  = "channel"
	HealthTargetCallback = "callback"

	HealthTargetIDMain = "main"

	healthProbeSuccessRetain = 7 * 24 * time.Hour
	healthProbeFailureRetain = 30 * 24 * time.Hour

	// Per-item probe timeouts (A44：并发探活，单项超时互不影响).
	HealthProbeHTTPTimeout     = 5 * time.Second // epay / bepusdt / chatwoot HTTP
	HealthProbeRedisTimeout    = 3 * time.Second
	HealthProbePostgresTimeout = 3 * time.Second
)

var healthTargetTypes = map[string]struct{}{
	HealthTargetEpay:     {},
	HealthTargetBepusdt:  {},
	HealthTargetChatwoot: {},
	HealthTargetPostgres: {},
	HealthTargetRedis:    {},
	HealthTargetChannel:  {},
	HealthTargetCallback: {},
}

// HealthProbe 探活结果（A18 / A44）。
type HealthProbe struct {
	Id         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TargetType string    `json:"target_type" gorm:"type:text;not null;index:idx_health_probes_target_checked,priority:1"`
	TargetId   string    `json:"target_id" gorm:"type:text;not null;default:'';index:idx_health_probes_target_checked,priority:2"`
	Ok         bool      `json:"ok" gorm:"not null"`
	LatencyMs  int       `json:"latency_ms" gorm:"not null;default:0"`
	Error      string    `json:"error" gorm:"type:text"`
	CheckedAt  time.Time `json:"checked_at" gorm:"not null;index:idx_health_probes_target_checked,priority:3,sort:desc"`
}

func (HealthProbe) TableName() string { return "health_probes" }

// HealthProbeView 管理端汇总一行（最近一条 + 最近失败时间）。
type HealthProbeView struct {
	TargetType    string     `json:"target_type"`
	TargetId      string     `json:"target_id"`
	Ok            bool       `json:"ok"`
	Status        string     `json:"status"` // ok | fail | skipped（A44：前端看此字段）
	LatencyMs     int        `json:"latency_ms"`
	Error         string     `json:"error"`
	CheckedAt     time.Time  `json:"checked_at"`
	LastFailureAt *time.Time `json:"last_failure_at"`
}

// HealthSummaryResponse GET/POST /api/admin/health 响应 data。
type HealthSummaryResponse struct {
	Probes    []HealthProbeView `json:"probes"`
	Refreshed bool              `json:"refreshed"`
}

// ProbeCleanupResult probe_cleanup 结果。
type ProbeCleanupResult struct {
	DeletedSuccess int64 `json:"deleted_success"`
	DeletedFailure int64 `json:"deleted_failure"`
	Skipped        bool  `json:"skipped"`
}

// EnsureHealthProbesTable 建表 + CHECK + 复合索引（幂等）。
func EnsureHealthProbesTable(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if err := db.AutoMigrate(&HealthProbe{}); err != nil {
		return err
	}
	_ = db.Exec(`
DO $$ BEGIN
  ALTER TABLE health_probes ADD CONSTRAINT health_probes_target_type_check
    CHECK (target_type IN ('epay','bepusdt','chatwoot','postgres','redis','channel','callback'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$`)
	// 显式 DESC 索引（GORM sort:desc 可能不够稳，补一条 IF NOT EXISTS）
	_ = db.Exec(`
CREATE INDEX IF NOT EXISTS idx_health_probes_target_checked
  ON health_probes (target_type, target_id, checked_at DESC)`)
	return nil
}

func normalizeHealthTarget(targetType, targetId string) (string, string, error) {
	tt := strings.TrimSpace(strings.ToLower(targetType))
	if _, ok := healthTargetTypes[tt]; !ok {
		return "", "", fmt.Errorf("invalid target_type %q", targetType)
	}
	tid := strings.TrimSpace(targetId)
	if tid == "" {
		tid = HealthTargetIDMain
	}
	return tt, tid, nil
}

// RecordHealthProbe 写入一条探活结果。
func RecordHealthProbe(targetType, targetId string, ok bool, latencyMs int, errMsg string, checkedAt time.Time) (*HealthProbe, error) {
	tt, tid, err := normalizeHealthTarget(targetType, targetId)
	if err != nil {
		return nil, err
	}
	if latencyMs < 0 {
		latencyMs = 0
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	} else {
		checkedAt = checkedAt.UTC()
	}
	row := &HealthProbe{
		TargetType: tt,
		TargetId:   tid,
		Ok:         ok,
		LatencyMs:  latencyMs,
		Error:      errMsg,
		CheckedAt:  checkedAt,
	}
	if err := DB.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// probeItemContext 从 parent 解绑取消信号，只保留 values；再套单项超时。
// 这样并发探活时父 ctx 取消/某个兄弟失败不会中途掐死其它项。
func probeItemContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	return context.WithTimeout(base, timeout)
}

// ProbePostgres SELECT 1 探活并落表（单项超时 HealthProbePostgresTimeout）。
func ProbePostgres(ctx context.Context) (*HealthProbe, error) {
	start := time.Now()
	pctx, cancel := probeItemContext(ctx, HealthProbePostgresTimeout)
	defer cancel()
	sqlDB, err := DB.DB()
	if err != nil {
		return RecordHealthProbe(HealthTargetPostgres, HealthTargetIDMain, false, 0, err.Error(), start)
	}
	pingErr := sqlDB.PingContext(pctx)
	if pingErr != nil {
		ms := int(time.Since(start).Milliseconds())
		return RecordHealthProbe(HealthTargetPostgres, HealthTargetIDMain, false, ms, sanitizeProbeErr(pingErr), start)
	}
	var one int
	qerr := DB.WithContext(pctx).Raw(`SELECT 1`).Scan(&one).Error
	ms := int(time.Since(start).Milliseconds())
	if qerr != nil {
		return RecordHealthProbe(HealthTargetPostgres, HealthTargetIDMain, false, ms, sanitizeProbeErr(qerr), start)
	}
	return RecordHealthProbe(HealthTargetPostgres, HealthTargetIDMain, true, ms, "", start)
}

// ProbeRedis PING；未启用则 ok=false、error 标 skipped（单项超时 HealthProbeRedisTimeout）。
func ProbeRedis(ctx context.Context) (*HealthProbe, error) {
	start := time.Now()
	if !common.RedisEnabled || common.RDB == nil {
		return RecordHealthProbe(HealthTargetRedis, HealthTargetIDMain, false, 0,
			"skipped: redis not enabled", start)
	}
	pingCtx, cancel := probeItemContext(ctx, HealthProbeRedisTimeout)
	defer cancel()
	_, err := common.RDB.Ping(pingCtx).Result()
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return RecordHealthProbe(HealthTargetRedis, HealthTargetIDMain, false, ms, sanitizeProbeErr(err), start)
	}
	return RecordHealthProbe(HealthTargetRedis, HealthTargetIDMain, true, ms, "", start)
}

// ProbeHTTPStub 可选 HTTP 探活（epay/bepusdt/chatwoot 等）；无真密钥，仅探测可达性。
// url 空则记 skipped。单项超时 HealthProbeHTTPTimeout。
func ProbeHTTPStub(ctx context.Context, targetType, targetId, rawURL string) (*HealthProbe, error) {
	start := time.Now()
	tt, tid, err := normalizeHealthTarget(targetType, targetId)
	if err != nil {
		return nil, err
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return RecordHealthProbe(tt, tid, false, 0, "skipped: no probe url", start)
	}
	reqCtx, cancel := probeItemContext(ctx, HealthProbeHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return RecordHealthProbe(tt, tid, false, 0, "invalid url", start)
	}
	client := &http.Client{Timeout: HealthProbeHTTPTimeout}
	resp, err := client.Do(req)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return RecordHealthProbe(tt, tid, false, ms, sanitizeProbeErr(err), start)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return RecordHealthProbe(tt, tid, false, ms, fmt.Sprintf("http %d", resp.StatusCode), start)
	}
	return RecordHealthProbe(tt, tid, true, ms, fmt.Sprintf("http %d", resp.StatusCode), start)
}

// RunBuiltinProbes 并发跑内置 postgres + redis 探活。
// ok=false 不算硬失败；仅 RecordHealthProbe DB 错误等才返回。
func RunBuiltinProbes(ctx context.Context) error {
	return runProbesConcurrent(ctx, []func(context.Context) error{
		func(c context.Context) error { _, err := ProbePostgres(c); return err },
		func(c context.Context) error { _, err := ProbeRedis(c); return err },
	})
}

// runProbesConcurrent 并行执行探活；等全部结束。仅收集硬错误（如落库失败），ok=false 不返回。
func runProbesConcurrent(ctx context.Context, fns []func(context.Context) error) error {
	if len(fns) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	for _, fn := range fns {
		fn := fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(ctx); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// HealthProbeStatus ok=true→ok；error 以 "skipped:" 开头→skipped；否则 fail。
func HealthProbeStatus(ok bool, errMsg string) string {
	if ok {
		return "ok"
	}
	if strings.HasPrefix(errMsg, "skipped:") {
		return "skipped"
	}
	return "fail"
}

// ListHealthSummary 按 (target_type, target_id) 聚合最近一条，并附最近失败时间。
func ListHealthSummary() ([]HealthProbeView, error) {
	type latestRow struct {
		TargetType string
		TargetId   string
		Ok         bool
		LatencyMs  int
		Error      string
		CheckedAt  time.Time
	}
	var latest []latestRow
	// DISTINCT ON 需 PG；云准绳为 PG。
	err := DB.Raw(`
SELECT DISTINCT ON (target_type, target_id)
  target_type, target_id, ok, latency_ms, error, checked_at
FROM health_probes
ORDER BY target_type, target_id, checked_at DESC`).Scan(&latest).Error
	if err != nil {
		return nil, err
	}

	type failRow struct {
		TargetType string
		TargetId   string
		CheckedAt  time.Time
	}
	var fails []failRow
	err = DB.Raw(`
SELECT DISTINCT ON (target_type, target_id)
  target_type, target_id, checked_at
FROM health_probes
WHERE ok = false AND error NOT LIKE 'skipped:%'
ORDER BY target_type, target_id, checked_at DESC`).Scan(&fails).Error
	if err != nil {
		return nil, err
	}
	failMap := make(map[string]time.Time, len(fails))
	for _, f := range fails {
		failMap[f.TargetType+"\x00"+f.TargetId] = f.CheckedAt
	}

	out := make([]HealthProbeView, 0, len(latest))
	for _, r := range latest {
		v := HealthProbeView{
			TargetType: r.TargetType,
			TargetId:   r.TargetId,
			Ok:         r.Ok,
			LatencyMs:  r.LatencyMs,
			Error:      r.Error,
			CheckedAt:  r.CheckedAt,
			Status:     HealthProbeStatus(r.Ok, r.Error),
		}
		if t, ok := failMap[r.TargetType+"\x00"+r.TargetId]; ok {
			tt := t
			v.LastFailureAt = &tt
		}
		out = append(out, v)
	}
	return out, nil
}

// BuildHealthSummary（A44：refresh 走 RunConfiguredProbes）汇总响应；refresh=true 时先跑内置探活。
func BuildHealthSummary(ctx context.Context, refresh bool) (*HealthSummaryResponse, error) {
	if refresh {
		if err := RunConfiguredProbes(ctx); err != nil {
			return nil, err
		}
	}
	probes, err := ListHealthSummary()
	if err != nil {
		return nil, err
	}
	return &HealthSummaryResponse{Probes: probes, Refreshed: refresh}, nil
}

// RunProbeCleanup 清理成功≤7天、失败≤30天；写 ops_job_runs job=probe_cleanup。
// force=true 时即使当日已 ok 也重跑（验收）。
func RunProbeCleanup(ctx context.Context, force bool) (*ProbeCleanupResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	runKey := now.Format("2006-01-02")
	row, skip, lock, err := beginOpsJobRun(ctx, OpsJobProbeCleanup, runKey, force)
	if err != nil {
		return nil, err
	}
	out := &ProbeCleanupResult{}
	if skip {
		out.Skipped = true
		return out, nil
	}
	defer lock.Release()

	successCutoff := now.Add(-healthProbeSuccessRetain)
	failureCutoff := now.Add(-healthProbeFailureRetain)

	resOK := DB.WithContext(ctx).Where("ok = ? AND checked_at < ?", true, successCutoff).Delete(&HealthProbe{})
	if resOK.Error != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"error": resOK.Error.Error()})
		_ = RecordOpsAlert(OpsAlertLevelUrgent, "probe_cleanup_failed", map[string]any{
			"error": resOK.Error.Error(), "phase": "success",
		})
		return nil, resOK.Error
	}
	out.DeletedSuccess = resOK.RowsAffected

	touchOpsJobHeartbeat(row)

	resFail := DB.WithContext(ctx).Where("ok = ? AND checked_at < ?", false, failureCutoff).Delete(&HealthProbe{})
	if resFail.Error != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"error": resFail.Error.Error()})
		_ = RecordOpsAlert(OpsAlertLevelUrgent, "probe_cleanup_failed", map[string]any{
			"error": resFail.Error.Error(), "phase": "failure",
		})
		return nil, resFail.Error
	}
	out.DeletedFailure = resFail.RowsAffected

	if err := finishOpsJobRun(row, OpsJobStatusOK, map[string]any{
		"deleted_success": out.DeletedSuccess,
		"deleted_failure": out.DeletedFailure,
		"success_cutoff":  successCutoff.Format(time.RFC3339),
		"failure_cutoff":  failureCutoff.Format(time.RFC3339),
	}); err != nil {
		return out, err
	}
	return out, nil
}

// CountHealthProbes 验收用。
func CountHealthProbes(targetType, targetId string, ok *bool) (int64, error) {
	q := DB.Model(&HealthProbe{})
	if targetType != "" {
		q = q.Where("target_type = ?", targetType)
	}
	if targetId != "" {
		q = q.Where("target_id = ?", targetId)
	}
	if ok != nil {
		q = q.Where("ok = ?", *ok)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}
