package controller

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// RegisterScheduledSystemTasks wires the periodic channel test, upstream model
// update, async task polling (Midjourney / Suno / video), and ledger ops
// (hourly_rollup / partition_precreate / probe_cleanup / daily_reconcile) jobs into the system task framework so
// a DB lease dedups execution across multiple master instances and each run is
// recorded as one task row. Call this before service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
	service.RegisterSystemTaskHandler(hourlyRollupHandler{})
	service.RegisterSystemTaskHandler(partitionPrecreateHandler{})
	service.RegisterSystemTaskHandler(probeCleanupHandler{})
	service.RegisterSystemTaskHandler(dailyReconcileHandler{})
	service.RegisterSystemTaskHandler(backupHandler{})
	service.RegisterSystemTaskHandler(dailyReportHandler{})
	service.RegisterSystemTaskHandler(restoreDrillHandler{})
	// B12/B13：model 层告警走 A43 Telegram 通道（model 不能直接引用 service）。
	model.OpsTelegramSender = service.AdtySendTelegramAlert
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// hourlyRollupHandler runs ledger hourly API-consume rollup backfill.
// Cadence ≈ 1h (after each closed hour); ops_job_runs still dedups per hour bucket.
type hourlyRollupHandler struct{}

func (hourlyRollupHandler) Type() string { return model.SystemTaskTypeHourlyRollup }

func (hourlyRollupHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("LEDGER_HOURLY_ROLLUP_TASK_ENABLED", true)
}

func (hourlyRollupHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("LEDGER_HOURLY_ROLLUP_TASK_INTERVAL_MINUTES", 60)
	if minutes < 1 {
		minutes = 60
	}
	return time.Duration(minutes) * time.Minute
}

func (hourlyRollupHandler) NewPayload() any { return nil }

func (hourlyRollupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := model.RunHourlyRollupBackfill(ctx, nil)
	if err != nil {
		common.SysError(common.LogText("system task hourly_rollup failed: %v", err))
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// partitionPrecreateHandler ensures current+next month ledger partitions exist.
// Cadence ≈ 24h; RunPartitionPrecreate also dedups via ops_job_runs run_key=YYYY-MM-DD.
type partitionPrecreateHandler struct{}

func (partitionPrecreateHandler) Type() string { return model.SystemTaskTypePartitionPrecreate }

func (partitionPrecreateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("LEDGER_PARTITION_PRECREATE_TASK_ENABLED", true)
}

func (partitionPrecreateHandler) Interval() time.Duration {
	hours := common.GetEnvOrDefault("LEDGER_PARTITION_PRECREATE_TASK_INTERVAL_HOURS", 24)
	if hours < 1 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

func (partitionPrecreateHandler) NewPayload() any { return nil }

func (partitionPrecreateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	err := model.RunPartitionPrecreate(ctx)
	if err != nil {
		common.SysError(common.LogText("system task partition_precreate failed: %v", err))
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, nil, nil)
}


// probeCleanupHandler runs daily health_probes retention cleanup (A18).
// Cadence ≈ 24h; RunProbeCleanup dedups via ops_job_runs run_key=YYYY-MM-DD.
type probeCleanupHandler struct{}

func (probeCleanupHandler) Type() string { return model.SystemTaskTypeProbeCleanup }

func (probeCleanupHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("HEALTH_PROBE_CLEANUP_TASK_ENABLED", true)
}

func (probeCleanupHandler) Interval() time.Duration {
	hours := common.GetEnvOrDefault("HEALTH_PROBE_CLEANUP_TASK_INTERVAL_HOURS", 24)
	if hours < 1 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

func (probeCleanupHandler) NewPayload() any { return nil }

func (probeCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := model.RunProbeCleanup(ctx, false)
	if err != nil {
		common.SysError(common.LogText("system task probe_cleanup failed: %v", err))
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}


// dailyReconcileHandler runs B6 daily reconcile (~00:30 Asia/Shanghai).
// Cadence 30m; ops_job_runs job=daily_reconcile run_key=YYYY-MM-DD dedups.
// Skips wall-clock ticks before 00:30 Shanghai so the first eligible tick is ~00:30.
type dailyReconcileHandler struct{}

func (dailyReconcileHandler) Type() string { return model.SystemTaskTypeDailyReconcile }

func (dailyReconcileHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("LEDGER_DAILY_RECONCILE_TASK_ENABLED", true)
}

func (dailyReconcileHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("LEDGER_DAILY_RECONCILE_TASK_INTERVAL_MINUTES", 30)
	if minutes < 1 {
		minutes = 30
	}
	return time.Duration(minutes) * time.Minute
}

func (dailyReconcileHandler) NewPayload() any { return nil }

func (dailyReconcileHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	now := time.Now().In(model.ShanghaiLocation())
	if now.Hour() == 0 && now.Minute() < 30 {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, map[string]any{
			"skipped": true, "reason": "before_0030_shanghai",
		}, nil)
		return
	}
	date := model.YesterdayBusinessDate()
	summary, err := model.RunDailyReconcile(ctx, date, false)
	if err != nil {
		common.SysError(common.LogText("system task daily_reconcile failed: %v", err))
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// backupHandler B12 每日备份（03:00 上海起首个 tick）。ops_job_runs job=backup run_key=上海日期 去重。
type backupHandler struct{}

func (backupHandler) Type() string { return model.SystemTaskTypeBackup }

func (backupHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("ADTY_BACKUP_TASK_ENABLED", true)
}

func (backupHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("ADTY_BACKUP_TASK_INTERVAL_MINUTES", 30)
	if minutes < 1 {
		minutes = 30
	}
	return time.Duration(minutes) * time.Minute
}

func (backupHandler) NewPayload() any { return nil }

func (backupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	now := time.Now().In(model.ShanghaiLocation())
	if now.Hour() < 3 {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, map[string]any{
			"skipped": true, "reason": "before_0300_shanghai",
		}, nil)
		return
	}
	summary, err := model.RunDailyBackup(ctx, "", false, nil)
	if err == nil && summary != nil && summary.Status == model.OpsJobStatusFailed {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, errors.New(summary.Error))
		return
	}
	if err != nil {
		common.SysError(common.LogText("system task backup failed: %v", err))
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// dailyReportHandler B13 经营日报（DailyReportTime 上海起首个 tick），报昨天。
type dailyReportHandler struct{}

func (dailyReportHandler) Type() string { return model.SystemTaskTypeDailyReport }

func (dailyReportHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("ADTY_DAILY_REPORT_TASK_ENABLED", true)
}

func (dailyReportHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("ADTY_DAILY_REPORT_TASK_INTERVAL_MINUTES", 15)
	if minutes < 1 {
		minutes = 15
	}
	return time.Duration(minutes) * time.Minute
}

func (dailyReportHandler) NewPayload() any { return nil }

func (dailyReportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if !model.DailyReportDue(time.Now()) {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, map[string]any{
			"skipped": true, "reason": "before_daily_report_time",
		}, nil)
		return
	}
	summary, err := model.RunDailyReport(ctx, "", false, nil)
	if err != nil {
		common.SysError(common.LogText("system task daily_report failed: %v", err))
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}


// restoreDrillHandler B12 每周日恢复演练骨架（默认关闭；ADTY_RESTORE_DRILL_TASK_ENABLED=true 开启）。
// 配了 ADTY_RESTORE_DRILL_IDENTITY_FILE（文件存在）→ RunRestoreDrill 内用 RestoreDecryptFromEnv 走真 age -d + 临时库 pg_restore；
// 未配时 Decrypt 返回 restore_decrypt_not_configured → urgent（同日只一次）。
type restoreDrillHandler struct{}

func (restoreDrillHandler) Type() string { return model.SystemTaskTypeRestoreDrill }

func (restoreDrillHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("ADTY_RESTORE_DRILL_TASK_ENABLED", false)
}

func (restoreDrillHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("ADTY_RESTORE_DRILL_TASK_INTERVAL_MINUTES", 60)
	if minutes < 1 {
		minutes = 60
	}
	return time.Duration(minutes) * time.Minute
}

func (restoreDrillHandler) NewPayload() any { return nil }

func (restoreDrillHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	now := time.Now()
	if !model.RestoreDrillDue(now) {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, map[string]any{
			"skipped": true, "reason": "not_sunday_window",
		}, nil)
		return
	}
	summary, err := model.RunRestoreDrill(ctx, "", false, nil)
	if err == nil && summary != nil && summary.Status == model.OpsJobStatusFailed {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, errors.New(summary.Error))
		return
	}
	if err != nil {
		common.SysError(common.LogText("system task restore_drill failed: %v", err))
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(common.LogText("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
