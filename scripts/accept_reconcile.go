//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	acceptUserPrefix  = "accept-recon-"
	acceptOrderPrefix = "ACCEPT-RECON-"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func adminCtx(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-reconcile-admin")
	return c, w
}

func seedDayBucketsOK(day time.Time) []time.Time {
	buckets := model.BusinessDayHourBuckets(day)
	for _, hb := range buckets {
		must(model.SeedOpsHourlyRollupOKForTest(hb))
	}
	return buckets
}

func clearDaily(date string) {
	_ = model.DeleteOpsJobRunForTest(model.OpsJobDailyReconcile, date)
}

func containsInt(ids []int, id int) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func main() {
	if os.Getenv("ADTY_SECRET_KEY") == "" {
		_ = os.Setenv("ADTY_SECRET_KEY", "adtyai-accept-test-secret-key-do-not-use-prod")
	}
	common.InitAdtySecretKeys()
	// 本脚本会建用户/流水/小时桶并做清理：任何 DB 操作前先要求近期可读备份
	must(model.RequireRecentAcceptBackup())
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	must(model.EnsureLedgerSchema(model.DB))
	must(model.EnsureOpsJobRunsTable(model.DB))
	must(model.EnsureOpsAlertTable(model.DB))
	fmt.Println("InitDB + ledger + ops_job_runs OK")

	ctx := context.Background()
	loc := model.ShanghaiLocation()
	day := time.Now().In(loc).AddDate(0, 0, -2)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	dateStr := day.Format("2006-01-02")
	clearDaily(dateStr)

	// 可重复跑：先清掉历次自造的用户/流水/订单/日志，避免旧数据挤掉本次断言（第 3 条首遍挂）
	// 注意：清理会临时关账本防篡改触发器，不能和联调写入同时跑。
	cleaned, err := model.CleanupAcceptReconcileDataForTest(acceptUserPrefix, acceptOrderPrefix)
	must(err)
	fmt.Printf("pre-clean accept data users=%d\n", cleaned)
	defer func() {
		if n, e := model.CleanupAcceptReconcileDataForTest(acceptUserPrefix, acceptOrderPrefix); e != nil {
			fmt.Printf("post-clean failed: %v\n", e)
		} else {
			fmt.Printf("post-clean accept data users=%d\n", n)
		}
		clearDaily(dateStr)
	}()

	// --- (1) 24 bucket 齐 + 本用户账平 ---
	buckets := seedDayBucketsOK(day)
	if len(buckets) != 24 {
		panic(fmt.Sprintf("(1) want 24 buckets got %d", len(buckets)))
	}

	u := model.User{
		Username:    acceptUserPrefix + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "rc" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	_, err = model.AdjustQuota(nil, u.Id, "paid", 5000, "seed paid", "accept_recon", "s1", "", 0)
	must(err)
	_, err = model.AdjustQuota(nil, u.Id, "bonus", 2000, "seed bonus", "accept_recon", "s2", "", 0)
	must(err)
	must(model.SeedConsumeLogForTest(u.Id, 100, time.Now().UTC()))
	must(model.DB.Exec(`UPDATE users SET quota = quota - 100 WHERE id = ? AND quota >= 100`, u.Id).Error)

	udiff, err := model.UserBalanceDiff(u.Id)
	must(err)
	if udiff != 0 {
		panic(fmt.Sprintf("(1) user balance diff want 0 got %d", udiff))
	}

	r1, err := model.RunDailyReconcile(ctx, dateStr, true)
	must(err)
	if len(r1.MissingBuckets) != 0 {
		panic(fmt.Sprintf("(1) missing_buckets should be empty got %v", r1.MissingBuckets))
	}
	if r1.Status == model.ReconcileStatusIncomplete || r1.Status == model.ReconcileStatusFailed {
		b, _ := json.Marshal(r1)
		panic(fmt.Sprintf("(1) unexpected status body=%s", string(b)))
	}
	// B12：last_backup = LastOKOpsJobRunKey("backup")，无成功备份时为 null
	if lk, lok, e := model.LastOKOpsJobRunKey(model.OpsJobBackup); e != nil {
		panic(e)
	} else if lok != (r1.LastBackup != nil) || (lok && *r1.LastBackup != lk) {
		panic("(1) last_backup should match last ok backup run_key")
	}
	if containsInt(r1.MismatchUserIDs, u.Id) {
		panic("(1) accept user should not be in mismatch_user_ids when books match")
	}
	// 云库可能有历史不平用户 → 全局可能 mismatch；本用户必须平
	fmt.Printf("(1) 24 buckets OK date=%s status=%s user_diff=0 global_mismatch_users=%d\n",
		dateStr, r1.Status, r1.MismatchUsers)

	c, w := adminCtx(http.MethodGet, "/api/admin/reconcile?date="+dateStr)
	controller.AdminGetReconcile(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1b) GET want 200 got %d %s", w.Code, w.Body.String()))
	}
	var getResp struct {
		Success bool                  `json:"success"`
		Data    model.ReconcileResult `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Success {
		panic(fmt.Sprintf("(1b) GET success=false: %s", w.Body.String()))
	}
	// force 缺省=false：r1 已跑完当日（ops_job_runs=ok，与对账结论无关）→ 应 skipped
	if !getResp.Data.Skipped {
		panic(fmt.Sprintf("(1b) force 缺省应为 false（当日已跑应 skipped）: %s", w.Body.String()))
	}
	cf, wf := adminCtx(http.MethodGet, "/api/admin/reconcile?date="+dateStr+"&force=true")
	controller.AdminGetReconcile(cf)
	var forceResp struct {
		Success bool                  `json:"success"`
		Data    model.ReconcileResult `json:"data"`
	}
	must(json.Unmarshal(wf.Body.Bytes(), &forceResp))
	if wf.Code != 200 || !forceResp.Success || forceResp.Data.Skipped {
		panic(fmt.Sprintf("(1b) force=true 应重跑: %d %s", wf.Code, wf.Body.String()))
	}
	fmt.Println("(1b) GET /api/admin/reconcile OK (default skip, force=true rerun)")

	// --- (2a) 缺 bucket → 补跑后齐 ---
	clearDaily(dateStr)
	victim := buckets[0]
	victimKey := model.HourBucketRunKey(victim)
	must(model.DeleteOpsJobRunForTest(model.OpsJobHourlyRollup, victimKey))

	r2, err := model.RunDailyReconcile(ctx, dateStr, true)
	must(err)
	if r2.Status == model.ReconcileStatusIncomplete {
		b, _ := json.Marshal(r2)
		panic(fmt.Sprintf("(2a) backfill should clear missing body=%s", string(b)))
	}
	if len(r2.MissingBuckets) != 0 {
		panic(fmt.Sprintf("(2a) missing still %v", r2.MissingBuckets))
	}
	if r2.BackfillStatus != model.BackfillOK && r2.BackfillStatus != model.BackfillRan {
		// after successful clear, BackfillOK
		fmt.Printf("(2a) note backfill_status=%s\n", r2.BackfillStatus)
	}
	fmt.Printf("(2a) missing bucket backfilled OK backfill_status=%s\n", r2.BackfillStatus)

	// --- (2b) incomplete：锁住补跑 ---
	clearDaily(dateStr)
	must(model.DeleteOpsJobRunForTest(model.OpsJobHourlyRollup, victimKey))
	release, held, err := model.HoldOpsJobLockForTest(ctx, model.OpsJobHourlyRollup, victimKey)
	must(err)
	if !held {
		panic("(2b) failed to hold ops lock")
	}
	r2b, err := model.RunDailyReconcile(ctx, dateStr, true)
	release()
	must(err)
	if r2b.Status != model.ReconcileStatusIncomplete {
		b, _ := json.Marshal(r2b)
		panic(fmt.Sprintf("(2b) want incomplete got %s body=%s", r2b.Status, string(b)))
	}
	if len(r2b.MissingBuckets) == 0 {
		panic("(2b) missing_buckets empty")
	}
	fmt.Printf("(2b) incomplete when backfill locked OK missing=%v\n", r2b.MissingBuckets)
	must(model.SeedOpsHourlyRollupOKForTest(victim))

	// --- (3) 人为不平能检出且不改账 ---
	clearDaily(dateStr)
	seedDayBucketsOK(day)
	var quotaBefore int
	must(model.DB.Model(&model.User{}).Where("id = ?", u.Id).Select("quota").Scan(&quotaBefore).Error)
	must(model.DB.Exec(`UPDATE users SET quota = quota + 777 WHERE id = ?`, u.Id).Error)

	orderNo := acceptOrderPrefix + common.GetRandomString(8)
	ord := model.TopupOrder{
		OrderNo:         orderNo,
		UserId:          u.Id,
		AmountUSD:       1,
		Status:          model.TopupOrderStatusPaid,
		PaidAmountUSD:   1,
		CreditAmountUSD: 1,
		BonusAmountUSD:  0,
		RefundStatus:    model.RefundStatusNone,
		CreateTime:      time.Now().Unix(),
	}
	must(model.DB.Create(&ord).Error)

	udiff2, err := model.UserBalanceDiff(u.Id)
	must(err)
	if udiff2 == 0 {
		panic("(3) user diff should be non-zero after wallet bump")
	}

	r3, err := model.RunDailyReconcile(ctx, dateStr, true)
	must(err)
	if r3.Status != model.ReconcileStatusMismatch {
		b, _ := json.Marshal(r3)
		panic(fmt.Sprintf("(3) want mismatch got %s body=%s", r3.Status, string(b)))
	}
	// mismatch_user_ids / order_nos 各截 50；云库历史不平用户 >50 时本用户可能被截掉。
	// 截断时不靠总数，改为按同一口径取全量不平集合精确判本用户在内，并按 user_id 精确查订单号。
	listed := containsInt(r3.MismatchUserIDs, u.Id)
	if !listed {
		if r3.MismatchUsers <= len(r3.MismatchUserIDs) {
			panic(fmt.Sprintf("(3) accept user %d not in mismatch_user_ids %v (total=%d, not truncated)", u.Id, r3.MismatchUserIDs, r3.MismatchUsers))
		}
		all, err := model.AllMismatchUserIDsForTest()
		must(err)
		if !containsInt(all, u.Id) {
			panic(fmt.Sprintf("(3) accept user %d not in full mismatch set (len=%d total=%d)", u.Id, len(all), r3.MismatchUsers))
		}
		if len(all) < r3.MismatchUsers {
			panic(fmt.Sprintf("(3) full mismatch set len=%d < reported total=%d", len(all), r3.MismatchUsers))
		}
		if !containsString(model.OrderNosForUsersForTest([]int{u.Id}, 50), orderNo) {
			panic(fmt.Sprintf("(3) truncated: order %s not found for accept user %d", orderNo, u.Id))
		}
		fmt.Printf("(3) truncated path: user %d in full mismatch set (%d) and order matched\n", u.Id, len(all))
	}
	var quotaAfter int
	must(model.DB.Model(&model.User{}).Where("id = ?", u.Id).Select("quota").Scan(&quotaAfter).Error)
	if quotaAfter != quotaBefore+777 {
		panic(fmt.Sprintf("(3) reconcile must not auto-fix wallet before=%d after=%d", quotaBefore, quotaAfter))
	}
	foundOrder := false
	for _, no := range r3.OrderNos {
		if no == orderNo {
			foundOrder = true
			break
		}
	}
	if listed && !foundOrder {
		panic(fmt.Sprintf("(3) order_nos missing %s got %v", orderNo, r3.OrderNos))
	}
	fmt.Printf("(3) mismatch detected user_diff=%d no auto-fix OK order_nos has seed\n", udiff2)

	must(model.DB.Exec(`UPDATE users SET quota = ? WHERE id = ?`, quotaBefore, u.Id).Error)

	// --- (4) ops_job_runs 唯一键 ---
	clearDaily(dateStr)
	seedDayBucketsOK(day)
	_, err = model.RunDailyReconcile(ctx, dateStr, true)
	must(err)
	cnt, err := model.CountOpsJobRuns(model.OpsJobDailyReconcile, dateStr)
	must(err)
	if cnt != 1 {
		panic(fmt.Sprintf("(4) daily_reconcile rows want 1 got %d", cnt))
	}
	dupErr := model.TryInsertDuplicateOpsJobRun(model.OpsJobDailyReconcile, dateStr)
	if dupErr == nil {
		panic("(4) duplicate (job,run_key) should fail")
	}
	fmt.Println("(4) ops_job_runs unique (daily_reconcile) OK")

	// --- (5) 系统任务已注册 ---
	controller.RegisterScheduledSystemTasks()
	h, ok := service.LookupSystemTaskHandler(model.SystemTaskTypeDailyReconcile)
	if !ok || h == nil {
		panic("(5) daily_reconcile handler missing")
	}
	sch, ok := h.(service.ScheduledSystemTaskHandler)
	if !ok {
		panic("(5) not ScheduledSystemTaskHandler")
	}
	if !sch.Enabled() {
		panic("(5) Enabled=false")
	}
	if sch.Interval() != 30*time.Minute {
		panic(fmt.Sprintf("(5) Interval want 30m got %v", sch.Interval()))
	}
	fmt.Printf("(5) system task daily_reconcile registered OK interval=%v\n", sch.Interval())

	fmt.Println("ACCEPT_RECONCILE_PASS")
}

func containsString(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
