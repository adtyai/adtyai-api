package model

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// HourlyRollupAccount API 扣费小时汇总默认 account（无拆分数据 / 差额兜底记实充）。
// B-03（选 B）：按 adty_wallet_consume_hourly 实际出资拆成 bonus / paid 至多两行；
// 每小时合计仍以 logs 为准，详见 writeHourlyRollupUserTx。
const HourlyRollupAccount = "paid"

// HourlyRollupAccountBonus 赠送钱包出资部分。
const HourlyRollupAccountBonus = "bonus"

// HourlyRollupResult 一次补跑结果。
type HourlyRollupResult struct {
	HoursAttempted int `json:"hours_attempted"`
	HoursOK        int `json:"hours_ok"`
	HoursSkipped   int `json:"hours_skipped"`
	HoursFailed    int `json:"hours_failed"`
	RowsWritten    int `json:"rows_written"`
}

// RunHourlyRollupBackfill 从上次 ok 的 run_key 下一小时扫到上一整点；forceHour 非零则只跑该小时（验收）。
func RunHourlyRollupBackfill(ctx context.Context, forceHour *time.Time) (*HourlyRollupResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	out := &HourlyRollupResult{}

	var hours []time.Time
	if forceHour != nil {
		hours = []time.Time{TruncateToHourUTC(*forceHour)}
	} else {
		prev := TruncateToHourUTC(time.Now().UTC()).Add(-time.Hour)
		start := prev.Add(-23 * time.Hour) // 默认最多补 24 小时
		if key, ok, err := LastOKOpsJobRunKey(OpsJobHourlyRollup); err != nil {
			return nil, err
		} else if ok {
			last, err := ParseHourBucketRunKey(key)
			if err == nil {
				start = last.Add(time.Hour)
			}
		}
		for h := start; !h.After(prev); h = h.Add(time.Hour) {
			hours = append(hours, h)
		}
	}

	for _, hb := range hours {
		out.HoursAttempted++
		n, skipped, err := runHourlyRollupOne(ctx, hb, forceHour != nil)
		if err != nil {
			out.HoursFailed++
			common.SysError(common.LogText("hourly_rollup %s: %s", HourBucketRunKey(hb), err.Error()))
			continue
		}
		if skipped {
			out.HoursSkipped++
		} else {
			out.HoursOK++
			out.RowsWritten += n
		}
	}
	return out, nil
}

func runHourlyRollupOne(ctx context.Context, hourBucket time.Time, force bool) (rowsWritten int, skipped bool, err error) {
	hb := TruncateToHourUTC(hourBucket)
	runKey := HourBucketRunKey(hb)

	row, skip, lock, err := beginOpsJobRun(ctx, OpsJobHourlyRollup, runKey, force)
	if err != nil {
		return 0, false, err
	}
	if skip {
		return 0, true, nil
	}
	defer lock.Release()

	touchOpsJobHeartbeat(row)

	startUnix := hb.Unix()
	endUnix := hb.Add(time.Hour).Unix()

	type agg struct {
		UserId int   `gorm:"column:user_id"`
		Total  int64 `gorm:"column:total"`
	}
	var aggs []agg
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if err := logDB.Table("logs").
		Select("user_id, COALESCE(SUM(quota),0) AS total").
		Where("type = ? AND created_at >= ? AND created_at < ? AND quota > 0", LogTypeConsume, startUnix, endUnix).
		Group("user_id").
		Scan(&aggs).Error; err != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"error": err.Error()})
		return 0, false, err
	}

	written := 0
	fallbackUsers := 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		splits, err := loadWalletConsumeForHour(tx, hb)
		if err != nil {
			return err
		}
		for _, a := range aggs {
			n, fb, err := writeHourlyRollupUserTx(tx, a.UserId, a.Total, splits[a.UserId], hb)
			if err != nil {
				return err
			}
			written += n
			if fb {
				fallbackUsers++
			}
		}
		return nil
	})
	if err != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"error": err.Error()})
		return 0, false, err
	}

	touchOpsJobHeartbeat(row)
	if err := finishOpsJobRun(row, OpsJobStatusOK, map[string]any{
		"users":          len(aggs),
		"written":        written,
		"fallback_users": fallbackUsers,
		"hour":           runKey,
	}); err != nil {
		return written, false, err
	}
	return written, false, nil
}

// CountHourlyRollupRows 验收：某用户某小时汇总行数。
func CountHourlyRollupRows(userId int, hourBucket time.Time) (int64, error) {
	hb := TruncateToHourUTC(hourBucket)
	var n int64
	err := DB.Model(&LedgerEntry{}).
		Where("user_id = ? AND kind = ? AND hour_bucket = ?", userId, LedgerKindAPIConsumeHourly, hb).
		Count(&n).Error
	return n, err
}

// SumHourlyRollupDelta 验收：汇总 delta。
func SumHourlyRollupDelta(userId int, hourBucket time.Time) (int64, error) {
	hb := TruncateToHourUTC(hourBucket)
	var sum int64
	err := DB.Model(&LedgerEntry{}).
		Where("user_id = ? AND kind = ? AND hour_bucket = ?", userId, LedgerKindAPIConsumeHourly, hb).
		Select("COALESCE(SUM(delta),0)").Scan(&sum).Error
	return sum, err
}

// DeleteHourlyRollupRowsForTest 验收专用：关触发器后删汇总行再开。
func DeleteHourlyRollupRowsForTest(userId int, hourBucket time.Time) error {
	hb := TruncateToHourUTC(hourBucket)
	// 同一事务内关/删/开：重新开启失败会回滚并返回错误，不再被吞掉
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := DisableLedgerImmutableTrigger(tx); err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND kind = ? AND hour_bucket = ?", userId, LedgerKindAPIConsumeHourly, hb).
			Delete(&LedgerEntry{}).Error; err != nil {
			return err
		}
		return EnableLedgerImmutableTrigger(tx)
	})
}

// SeedConsumeLogForTest 验收：写入一条消费日志（unix 秒）。
func SeedConsumeLogForTest(userId int, quota int, at time.Time) error {
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	username, _ := GetUsernameById(userId, false)
	row := Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: at.UTC().Unix(),
		Type:      LogTypeConsume,
		Content:   "accept hourly rollup seed",
		Quota:     quota,
		ModelName: "accept-model",
	}
	return logDB.Create(&row).Error
}

// writeHourlyRollupUserTx 为单用户单小时写至多两行（bonus、paid），合计 = logs total。
// fallback=true 表示拆分表与日志合计不一致，差额已记 paid 并打日志。
func writeHourlyRollupUserTx(tx *gorm.DB, userId int, total int64, split AdtyWalletConsumeHourly, hb time.Time) (written int, fallback bool, err error) {
	if total <= 0 {
		return 0, false, nil
	}
	// 幂等按「用户×小时」整体判断：已有任一汇总行（含切换前只记 paid 的旧行）则整体跳过，
	// 避免 force 重跑旧小时时再补一条 bonus 造成重复扣账。两行在同一事务写入。
	var exist int64
	if err := tx.Model(&LedgerEntry{}).
		Where("user_id = ? AND account IN ? AND kind = ? AND hour_bucket = ? AND created_at = ?",
			userId, []string{HourlyRollupAccount, HourlyRollupAccountBonus}, LedgerKindAPIConsumeHourly, hb, hb).
		Count(&exist).Error; err != nil {
		return 0, false, err
	}
	if exist > 0 {
		return 0, false, nil
	}
	bonus, paid, remainder := planHourlyRollupSplit(total, split)
	if remainder != 0 {
		fallback = true
		common.SysLog(common.LogText("hourly_rollup %s user %d: wallet split (bonus=%d paid=%d) != logs total %d, remainder %d booked as paid",
			HourBucketRunKey(hb), userId, split.Bonus, split.Paid, total, remainder))
	}
	for _, part := range []struct {
		account string
		amount  int64
	}{{HourlyRollupAccountBonus, bonus}, {HourlyRollupAccount, paid}} {
		if part.amount <= 0 {
			continue
		}
		if _, err := createHourlyRollupLedgerTx(tx, userId, part.account, -part.amount, 0, hb); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return written, fallback, err
		}
		written++
	}
	return written, fallback, nil
}
