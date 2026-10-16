package model

import (
	"context"
	"time"
)

// RunPartitionPrecreate 提前 7 天预建下月分区；当前月+下月始终确保存在。
// 失败记 ops_job_runs failed + stub 告警。
func RunPartitionPrecreate(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	runKey := now.Format("2006-01-02")
	row, skip, lock, err := beginOpsJobRun(ctx, OpsJobPartitionPrecreate, runKey, false)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	defer lock.Release()

	cur := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	next := cur.AddDate(0, 1, 0)
	targets := []time.Time{cur, next}
	daysToNext := int(next.Sub(now).Hours() / 24)
	created := []string{}
	for _, m := range targets {
		name := monthPartitionName(m)
		if err := EnsureLedgerMonthPartition(DB, m); err != nil {
			_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{
				"error": err.Error(), "month": name,
			})
			_ = RecordOpsAlert(OpsAlertLevelUrgent, "partition_precreate_failed", map[string]any{
				"month": name, "error": err.Error(),
			})
			return err
		}
		created = append(created, name)
	}
	touchOpsJobHeartbeat(row)
	return finishOpsJobRun(row, OpsJobStatusOK, map[string]any{
		"partitions":    created,
		"days_to_next":  daysToNext,
		"within_7_days": daysToNext <= 7,
	})
}

// ListLedgerPartitions 验收用。
func ListLedgerPartitions() ([]string, error) {
	var names []string
	err := DB.Raw(`
SELECT c.relname FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'ledger_entries'
ORDER BY c.relname`).Scan(&names).Error
	return names, err
}
