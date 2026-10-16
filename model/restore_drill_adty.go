package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
)

// B12 恢复演练骨架：每周日在预览环境校验最近一份备份对象存在，再调可注入的解密/恢复钩子。
// 真 age 私钥解密 + pg_restore 由运维接线；默认钩子返回 restore_decrypt_not_configured。

const (
	restoreDrillKindFailed         = "restore_drill_failed"
	ErrRestoreDecryptNotConfigured = "restore_decrypt_not_configured"
	defaultRestoreDrillAfterHour   = 4 // 上海周日 ≥ 此小时才跑（备份 03:00 后）
)

// RestoreDecryptFn 运维注入：真实实现做 age 解密 + pg_restore；验收注入假成功/失败。
// obj 为已 Head 校验过的远端对象；返回写入 ops_job_runs.detail 的额外字段。
type RestoreDecryptFn func(ctx context.Context, obj *BackupObject) (map[string]any, error)

// RestoreDrillDeps 可注入依赖。
type RestoreDrillDeps struct {
	Store   BackupStore
	Decrypt RestoreDecryptFn
	Now     func() time.Time
}

func defaultRestoreDecrypt(_ context.Context, _ *BackupObject) (map[string]any, error) {
	return nil, errors.New(ErrRestoreDecryptNotConfigured)
}

// isDefaultRestoreDecrypt 判断解密钩子是否仍为默认未接线桩（nil 或 defaultRestoreDecrypt）。
func isDefaultRestoreDecrypt(fn RestoreDecryptFn) bool {
	if fn == nil {
		return true
	}
	return reflect.ValueOf(fn).Pointer() == reflect.ValueOf(defaultRestoreDecrypt).Pointer()
}

// restoreDrillNotConfiguredAlready 同 runKey 已因未接线失败过（已告警一次），且钩子仍未接线 → 不再重试/重复 urgent。
func restoreDrillNotConfiguredAlready(job, runKey string) bool {
	if DB == nil {
		return false
	}
	var existing OpsJobRun
	if err := DB.Where("job = ? AND run_key = ?", job, runKey).First(&existing).Error; err != nil {
		return false
	}
	return existing.Status == OpsJobStatusFailed && strings.Contains(existing.Detail, ErrRestoreDecryptNotConfigured)
}

func (d *RestoreDrillDeps) fill() error {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Decrypt == nil {
		d.Decrypt = defaultRestoreDecrypt
	}
	if d.Store == nil {
		s, err := NewR2BackupStoreFromEnv()
		if err != nil {
			return err
		}
		d.Store = s
	}
	return nil
}

// RestoreDrillAfterHour 上海周日起始小时（可用 ADTY_RESTORE_DRILL_AFTER_HOUR 覆盖，0–23）。
func RestoreDrillAfterHour() int {
	v := strings.TrimSpace(os.Getenv("ADTY_RESTORE_DRILL_AFTER_HOUR"))
	if v == "" {
		return defaultRestoreDrillAfterHour
	}
	var h int
	if _, err := fmt.Sscanf(v, "%d", &h); err != nil || h < 0 || h > 23 {
		return defaultRestoreDrillAfterHour
	}
	return h
}

// RestoreDrillDue 上海时区周日且已过 RestoreDrillAfterHour。
func RestoreDrillDue(now time.Time) bool {
	n := now.In(ShanghaiLocation())
	if n.Weekday() != time.Sunday {
		return false
	}
	return n.Hour() >= RestoreDrillAfterHour()
}

// RestoreDrillResult 返回给 system task / 验收。
type RestoreDrillResult struct {
	RunKey    string         `json:"run_key"`
	Skipped   bool           `json:"skipped"`
	Status    string         `json:"status"`
	Reason    string         `json:"reason,omitempty"`
	ObjectKey string         `json:"object_key,omitempty"`
	Size      int64          `json:"size,omitempty"`
	SHA256    string         `json:"sha256,omitempty"`
	BackupDay string         `json:"backup_day,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// FindLatestDailyBackupObject 在 Store 的 daily/ 下找最新一份 adtyai-YYYY-MM-DD.dump.age，
// 并 Head 校验 size>0 + sha256 meta 非空；List 条目若带 sha256 须与 Head 一致。
func FindLatestDailyBackupObject(ctx context.Context, store BackupStore) (*BackupObject, string, error) {
	if store == nil {
		return nil, "", errors.New("backup store nil")
	}
	objs, err := store.List(ctx, BackupDailyPrefix)
	if err != nil {
		return nil, "", err
	}
	var bestDay string
	var bestKey string
	var bestListSHA string
	for _, o := range objs {
		// daily/YYYY-MM-DD/adtyai-YYYY-MM-DD.dump.age
		rest := strings.TrimPrefix(o.Key, BackupDailyPrefix)
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			continue
		}
		day := parts[0]
		if _, e := time.Parse("2006-01-02", day); e != nil {
			continue
		}
		if IsBackupRetentionExempt(o.Key) && !strings.HasPrefix(strings.ToLower(parts[1]), "adtyai-") {
			continue
		}
		expect := BackupObjectKey(day)
		if o.Key != expect && !strings.HasSuffix(o.Key, ".dump.age") {
			continue
		}
		if bestDay == "" || day > bestDay {
			bestDay = day
			bestKey = o.Key
			bestListSHA = strings.TrimSpace(o.SHA256)
		}
	}
	if bestKey == "" {
		return nil, "", errors.New("no daily backup object found")
	}
	obj, err := store.Head(ctx, bestKey)
	if err != nil {
		return nil, "", fmt.Errorf("head %s: %w", bestKey, err)
	}
	if obj.Size <= 0 {
		return nil, "", fmt.Errorf("backup object empty: %s", bestKey)
	}
	headSHA := strings.TrimSpace(obj.SHA256)
	if headSHA == "" {
		return nil, "", fmt.Errorf("backup object sha256 meta missing: %s", bestKey)
	}
	if bestListSHA != "" && !strings.EqualFold(bestListSHA, headSHA) {
		return nil, "", fmt.Errorf("backup object sha256 mismatch list=%s head=%s: %s", bestListSHA, headSHA, bestKey)
	}
	return obj, bestDay, nil
}

// RunRestoreDrill 执行一次恢复演练骨架。
// force=true 时跳过周日窗口（验收）；runKey 空=今天（上海）。
// 不持私钥、不做真 pg_restore；解密步骤走 deps.Decrypt。
func RunRestoreDrill(ctx context.Context, runKey string, force bool, deps *RestoreDrillDeps) (*RestoreDrillResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deps == nil {
		deps = &RestoreDrillDeps{}
	}
	d := *deps
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Decrypt == nil {
		// 生产：配置了 ADTY_RESTORE_DRILL_IDENTITY_FILE 且文件存在 → 真 age+pg_restore；否则仍是未接线桩。
		d.Decrypt = RestoreDecryptFromEnv()
	}
	now := d.Now()
	if runKey == "" {
		runKey = now.In(ShanghaiLocation()).Format("2006-01-02")
	}
	out := &RestoreDrillResult{RunKey: runKey, Detail: map[string]any{}}

	if !force && !RestoreDrillDue(now) {
		out.Skipped = true
		out.Status = OpsJobStatusOK
		out.Reason = "not_sunday_window"
		return out, nil
	}

	// 未接线已失败告警过一次：同日不再重试、不再 urgent（运维注入真 Decrypt 后允许同日重试）。
	if !force && isDefaultRestoreDecrypt(d.Decrypt) && restoreDrillNotConfiguredAlready(OpsJobRestoreDrill, runKey) {
		out.Skipped = true
		out.Status = OpsJobStatusFailed
		out.Reason = "decrypt_not_configured_already"
		return out, nil
	}

	row, skip, lock, err := beginOpsJobRun(ctx, OpsJobRestoreDrill, runKey, force)
	if err != nil {
		return nil, err
	}
	if skip {
		out.Skipped = true
		out.Status = OpsJobStatusOK
		if row != nil {
			out.Status = row.Status
		}
		out.Reason = "already_ok"
		return out, nil
	}
	defer lock.Release()

	fail := func(stage string, e error) (*RestoreDrillResult, error) {
		out.Status = OpsJobStatusFailed
		out.Error = stage + ": " + e.Error()
		detail := map[string]any{
			"stage": stage, "error": out.Error,
			"object_key": out.ObjectKey, "backup_day": out.BackupDay,
		}
		_ = finishOpsJobRun(row, OpsJobStatusFailed, detail)
		pushOpsAlert(OpsAlertLevelUrgent, restoreDrillKindFailed,
			fmt.Sprintf("[adtyai] 恢复演练失败 %s（%s）：%s", runKey, stage, e.Error()),
			map[string]any{"run_key": runKey, "stage": stage})
		return out, nil
	}

	if err := d.fill(); err != nil {
		return fail("config", err)
	}

	obj, backupDay, err := FindLatestDailyBackupObject(ctx, d.Store)
	if err != nil {
		return fail("locate", err)
	}
	out.ObjectKey = obj.Key
	out.Size = obj.Size
	out.SHA256 = obj.SHA256
	out.BackupDay = backupDay
	touchOpsJobHeartbeat(row)

	extra, err := d.Decrypt(ctx, obj)
	if err != nil {
		return fail("decrypt", err)
	}
	detail := map[string]any{
		"object_key": obj.Key,
		"size":       obj.Size,
		"sha256":     obj.SHA256,
		"backup_day": backupDay,
		"skeleton":   true, // 真实现会在 extra 里覆盖为 false
	}
	for k, v := range extra {
		detail[k] = v
	}
	if err := finishOpsJobRun(row, OpsJobStatusOK, detail); err != nil {
		return out, err
	}
	out.Status = OpsJobStatusOK
	out.Detail = detail
	return out, nil
}
