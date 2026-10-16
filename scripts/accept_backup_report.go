//go:build ignore

// B12 备份 + B13 经营日报验收（云 PG；不连真 R2、不推真 Telegram）。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func fakeDump(ctx context.Context, w io.Writer) error {
	_, err := io.WriteString(w, "PGDMP fake dump "+time.Now().String())
	return err
}

func fakeEncrypt(ctx context.Context, r io.Reader, w io.Writer) error {
	_, _ = io.WriteString(w, "age-encryption.org/v1\n")
	_, err := io.Copy(w, r)
	return err
}

func main() {
	if os.Getenv("ADTY_SECRET_KEY") == "" {
		_ = os.Setenv("ADTY_SECRET_KEY", "adtyai-accept-test-secret-key-do-not-use-prod")
	}
	common.InitAdtySecretKeys()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	must(model.EnsureLedgerSchema(model.DB))
	must(model.EnsureOpsJobRunsTable(model.DB))
	must(model.EnsureOpsJobRunsTable(model.DB)) // 幂等
	must(model.EnsureOpsAlertTable(model.DB))
	ctx := context.Background()

	var def string
	must(model.DB.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='ops_job_runs_job_check'`).Scan(&def).Error)
	if !strings.Contains(def, "daily_report") || !strings.Contains(def, "backup") {
		panic("(0) job CHECK missing daily_report: " + def)
	}
	fmt.Println("(0) ops_job_runs CHECK includes daily_report OK")

	// ---- B12 backup ----
	loc := model.ShanghaiLocation()
	now := time.Now().In(loc)
	runKey := now.Format("2006-01-02")
	_ = model.DeleteOpsJobRunForTest(model.OpsJobBackup, runKey)

	store := model.NewMemBackupStore()
	old := now.AddDate(0, 0, -(model.GetBackupRetentionDays() + 3)).Format("2006-01-02")
	recent := now.AddDate(0, 0, -1).Format("2006-01-02")
	oldKey := model.BackupObjectKey(old)
	recentKey := model.BackupObjectKey(recent)
	preKeys := []string{"pre-v1.0/adtyai-pre.dump.age", "daily/pre-upgrade/x.dump.age", "daily/" + old + "/pre-v2.dump.age"}
	for _, k := range append([]string{oldKey, recentKey}, preKeys...) {
		store.Objects[k] = model.BackupObject{Key: k, Size: 1, SHA256: "x"}
	}

	deps := &model.BackupDeps{Dump: fakeDump, Encrypt: fakeEncrypt, Store: store}
	r, err := model.RunDailyBackup(ctx, runKey, true, deps)
	must(err)
	if r.Status != model.OpsJobStatusOK || r.Size == 0 || len(r.SHA256) != 64 {
		b, _ := json.Marshal(r)
		panic("(1) backup not ok: " + string(b))
	}
	row, err := model.GetOpsJobRun(model.OpsJobBackup, runKey)
	must(err)
	if row.Status != model.OpsJobStatusOK || !strings.Contains(row.Detail, r.SHA256) || !strings.Contains(row.Detail, "retention_days") {
		panic("(1) ops_job_runs backup row bad: " + row.Detail)
	}
	fmt.Printf("(1) backup ok key=%s size=%d sha=%s…\n", r.ObjectKey, r.Size, r.SHA256[:12])

	if _, ok := store.Objects[oldKey]; ok {
		panic("(2) old daily object not deleted")
	}
	if _, ok := store.Objects[recentKey]; !ok {
		panic("(2) recent object wrongly deleted")
	}
	for _, k := range preKeys {
		if _, ok := store.Objects[k]; !ok {
			panic("(2) pre-* object deleted: " + k)
		}
	}
	fmt.Printf("(2) retention deleted=%v pre-* kept=%d OK\n", r.Deleted, len(preKeys))

	r2, err := model.RunDailyBackup(ctx, runKey, false, deps)
	must(err)
	if !r2.Skipped {
		panic("(3) second run same day should skip")
	}
	fmt.Println("(3) same-day rerun skipped OK")

	// last_backup wired
	yday := now.AddDate(0, 0, -2).Format("2006-01-02")
	rec, err := model.RunDailyReconcile(ctx, yday, true)
	if err != nil {
		panic(err)
	}
	lk, _, _ := model.LastOKOpsJobRunKey(model.OpsJobBackup)
	if rec.LastBackup == nil || *rec.LastBackup != lk {
		panic("(4) reconcile last_backup not wired")
	}
	fmt.Printf("(4) reconcile last_backup=%s OK\n", *rec.LastBackup)

	// fail path → failed + urgent alert hook
	var sent []string
	model.OpsTelegramSender = func(level, kind, text string) error {
		sent = append(sent, level+"|"+kind+"|"+text)
		return nil
	}
	failKey := "2000-01-01"
	_ = model.DeleteOpsJobRunForTest(model.OpsJobBackup, failKey)
	fs := model.NewMemBackupStore()
	fs.FailPut = true
	rf, err := model.RunDailyBackup(ctx, failKey, true, &model.BackupDeps{Dump: fakeDump, Encrypt: fakeEncrypt, Store: fs})
	must(err)
	frow, _ := model.GetOpsJobRun(model.OpsJobBackup, failKey)
	if rf.Status != model.OpsJobStatusFailed || frow == nil || frow.Status != model.OpsJobStatusFailed {
		panic("(5) upload fail should record failed")
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "urgent|backup_failed|") {
		panic(fmt.Sprintf("(5) urgent alert not pushed: %v", sent))
	}
	// dump fail
	sent = nil
	_ = model.DeleteOpsJobRunForTest(model.OpsJobBackup, failKey)
	_, err = model.RunDailyBackup(ctx, failKey, true, &model.BackupDeps{
		Dump:    func(context.Context, io.Writer) error { return errors.New("forced dump fail") },
		Encrypt: fakeEncrypt, Store: model.NewMemBackupStore()})
	must(err)
	if len(sent) != 1 || !strings.Contains(sent[0], "dump") {
		panic(fmt.Sprintf("(5b) dump fail alert: %v", sent))
	}
	_ = model.DeleteOpsJobRunForTest(model.OpsJobBackup, failKey)
	if lk2, _, _ := model.LastOKOpsJobRunKey(model.OpsJobBackup); lk2 != lk {
		panic("(5c) failed run must not move last_backup")
	}
	fmt.Println("(5) upload/dump fail → failed + urgent Telegram hook OK; last_backup unchanged")

	// ---- B13 daily report ----
	date := now.AddDate(0, 0, -1).Format("2006-01-02")
	_ = model.DeleteOpsJobRunForTest(model.OpsJobDailyReport, date)
	var texts []string
	sendFn := func(t string) error { texts = append(texts, t); return nil }
	d1, err := model.RunDailyReport(ctx, date, false, sendFn)
	must(err)
	if d1.Status != model.OpsJobStatusOK || d1.Report == nil || len(texts) != 1 {
		panic("(6) report not sent")
	}
	b, _ := json.Marshal(d1.Report)
	var m map[string]any
	must(json.Unmarshal(b, &m))
	for _, k := range []string{"date", "topup_paid_usd", "consume_quota", "new_users", "reconcile_status",
		"channel_abnormal", "pending_withdrawals", "pending_risk_reviews"} {
		if _, ok := m[k]; !ok {
			panic("(6) missing key " + k)
		}
	}
	low := strings.ToLower(string(b) + texts[0])
	for _, bad := range []string{"margin", "profit", "cost", "毛利", "利润", "成本"} {
		if strings.Contains(low, bad) {
			panic("(6) report contains forbidden field " + bad)
		}
	}
	fmt.Printf("(6) report keys OK, no margin fields\n%s\n", texts[0])

	d2, err := model.RunDailyReport(ctx, date, false, sendFn)
	must(err)
	if !d2.Skipped || len(texts) != 1 {
		panic("(7) second report same day should skip")
	}
	fmt.Println("(7) same-date report skipped OK")

	// send fail → failed, retry later sends
	fd := "2000-01-02"
	_ = model.DeleteOpsJobRunForTest(model.OpsJobDailyReport, fd)
	if _, err := model.RunDailyReport(ctx, fd, false, func(string) error { return errors.New("tg down") }); err == nil {
		panic("(8) send fail should error")
	}
	if rr, _ := model.GetOpsJobRun(model.OpsJobDailyReport, fd); rr == nil || rr.Status != model.OpsJobStatusFailed {
		panic("(8) send fail should record failed")
	}
	d3, err := model.RunDailyReport(ctx, fd, false, sendFn)
	must(err)
	if d3.Status != model.OpsJobStatusOK {
		panic("(8) retry should succeed")
	}
	_ = model.DeleteOpsJobRunForTest(model.OpsJobDailyReport, fd)
	fmt.Println("(8) send fail → failed, retry ok")

	// schedule window
	rt, err := time.Parse("15:04", model.GetDailyReportTime())
	must(err)
	at := func(h, mi int) time.Time { return time.Date(2026, 1, 2, h, mi, 0, 0, loc) }
	edge := at(rt.Hour(), rt.Minute())
	if model.DailyReportDue(edge.Add(-time.Minute)) || !model.DailyReportDue(edge) || !model.DailyReportDue(at(23, 59)) {
		panic("(9) DailyReportTime window wrong")
	}
	fmt.Printf("(9) DailyReportTime %s window OK\n", model.GetDailyReportTime())

	// handlers registered
	controller.RegisterScheduledSystemTasks()
	for _, t := range []string{model.SystemTaskTypeBackup, model.SystemTaskTypeDailyReport} {
		h, ok := service.LookupSystemTaskHandler(t)
		if !ok || h == nil {
			panic("(10) handler missing " + t)
		}
	}
	if model.OpsTelegramSender == nil {
		panic("(10) OpsTelegramSender not wired")
	}
	fmt.Println("(10) backup/daily_report system tasks registered; Telegram sender wired")
	// 清理假备份/假日报行：不让云库 last_backup 显示假成功，也不挡当天真任务
	must(model.DeleteOpsJobRunForTest(model.OpsJobBackup, runKey))
	must(model.DeleteOpsJobRunForTest(model.OpsJobDailyReport, date))
	fmt.Println("(11) cleanup fake backup/report rows OK")
	fmt.Println("ACCEPT_BACKUP_REPORT_PASS")
}
