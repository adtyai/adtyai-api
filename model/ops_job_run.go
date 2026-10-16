package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OpsJobHourlyRollup       = "hourly_rollup"
	OpsJobDailyReconcile     = "daily_reconcile"
	OpsJobPartitionPrecreate = "partition_precreate"
	OpsJobBackup             = "backup"
	OpsJobProbeCleanup       = "probe_cleanup"
	OpsJobRestoreDrill       = "restore_drill"
	OpsJobDailyReport        = "daily_report"

	OpsJobStatusRunning = "running"
	OpsJobStatusOK      = "ok"
	OpsJobStatusFailed  = "failed"
	OpsJobStatusStale   = "stale"
)

// OpsJobRun 运维任务运行记录（A18）。
type OpsJobRun struct {
	Id          int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	Job         string     `json:"job" gorm:"type:text;not null;uniqueIndex:ux_ops_job_run_key,priority:1"`
	RunKey      string     `json:"run_key" gorm:"type:text;not null;uniqueIndex:ux_ops_job_run_key,priority:2"`
	Status      string     `json:"status" gorm:"type:text;not null"`
	Attempt     int        `json:"attempt" gorm:"not null;default:1"`
	HeartbeatAt *time.Time `json:"heartbeat_at"`
	Detail      string     `json:"detail" gorm:"type:jsonb"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

func (OpsJobRun) TableName() string { return "ops_job_runs" }

// EnsureOpsJobRunsTable 建表 + CHECK（幂等）。
func EnsureOpsJobRunsTable(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if err := db.AutoMigrate(&OpsJobRun{}); err != nil {
		return err
	}
	// B13：job CHECK 增加 daily_report。旧约束（无 daily_report）先删再加；
	// 已是新定义时不动（幂等，不重复 DROP）。
	_ = db.Exec(`
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ops_job_runs_job_check'
             AND pg_get_constraintdef(oid) NOT LIKE '%daily_report%') THEN
    ALTER TABLE ops_job_runs DROP CONSTRAINT ops_job_runs_job_check;
  END IF;
END $$`)
	_ = db.Exec(`
DO $$ BEGIN
  ALTER TABLE ops_job_runs ADD CONSTRAINT ops_job_runs_job_check
    CHECK (job IN ('hourly_rollup','daily_reconcile','partition_precreate','backup','probe_cleanup','restore_drill','daily_report'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$`)
	_ = db.Exec(`
DO $$ BEGIN
  ALTER TABLE ops_job_runs ADD CONSTRAINT ops_job_runs_status_check
    CHECK (status IN ('running','ok','failed','stale'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$`)
	return nil
}

func opsJobLockKey(job, runKey string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(job + "\x00" + runKey))
	return int64(h.Sum32())
}

type opsJobLock struct {
	conn *sql.Conn
	key  int64
}

func (l *opsJobLock) Release() {
	if l == nil || l.conn == nil {
		return
	}
	_, _ = l.conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, l.key)
	_ = l.conn.Close()
	l.conn = nil
}

// tryOpsJobLock 专用连接 try_advisory_lock；拿不到返回 nil,false。
func tryOpsJobLock(ctx context.Context, job, runKey string) (*opsJobLock, bool, error) {
	sqlDB, err := DB.DB()
	if err != nil {
		return nil, false, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	key := opsJobLockKey(job, runKey)
	var got bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !got {
		_ = conn.Close()
		_ = RecordOpsAlert(OpsAlertLevelPending, "ops_job_lock_held", map[string]any{
			"job": job, "run_key": runKey,
		})
		return nil, false, nil
	}
	return &opsJobLock{conn: conn, key: key}, true, nil
}

// beginOpsJobRun 拿锁并标记 running。skip=true：已 ok 或拿不到锁。
// force=true 时即使 ok 也重跑（验收删汇总后补跑用）。
func beginOpsJobRun(ctx context.Context, job, runKey string, force bool) (row *OpsJobRun, skip bool, lock *opsJobLock, err error) {
	lock, got, err := tryOpsJobLock(ctx, job, runKey)
	if err != nil {
		return nil, false, nil, err
	}
	if !got {
		return nil, true, nil, nil
	}

	now := time.Now().UTC()
	var existing OpsJobRun
	err = DB.Where("job = ? AND run_key = ?", job, runKey).First(&existing).Error
	if err == nil {
		if existing.Status == OpsJobStatusOK && !force {
			lock.Release()
			return &existing, true, nil, nil
		}
		if existing.Status == OpsJobStatusRunning {
			existing.Status = OpsJobStatusStale
			_ = DB.Model(&existing).Update("status", OpsJobStatusStale).Error
		}
		existing.Status = OpsJobStatusRunning
		existing.Attempt++
		existing.StartedAt = &now
		existing.HeartbeatAt = &now
		existing.FinishedAt = nil
		existing.Detail = "{}"
		if err := DB.Save(&existing).Error; err != nil {
			lock.Release()
			return nil, false, nil, err
		}
		return &existing, false, lock, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		lock.Release()
		return nil, false, nil, err
	}

	row = &OpsJobRun{
		Job:         job,
		RunKey:      runKey,
		Status:      OpsJobStatusRunning,
		Attempt:     1,
		HeartbeatAt: &now,
		StartedAt:   &now,
		Detail:      "{}",
	}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error; err != nil {
		lock.Release()
		return nil, false, nil, err
	}
	if row.Id == 0 {
		if err := DB.Where("job = ? AND run_key = ?", job, runKey).First(row).Error; err != nil {
			lock.Release()
			return nil, false, nil, err
		}
		if row.Status == OpsJobStatusOK && !force {
			lock.Release()
			return row, true, nil, nil
		}
		row.Status = OpsJobStatusRunning
		row.Attempt++
		row.StartedAt = &now
		row.HeartbeatAt = &now
		row.FinishedAt = nil
		if err := DB.Save(row).Error; err != nil {
			lock.Release()
			return nil, false, nil, err
		}
	}
	return row, false, lock, nil
}

func finishOpsJobRun(row *OpsJobRun, status string, detail map[string]any) error {
	now := time.Now().UTC()
	b := []byte("{}")
	if detail != nil {
		b, _ = json.Marshal(detail)
	}
	return DB.Model(row).Updates(map[string]any{
		"status":       status,
		"finished_at":  now,
		"heartbeat_at": now,
		"detail":       string(b),
		"attempt":      row.Attempt,
	}).Error
}

func touchOpsJobHeartbeat(row *OpsJobRun) {
	now := time.Now().UTC()
	_ = DB.Model(row).Update("heartbeat_at", now).Error
}

// LastOKOpsJobRunKey 返回某 job 最近一次 status=ok 的 run_key。
func LastOKOpsJobRunKey(job string) (string, bool, error) {
	var row OpsJobRun
	err := DB.Where("job = ? AND status = ?", job, OpsJobStatusOK).
		Order("run_key DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.RunKey, true, nil
}

// CountOpsJobRuns 验收用。
func CountOpsJobRuns(job, runKey string) (int64, error) {
	var n int64
	q := DB.Model(&OpsJobRun{}).Where("job = ?", job)
	if runKey != "" {
		q = q.Where("run_key = ?", runKey)
	}
	err := q.Count(&n).Error
	return n, err
}

// GetOpsJobRun 验收用。
func GetOpsJobRun(job, runKey string) (*OpsJobRun, error) {
	var row OpsJobRun
	err := DB.Where("job = ? AND run_key = ?", job, runKey).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// TryInsertDuplicateOpsJobRun 验收唯一键。
func TryInsertDuplicateOpsJobRun(job, runKey string) error {
	row := OpsJobRun{
		Job:     job,
		RunKey:  runKey,
		Status:  OpsJobStatusOK,
		Attempt: 1,
		Detail:  "{}",
	}
	return DB.Create(&row).Error
}

func fmtOpsErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}

// HoldOpsJobLockForTest 验收：占住 advisory lock，使 beginOpsJobRun skip。
// 调用方必须 release()。
func HoldOpsJobLockForTest(ctx context.Context, job, runKey string) (release func(), held bool, err error) {
	lock, got, err := tryOpsJobLock(ctx, job, runKey)
	if err != nil {
		return nil, false, err
	}
	if !got {
		return func() {}, false, nil
	}
	return func() { lock.Release() }, true, nil
}
