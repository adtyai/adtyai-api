//go:build ignore

// B-03 / H-A / H-B accept（PostgreSQL）：走真实 WalletFunding 预扣/结算/退回 + 小时汇总 + 日对账。
// 只能跑在临时库：要求 SQL_DSN 与 LOG_SQL_DSN 显式设置，且 current_database() == ADTY_B03_EXPECT_DB。
// 只造、只清 accept-b03- 前缀用户。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

const prefix = "accept-b03-"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func check(cond bool, f string, a ...any) {
	if !cond {
		panic(fmt.Sprintf("FAIL: "+f, a...))
	}
}

func newUser(paid, gift int) int {
	u := model.User{
		Username: prefix + common.GetRandomString(6), Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: 0,
		AffCode: "b03" + common.GetRandomString(8),
	}
	must(model.DB.Create(&u).Error)
	// 入账走 CreditUserWallet 写流水，保证对账起点平
	must(model.CreditUserWallet(u.Id, gift, paid, "accept_seed", "b03 seed", "accept", prefix+"seed"))
	return u.Id
}

func bal(uid int) (paid int, gift int64) {
	p, err := model.GetUserPaidQuota(uid)
	must(err)
	g, err := model.GetUserBonusBalance(uid)
	must(err)
	return p, g
}

func at(t time.Time) { model.SetWalletConsumeNowForTest(func() time.Time { return t }) }

func refundLog(uid, quota int, t time.Time) {
	must(model.LOG_DB.Create(&model.Log{UserId: uid, CreatedAt: t.Unix(), Type: model.LogTypeRefund,
		Content: "accept b03 task refund", Quota: quota, ModelName: "accept-model"}).Error)
}

func main() {
	expect := os.Getenv("ADTY_B03_EXPECT_DB")
	if os.Getenv("SQL_DSN") == "" || os.Getenv("LOG_SQL_DSN") == "" || expect == "" {
		panic("SQL_DSN, LOG_SQL_DSN and ADTY_B03_EXPECT_DB must all be set explicitly")
	}
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	var cur, curLog string
	must(model.DB.Raw("SELECT current_database()").Scan(&cur).Error)
	must(model.LOG_DB.Raw("SELECT current_database()").Scan(&curLog).Error)
	fmt.Printf("current_database: main=%s log=%s\n", cur, curLog)
	check(cur == expect && curLog == expect, "refusing to run: current_database main=%s log=%s expect=%s", cur, curLog, expect)

	clean := func(stage string) {
		// LOG_DB 是独立连接（即便同库），先按前缀用户删其日志，避免清理事务里的残留引用检查失败
		var ids []int
		must(model.DB.Model(&model.User{}).Unscoped().Where("username LIKE ?", prefix+"%").Pluck("id", &ids).Error)
		if len(ids) > 0 {
			must(model.LOG_DB.Where("user_id IN ?", ids).Delete(&model.Log{}).Error)
		}
		n, err := model.CleanupAcceptUsersForTest([]string{prefix}, nil)
		must(err)
		fmt.Printf("%s-clean %s users=%d\n", stage, prefix, n)
	}
	clean("pre")
	defer clean("post")
	defer model.SetWalletConsumeNowForTest(nil)

	// 昨天（上海）10:00 / 11:00 两个整点，均已过去，可完整汇总
	loc := model.ShanghaiLocation()
	y := time.Now().In(loc).AddDate(0, 0, -1)
	day := time.Date(y.Year(), y.Month(), y.Day(), 0, 0, 0, 0, loc)
	h10 := day.Add(10 * time.Hour).UTC()
	h11 := h10.Add(time.Hour)

	// R1：实充 100 / 赠送 10，预扣 30，实际 12，结算退回 18，日志 12
	r1 := newUser(100, 10)
	at(h10.Add(5 * time.Minute))
	f := service.NewWalletFundingForAccept(r1)
	must(f.PreConsume(30))
	must(f.Settle(-18))
	b, p := f.WalletSplitForAccept()
	check(b == 10 && p == 2, "R1 split bonus=%d paid=%d want 10/2", b, p)
	must(model.SeedConsumeLogForTest(r1, 12, h10.Add(5*time.Minute)))
	pp, gg := bal(r1)
	check(pp == 98 && gg == 0, "R1 balance %d/%d want 98/0", pp, gg)
	fmt.Println("R1 partial restore OK")

	// R2：预扣 30 后请求失败整笔退，无日志
	r2 := newUser(100, 10)
	f2 := service.NewWalletFundingForAccept(r2)
	must(f2.PreConsume(30))
	must(f2.Refund())
	pp, gg = bal(r2)
	check(pp == 100 && gg == 10, "R2 balance %d/%d want 100/10", pp, gg)
	fmt.Println("R2 full failure refund OK")

	// X：跨整点：10:59 预扣 20（赠送 5 + 实充 15），11:01 结算退回 8，日志 12 记在 11:01
	x := newUser(100, 5)
	at(h10.Add(59 * time.Minute))
	fx := service.NewWalletFundingForAccept(x)
	must(fx.PreConsume(20))
	at(h11.Add(1 * time.Minute))
	must(fx.Settle(-8))
	must(model.SeedConsumeLogForTest(x, 12, h11.Add(1*time.Minute)))
	fmt.Println("X cross-hour reserve/return OK")

	// T：任务：实充 100 / 赠送 10，提交扣 10（赠送），日志 consume 10；之后失败按记录拆分退 10，日志 refund 10
	tu := newUser(100, 10)
	at(h10.Add(20 * time.Minute))
	tb, tp, err := model.DecreaseUserWallet(tu, 10)
	must(err)
	must(model.SeedConsumeLogForTest(tu, 10, h10.Add(20*time.Minute)))
	at(h11.Add(30 * time.Minute))
	rb, rp, legacy := model.SplitRefundByRecorded(10, tb, tp)
	check(!legacy, "T legacy")
	must(model.RefundUserWalletAfterTheFact(tu, rb, rp, "task_refund", prefix+"task"))
	refundLog(tu, 10, h11.Add(30*time.Minute))
	pp, gg = bal(tu)
	check(pp == 100 && gg == 10, "T balance %d/%d want 100/10", pp, gg)
	fmt.Println("T task-failure refund OK")

	// H2b：实充 100 / 赠送 0，任务扣 10（实充）；获赠 10 被另一请求用掉；任务失败退 10 → 100/0
	h2 := newUser(100, 0)
	at(h10.Add(30 * time.Minute))
	hb2, hp2, err := model.DecreaseUserWallet(h2, 10)
	must(err)
	must(model.SeedConsumeLogForTest(h2, 10, h10.Add(30*time.Minute)))
	must(model.CreditUserWallet(h2, 10, 0, "accept_seed", "b03 gift", "accept", prefix+"gift"))
	f3 := service.NewWalletFundingForAccept(h2)
	must(f3.PreConsume(10))
	must(model.SeedConsumeLogForTest(h2, 10, h10.Add(31*time.Minute)))
	rb, rp, _ = model.SplitRefundByRecorded(10, hb2, hp2)
	must(model.RefundUserWalletAfterTheFact(h2, rb, rp, "task_refund", prefix+"task2"))
	refundLog(h2, 10, h10.Add(40*time.Minute))
	pp, gg = bal(h2)
	check(pp == 100 && gg == 0, "H2b balance %d/%d want 100/0", pp, gg)
	fmt.Println("H2b refund keeps paid OK")
	model.SetWalletConsumeNowForTest(nil)

	ctx := context.Background()
	for _, h := range []time.Time{h10, h11} {
		hh := h
		_, err := model.RunHourlyRollupBackfill(ctx, &hh)
		must(err)
	}
	rows := func(uid int, h time.Time) map[string]int64 {
		var es []model.LedgerEntry
		must(model.DB.Where("user_id = ? AND kind = ? AND hour_bucket = ?", uid, model.LedgerKindAPIConsumeHourly, h).Find(&es).Error)
		m := map[string]int64{}
		for _, e := range es {
			m[e.Account] += e.Delta
		}
		return m
	}
	m := rows(r1, h10)
	check(m["bonus"] == -10 && m["paid"] == -2, "R1 rollup %v want bonus-10 paid-2", m)
	m10, m11 := rows(x, h10), rows(x, h11)
	check(m10["bonus"]+m10["paid"]+m11["bonus"]+m11["paid"] == -12, "X rollup total %v %v", m10, m11)
	m = rows(tu, h10)
	check(m["bonus"] == -10, "T rollup %v want bonus-10", m)
	var refundLedger int64
	must(model.DB.Model(&model.LedgerEntry{}).Where("user_id IN ? AND kind = ?", []int{r1, r2, x}, "wallet_refund").Count(&refundLedger).Error)
	check(refundLedger == 0, "request-level returns wrote %d wallet_refund rows", refundLedger)
	fmt.Println("hourly rollup rows OK")

	for _, uid := range []int{r1, r2, x, tu, h2} {
		d, err := model.UserBalanceDiff(uid)
		must(err)
		check(d == 0, "user %d balance diff %d", uid, d)
	}
	model.SetWalletAccountReconcileFromForTest(day.Format("2006-01-02"))
	defer model.SetWalletAccountReconcileFromForTest("")
	res, err := model.RunDailyReconcile(ctx, day.Format("2006-01-02"), true)
	must(err)
	fmt.Printf("daily reconcile: status=%s balanced=%v mismatch_users=%d account_checked=%d account_drift=%d ids=%v\n",
		res.Status, res.Balanced, res.MismatchUsers, res.AccountCheckedUsers, res.AccountDriftUsers, res.AccountDriftUserIDs)
	check(res.Balanced, "daily reconcile not balanced: %+v", res)
	// 按账户：5 个用户都在切换日之后；只有跨整点用户 X 偏差（L-B，差额兜底记 paid），其余应为 0
	// 库里可能还有别的验收用户：只断言本脚本 5 人都在核对范围内，且其中只有 X 偏差
	scope, err := model.AccountReconcileScope(day)
	must(err)
	inScope := map[int]bool{}
	for _, id := range scope {
		inScope[id] = true
	}
	drift, _, err := model.AccountDriftUsers(day)
	must(err)
	drifted := map[int]bool{}
	for _, id := range drift {
		drifted[id] = true
	}
	check(res.AccountCheckedUsers >= 5, "account checked users %d want >=5", res.AccountCheckedUsers)
	for _, uid := range []int{r1, r2, x, tu, h2} {
		check(inScope[uid], "user %d not in account reconcile scope", uid)
		check(drifted[uid] == (uid == x), "user %d drift=%v want %v (only cross-hour X drifts)", uid, drifted[uid], uid == x)
	}
	fmt.Println("PASS accept_b03_wallet_split")
}
