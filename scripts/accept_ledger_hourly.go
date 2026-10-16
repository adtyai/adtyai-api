//go:build ignore

package main

import (
	"context"
	"fmt"
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

func main() {
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())

	// 不绕账本留脏数据：本脚本会直接改余额/插流水来造场景，跑前跑后都清掉自己造的 accept 用户及其流水/订单等，
	// 保证对账（collectUserBalanceDiffs）不留不平用户。清理事务内临时关账本防篡改触发器，不能和联调写入同时跑。
	acceptUserPrefixes := []string{"accept-ledger-"}
	acceptOrderPrefixes := []string{}
	if n, err := model.CleanupAcceptUsersForTest(acceptUserPrefixes, acceptOrderPrefixes); err != nil {
		panic(fmt.Sprintf("pre-clean accept data: %v", err))
	} else {
		fmt.Printf("pre-clean accept data users=%d\n", n)
	}
	defer func() {
		if n, err := model.CleanupAcceptUsersForTest(acceptUserPrefixes, acceptOrderPrefixes); err != nil {
			fmt.Printf("post-clean accept data failed: %v\n", err)
		} else {
			fmt.Printf("post-clean accept data users=%d\n", n)
		}
	}()
	must(model.EnsureLedgerSchema(model.DB))
	must(model.EnsureOpsJobRunsTable(model.DB))
	must(model.EnsureOpsAlertTable(model.DB))
	fmt.Println("InitDB + ledger schema + ops_job_runs OK")

	// (1) partitioned + idempotency
	ok, err := model.IsLedgerPartitioned(model.DB)
	must(err)
	if !ok {
		panic("ledger_entries not partitioned")
	}
	if !model.DB.Migrator().HasTable(&model.LedgerIdempotency{}) {
		panic("ledger_idempotency missing")
	}
	if !model.DB.Migrator().HasTable(&model.OpsJobRun{}) {
		panic("ops_job_runs missing")
	}
	must(model.LedgerSchemaOK(model.DB))
	fmt.Println("(1) partition + ledger_idempotency + ops_job_runs + trigger OK")

	// (2) UPDATE/DELETE rejected
	u := model.User{
		Username:    "accept-ledger-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       10000,
		AffCode:     "lg" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	entry, err := model.AdjustQuota(nil, u.Id, "quota", 100, "seed", "accept_seed", "s1", "", 0)
	must(err)
	seedId := entry.Entry.Id

	upd := model.DB.Exec(`UPDATE ledger_entries SET reason = 'hack' WHERE id = ?`, seedId)
	if upd.Error == nil {
		panic("(2) UPDATE should fail")
	}
	if !strings.Contains(strings.ToLower(upd.Error.Error()), "append-only") &&
		!strings.Contains(strings.ToLower(upd.Error.Error()), "not allowed") {
		panic(fmt.Sprintf("(2) UPDATE unexpected err: %v", upd.Error))
	}
	del := model.DB.Exec(`DELETE FROM ledger_entries WHERE id = ?`, seedId)
	if del.Error == nil {
		panic("(2) DELETE should fail")
	}
	fmt.Println("(2) UPDATE/DELETE ledger_entries rejected OK")

	// (3) hourly_rollup write + backfill + no dup
	hb := model.TruncateToHourUTC(time.Now().UTC().Add(-2 * time.Hour))
	must(model.SeedConsumeLogForTest(u.Id, 700, hb.Add(10*time.Minute)))
	must(model.SeedConsumeLogForTest(u.Id, 300, hb.Add(40*time.Minute)))

	ctx := context.Background()
	r1, err := model.RunHourlyRollupBackfill(ctx, &hb)
	must(err)
	if r1.HoursFailed > 0 {
		panic(fmt.Sprintf("rollup failed: %+v", r1))
	}
	n, err := model.CountHourlyRollupRows(u.Id, hb)
	must(err)
	if n != 1 {
		panic(fmt.Sprintf("want 1 rollup row got %d", n))
	}
	sum, err := model.SumHourlyRollupDelta(u.Id, hb)
	must(err)
	if sum != -1000 {
		panic(fmt.Sprintf("want delta -1000 got %d", sum))
	}
	fmt.Println("(3a) hourly_rollup wrote summary row OK")

	// delete + backfill
	must(model.DeleteHourlyRollupRowsForTest(u.Id, hb))
	n, _ = model.CountHourlyRollupRows(u.Id, hb)
	if n != 0 {
		panic("delete rollup failed")
	}
	r2, err := model.RunHourlyRollupBackfill(ctx, &hb)
	must(err)
	_ = r2
	n, err = model.CountHourlyRollupRows(u.Id, hb)
	must(err)
	if n != 1 {
		panic(fmt.Sprintf("backfill want 1 got %d", n))
	}
	fmt.Println("(3b) delete + backfill restored OK")

	// repeat no dup
	r3, err := model.RunHourlyRollupBackfill(ctx, &hb)
	must(err)
	_ = r3
	n, err = model.CountHourlyRollupRows(u.Id, hb)
	must(err)
	if n != 1 {
		panic(fmt.Sprintf("repeat want 1 got %d", n))
	}
	fmt.Println("(3c) repeat rollup no duplicate OK")

	// (4) same (job,run_key) only one row; attempt may increase on force
	runKey := model.HourBucketRunKey(hb)
	cnt, err := model.CountOpsJobRuns(model.OpsJobHourlyRollup, runKey)
	must(err)
	if cnt != 1 {
		panic(fmt.Sprintf("ops_job_runs want 1 got %d", cnt))
	}
	jobRow, err := model.GetOpsJobRun(model.OpsJobHourlyRollup, runKey)
	must(err)
	if jobRow.Status != model.OpsJobStatusOK {
		panic(fmt.Sprintf("status want ok got %s", jobRow.Status))
	}
	dupErr := model.TryInsertDuplicateOpsJobRun(model.OpsJobHourlyRollup, runKey)
	if dupErr == nil {
		panic("duplicate (job,run_key) should fail")
	}
	fmt.Printf("(4) ops_job_runs unique OK (attempt=%d)\n", jobRow.Attempt)

	// (5) idempotency replay
	idem := "accept-ledger-idem-" + common.GetRandomString(8)
	a1, err := model.AdjustQuota(nil, u.Id, "quota", 50, "idem-a", "accept_idem", "i1", idem, 0)
	must(err)
	a2, err := model.AdjustQuota(nil, u.Id, "quota", 50, "idem-b", "accept_idem", "i1", idem, 0)
	must(err)
	if !a2.Replay {
		panic("second AdjustQuota should be replay")
	}
	if a1.Entry.Id != a2.Entry.Id {
		panic("idem replay entry id mismatch")
	}
	fmt.Println("(5) ledger_idempotency replay OK")

	// (6) partition_precreate
	must(model.RunPartitionPrecreate(ctx))
	parts, err := model.ListLedgerPartitions()
	must(err)
	if len(parts) < 2 {
		panic(fmt.Sprintf("want >=2 partitions got %v", parts))
	}
	fmt.Printf("(6) partition_precreate OK parts=%v\n", parts)

	// smoke: insert still works after partition
	_, err = model.AdjustQuota(nil, u.Id, "bonus", 10, "post-part", "accept", "p1", "", 0)
	must(err)

	// (7) system task handlers registered (cron mount)
	controller.RegisterScheduledSystemTasks()
	for _, typ := range []string{model.SystemTaskTypeHourlyRollup, model.SystemTaskTypePartitionPrecreate} {
		h, ok := service.LookupSystemTaskHandler(typ)
		if !ok || h == nil {
			panic(fmt.Sprintf("(7) handler %s not registered", typ))
		}
		sch, ok := h.(service.ScheduledSystemTaskHandler)
		if !ok {
			panic(fmt.Sprintf("(7) handler %s is not ScheduledSystemTaskHandler", typ))
		}
		if !sch.Enabled() {
			panic(fmt.Sprintf("(7) handler %s Enabled=false (default should be true)", typ))
		}
		if sch.Interval() <= 0 {
			panic(fmt.Sprintf("(7) handler %s Interval invalid", typ))
		}
	}
	hr, _ := service.LookupSystemTaskHandler(model.SystemTaskTypeHourlyRollup)
	pr, _ := service.LookupSystemTaskHandler(model.SystemTaskTypePartitionPrecreate)
	if hr.(service.ScheduledSystemTaskHandler).Interval() != time.Hour {
		panic(fmt.Sprintf("(7) hourly_rollup Interval want 1h got %v", hr.(service.ScheduledSystemTaskHandler).Interval()))
	}
	if pr.(service.ScheduledSystemTaskHandler).Interval() != 24*time.Hour {
		panic(fmt.Sprintf("(7) partition_precreate Interval want 24h got %v", pr.(service.ScheduledSystemTaskHandler).Interval()))
	}
	// smoke: Run via handler path (no panic; Finish needs claimed task — call model path already covered)
	fmt.Printf("(7) system task handlers registered OK (hourly_rollup=%v partition_precreate=%v)\n",
		hr.(service.ScheduledSystemTaskHandler).Interval(),
		pr.(service.ScheduledSystemTaskHandler).Interval())

	fmt.Println("ACCEPT_LEDGER_HOURLY_PASS")
}
