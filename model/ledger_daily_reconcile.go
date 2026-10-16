package model

import (
	"gorm.io/gorm"

	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	ReconcileTimezone = "Asia/Shanghai"

	ReconcileStatusOK         = "ok"
	ReconcileStatusIncomplete = "incomplete"
	ReconcileStatusMismatch   = "mismatch"
	ReconcileStatusFailed     = "failed"

	BackfillNone    = "none"
	BackfillRan     = "ran"
	BackfillPartial = "partial"
	BackfillOK      = "ok"
)

// ReconcileRefundCheck 当天退款三方核对（bonus_shortfall 单列不算不平）。
type ReconcileRefundCheck struct {
	RegisteredQuota int64 `json:"registered_quota"` // 线下原路退款登记（Amount→quota）
	PaidClawed      int64 `json:"paid_clawed"`      // 实充扣回合计（ledger abs）
	Equal           bool  `json:"equal"`            // registered == paid_clawed
	BonusClawed     int64 `json:"bonus_clawed"`
	AffClawed       int64 `json:"aff_clawed"`
	BonusShortfall  int64 `json:"bonus_shortfall"` // 单列展示，不算对账不平
}

// ReconcileResult GET /api/admin/reconcile 与 daily_reconcile 共用结果。
// hour_bucket 一律 UTC RFC3339；业务日 date 为 Asia/Shanghai 日历日。
type ReconcileResult struct {
	Date            string               `json:"date"`
	Timezone        string               `json:"timezone"`
	Status          string               `json:"status"`
	MissingBuckets  []string             `json:"missing_buckets"`
	BackfillStatus  string               `json:"backfill_status"`
	LastBackup      *string              `json:"last_backup"` // B12：最近一次 backup ok 的 run_key（上海日期）；从未成功为 nil
	Balanced        bool                 `json:"balanced"`
	Diff            int64                `json:"diff,omitempty"`
	LedgerSum       int64                `json:"ledger_sum,omitempty"`
	UnrolledConsume int64                `json:"unrolled_consume,omitempty"` // 本小时未汇总消费合计（正数=已消耗）
	WalletPaidSum   int64                `json:"wallet_paid_sum,omitempty"`
	WalletBonusSum  int64                `json:"wallet_bonus_sum,omitempty"`
	Refund          ReconcileRefundCheck `json:"refund"`
	OrderNos        []string             `json:"order_nos,omitempty"` // 不平才附
	MismatchUsers   int                  `json:"mismatch_users,omitempty"`
	MismatchUserIDs []int                `json:"mismatch_user_ids,omitempty"`
	// B-03 M-B：按账户核对（只报告，不计入 balanced）。仅核对首条流水在切换日期之后的用户。
	AccountCheckFrom    string `json:"account_check_from,omitempty"`
	AccountCheckedUsers int    `json:"account_checked_users,omitempty"`
	AccountDriftUsers   int    `json:"account_drift_users"`
	AccountDriftUserIDs []int  `json:"account_drift_user_ids,omitempty"`
	Skipped             bool   `json:"skipped,omitempty"`
	Note                string `json:"note,omitempty"`
}

// ShanghaiLocation 业务日时区。
func ShanghaiLocation() *time.Location {
	loc, err := time.LoadLocation(ReconcileTimezone)
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// YesterdayBusinessDate 上海时区昨天 YYYY-MM-DD。
func YesterdayBusinessDate() string {
	now := time.Now().In(ShanghaiLocation())
	y := now.AddDate(0, 0, -1)
	return y.Format("2006-01-02")
}

// ErrInvalidBusinessDate 业务日格式非法（调用方用 errors.Is 判别）。
var ErrInvalidBusinessDate = errors.New("invalid date")

// ParseBusinessDate 解析 YYYY-MM-DD（上海日历日）；空则昨天。
func ParseBusinessDate(date string) (time.Time, string, error) {
	loc := ShanghaiLocation()
	date = trimSpaceDate(date)
	if date == "" {
		date = YesterdayBusinessDate()
	}
	t, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%w %q (want YYYY-MM-DD)", ErrInvalidBusinessDate, date)
	}
	return t, date, nil
}

func trimSpaceDate(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// BusinessDayHourBuckets 返回上海日历日对应的 24 个 UTC 整点 hour_bucket。
// 例：2026-10-15 Asia/Shanghai → UTC [2026-10-14T16:00:00Z .. 2026-10-15T15:00:00Z]。
func BusinessDayHourBuckets(dayStartShanghai time.Time) []time.Time {
	loc := ShanghaiLocation()
	start := time.Date(dayStartShanghai.Year(), dayStartShanghai.Month(), dayStartShanghai.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	out := make([]time.Time, 0, 24)
	for h := TruncateToHourUTC(start.UTC()); h.Before(end.UTC()); h = h.Add(time.Hour) {
		out = append(out, h)
	}
	return out
}

// BusinessDayUTCRange 上海日历日的 [start,end) UTC。
func BusinessDayUTCRange(dayStartShanghai time.Time) (time.Time, time.Time) {
	loc := ShanghaiLocation()
	start := time.Date(dayStartShanghai.Year(), dayStartShanghai.Month(), dayStartShanghai.Day(), 0, 0, 0, 0, loc)
	return start.UTC(), start.Add(24 * time.Hour).UTC()
}

// ListMissingHourlyRollupBuckets 返回当日尚未 status=ok 的 hour_bucket run_key。
func ListMissingHourlyRollupBuckets(buckets []time.Time) ([]string, error) {
	missing := make([]string, 0)
	for _, hb := range buckets {
		key := HourBucketRunKey(hb)
		row, err := GetOpsJobRun(OpsJobHourlyRollup, key)
		if err != nil || row == nil || row.Status != OpsJobStatusOK {
			missing = append(missing, key)
			continue
		}
	}
	return missing, nil
}

// backfillMissingHourlyBuckets 对缺失整点逐个 force 补跑。
func backfillMissingHourlyBuckets(ctx context.Context, missingKeys []string) (stillMissing []string, ran bool) {
	stillMissing = make([]string, 0)
	for _, key := range missingKeys {
		hb, err := ParseHourBucketRunKey(key)
		if err != nil {
			stillMissing = append(stillMissing, key)
			continue
		}
		ran = true
		_, err = RunHourlyRollupBackfill(ctx, &hb)
		if err != nil {
			common.SysError(common.LogText("reconcile backfill %s: %s", key, err.Error()))
			stillMissing = append(stillMissing, key)
			continue
		}
		row, err := GetOpsJobRun(OpsJobHourlyRollup, key)
		if err != nil || row == nil || row.Status != OpsJobStatusOK {
			stillMissing = append(stillMissing, key)
		}
	}
	return stillMissing, ran
}

// RunDailyReconcile 执行某业务日对账（B6/A16）。
// force=true 时即使当日 daily_reconcile 已 ok 也重跑（管理端 GET / 验收）。
func RunDailyReconcile(ctx context.Context, dateStr string, force bool) (*ReconcileResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	day, dateStr, err := ParseBusinessDate(dateStr)
	if err != nil {
		return nil, err
	}
	out := &ReconcileResult{
		Date:           dateStr,
		Timezone:       ReconcileTimezone,
		MissingBuckets: []string{},
		BackfillStatus: BackfillNone,
	}
	if key, ok, e := LastOKOpsJobRunKey(OpsJobBackup); e == nil && ok {
		k := key
		out.LastBackup = &k
	} else {
		out.Note = "last_backup=null: no successful backup run yet"
	}

	runKey := dateStr
	row, skip, lock, err := beginOpsJobRun(ctx, OpsJobDailyReconcile, runKey, force)
	if err != nil {
		return nil, err
	}
	if skip {
		out.Skipped = true
		out.Status = ReconcileStatusOK
		// 尽量回填上次 detail 里的关键字段；失败则仅标记 skipped
		if prev, e := GetOpsJobRun(OpsJobDailyReconcile, runKey); e == nil && prev != nil {
			out.Status = prev.Status
			if prev.Status == OpsJobStatusOK {
				out.Balanced = true
				out.Status = ReconcileStatusOK
			}
		}
		return out, nil
	}
	defer lock.Release()

	buckets := BusinessDayHourBuckets(day)
	missing, err := ListMissingHourlyRollupBuckets(buckets)
	if err != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"error": err.Error()})
		out.Status = ReconcileStatusFailed
		return out, err
	}
	out.MissingBuckets = missing

	if len(missing) > 0 {
		touchOpsJobHeartbeat(row)
		still, ran := backfillMissingHourlyBuckets(ctx, missing)
		if ran {
			out.BackfillStatus = BackfillRan
		}
		out.MissingBuckets = still
		if len(still) > 0 {
			out.BackfillStatus = BackfillPartial
			out.Status = ReconcileStatusIncomplete
			out.Balanced = false
			_ = RecordOpsAlert(OpsAlertLevelUrgent, "hourly_rollup_missing", map[string]any{
				"date":            dateStr,
				"missing_buckets": still,
			})
			_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{
				"status":          ReconcileStatusIncomplete,
				"missing_buckets": still,
				"backfill_status": out.BackfillStatus,
				"note":            "buckets incomplete after backfill; no full reconcile",
			})
			return out, nil
		}
		out.BackfillStatus = BackfillOK
	} else {
		out.BackfillStatus = BackfillNone
	}
	out.MissingBuckets = []string{}

	touchOpsJobHeartbeat(row)

	if err := computeBalanceReconcile(out); err != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"error": err.Error()})
		out.Status = ReconcileStatusFailed
		return out, err
	}

	dayStart, dayEnd := BusinessDayUTCRange(day)
	if err := computeRefundReconcile(out, dayStart, dayEnd); err != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"error": err.Error()})
		out.Status = ReconcileStatusFailed
		return out, err
	}

	// 退款三方不等也算不平；bonus_shortfall 不算
	booksOK := out.Balanced && out.Refund.Equal
	if !booksOK {
		out.Status = ReconcileStatusMismatch
		out.Balanced = false
		_ = RecordOpsAlert(OpsAlertLevelUrgent, "reconcile_mismatch", map[string]any{
			"date":            dateStr,
			"diff":            out.Diff,
			"mismatch_users":  out.MismatchUsers,
			"order_nos":       out.OrderNos,
			"refund_equal":    out.Refund.Equal,
			"bonus_shortfall": out.Refund.BonusShortfall,
		})
	} else {
		out.Status = ReconcileStatusOK
		out.Balanced = true
		out.OrderNos = nil
	}

	detail := map[string]any{
		"status":                 out.Status,
		"balanced":               out.Balanced,
		"diff":                   out.Diff,
		"ledger_sum":             out.LedgerSum,
		"unrolled_consume":       out.UnrolledConsume,
		"wallet_paid_sum":        out.WalletPaidSum,
		"wallet_bonus_sum":       out.WalletBonusSum,
		"refund":                 out.Refund,
		"backfill_status":        out.BackfillStatus,
		"mismatch_users":         out.MismatchUsers,
		"order_nos":              out.OrderNos,
		"last_backup":            out.LastBackup,
		"account_check_from":     out.AccountCheckFrom,
		"account_checked_users":  out.AccountCheckedUsers,
		"account_drift_users":    out.AccountDriftUsers,
		"account_drift_user_ids": out.AccountDriftUserIDs,
	}
	if err := finishOpsJobRun(row, OpsJobStatusOK, detail); err != nil {
		return out, err
	}
	return out, nil
}

func computeBalanceReconcile(out *ReconcileResult) error {
	diffs, ledgerSum, unrolledTotal, paidSum, bonusSum, err := collectUserBalanceDiffs()
	if err != nil {
		return err
	}
	out.LedgerSum = ledgerSum
	out.UnrolledConsume = unrolledTotal
	out.WalletPaidSum = paidSum
	out.WalletBonusSum = bonusSum
	out.Diff = (ledgerSum - unrolledTotal) - (paidSum + bonusSum)

	mismatchUIDs := make([]int, 0)
	for uid, d := range diffs {
		if d != 0 {
			mismatchUIDs = append(mismatchUIDs, uid)
		}
	}
	out.MismatchUsers = len(mismatchUIDs)
	out.MismatchUserIDs = mismatchUIDs
	if len(mismatchUIDs) > 50 {
		out.MismatchUserIDs = mismatchUIDs[:50]
	}
	out.Balanced = len(mismatchUIDs) == 0
	if len(mismatchUIDs) > 0 {
		out.OrderNos = listOrderNosForUsers(mismatchUIDs, 50)
	}
	// B-03 M-B：按账户核对只报告，失败只记日志不影响合计对账
	if err := computeAccountReconcile(out); err != nil {
		common.SysError(common.LogText("account reconcile failed: %s", err.Error()))
	}
	return nil
}

// computeAccountReconcile B-03 M-B：对首条流水在 WalletAccountReconcileFrom 之后的用户，
// 核对 SUM(ledger bonus) − 本小时未汇总赠送 = 赠送余额、SUM(ledger paid) − 本小时未汇总实充 = 实充余额。
// 本小时未汇总消费按拆分表同口径（planHourlyRollupSplit）拆。只报告，不计入 balanced。
func computeAccountReconcile(out *ReconcileResult) error {
	from := GetWalletAccountReconcileFrom()
	if from == "" {
		return nil
	}
	day, err := time.ParseInLocation("2006-01-02", from, ShanghaiLocation())
	if err != nil {
		return err
	}
	out.AccountCheckFrom = from
	drift, checked, err := AccountDriftUsers(day)
	if err != nil {
		return err
	}
	out.AccountCheckedUsers = checked
	out.AccountDriftUsers = len(drift)
	out.AccountDriftUserIDs = drift
	if len(drift) > 50 {
		out.AccountDriftUserIDs = drift[:50]
	}
	return nil
}

// AccountReconcileScope 按账户核对的用户范围：首条 paid/bonus 流水时间 ≥ since 的用户。
func AccountReconcileScope(since time.Time) ([]int, error) {
	var err error
	// 首条流水时间 ≥ 切换日期的用户才核对（切换前的历史全记 paid，会误报）
	var entries []struct {
		UserId    int       `gorm:"column:user_id"`
		CreatedAt time.Time `gorm:"column:created_at"`
	}
	if err = DB.Model(&LedgerEntry{}).Select("user_id, MIN(created_at) AS created_at").
		Where("account IN ?", []string{"paid", "bonus"}).Group("user_id").Scan(&entries).Error; err != nil {
		return nil, err
	}
	users := map[int]struct{}{}
	for _, e := range entries {
		if !e.CreatedAt.Before(since) {
			users[e.UserId] = struct{}{}
		}
	}
	if len(users) == 0 {
		return nil, nil
	}
	ids := make([]int, 0, len(users))
	for uid := range users {
		ids = append(ids, uid)
	}
	return ids, nil
}

// AccountDriftUsers 返回按账户对不上的用户（since 之后才有流水的用户），以及核对的用户数。
func AccountDriftUsers(since time.Time) (drift []int, checked int, err error) {
	type row struct {
		UserId  int    `gorm:"column:user_id"`
		Account string `gorm:"column:account"`
		Sum     int64  `gorm:"column:sum"`
	}
	ids, err := AccountReconcileScope(since)
	if err != nil || len(ids) == 0 {
		return nil, 0, err
	}
	var sums []row
	if err = DB.Model(&LedgerEntry{}).Select("user_id, account, COALESCE(SUM(delta),0) AS sum").
		Where("account IN ? AND user_id IN ?", []string{"paid", "bonus"}, ids).
		Group("user_id, account").Scan(&sums).Error; err != nil {
		return
	}
	ledger := map[int][2]int64{}
	for _, s := range sums {
		v := ledger[s.UserId]
		if s.Account == "bonus" {
			v[0] += s.Sum
		} else {
			v[1] += s.Sum
		}
		ledger[s.UserId] = v
	}
	hb := TruncateToHourUTC(time.Now().UTC())
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	var unrolled []struct {
		UserId int   `gorm:"column:user_id"`
		Sum    int64 `gorm:"column:sum"`
	}
	if err = logDB.Table("logs").Select("user_id, COALESCE(SUM(quota),0) AS sum").
		Where("type = ? AND created_at >= ? AND created_at < ? AND quota > 0 AND user_id IN ?",
			LogTypeConsume, hb.Unix(), time.Now().UTC().Unix()+1, ids).
		Group("user_id").Scan(&unrolled).Error; err != nil {
		return
	}
	unrolledBy := map[int]int64{}
	for _, u := range unrolled {
		unrolledBy[u.UserId] = u.Sum
	}
	splits, err := loadWalletConsumeForHour(DB, hb)
	if err != nil {
		return
	}
	for _, uid := range ids {
		checked++
		ub, up, _ := planHourlyRollupSplit(unrolledBy[uid], splits[uid])
		var paid, bonus int64
		if err = DB.Model(&User{}).Where("id = ?", uid).Select("COALESCE(quota,0)").Scan(&paid).Error; err != nil {
			return
		}
		_ = DB.Model(&UserBonusWallet{}).Where("user_id = ?", uid).Select("COALESCE(balance,0)").Scan(&bonus)
		l := ledger[uid]
		if l[0]-ub != bonus || l[1]-up != paid {
			drift = append(drift, uid)
		}
	}
	return drift, checked, nil
}

// UserBalanceDiff 单用户：ledger(paid+bonus) − 本小时未汇总消费 − (实充+赠送)。0 表示平衡。
func UserBalanceDiff(userId int) (diff int64, err error) {
	diffs, _, _, _, _, err := collectUserBalanceDiffs()
	if err != nil {
		return 0, err
	}
	return diffs[userId], nil
}

func collectUserBalanceDiffs() (diffs map[int]int64, ledgerSum, unrolledTotal, paidSum, bonusSum int64, err error) {
	diffs = map[int]int64{}
	hourStart := TruncateToHourUTC(time.Now().UTC())
	// +1s：包含当前秒写入的消费日志（created_at < now 会漏掉同秒 seed）
	hourEndUnix := time.Now().UTC().Unix() + 1
	hourStartUnix := hourStart.Unix()

	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}

	type userAgg struct {
		UserId int   `gorm:"column:user_id"`
		Sum    int64 `gorm:"column:sum"`
	}

	var ledgerRows []userAgg
	if err = DB.Model(&LedgerEntry{}).
		Select("user_id, COALESCE(SUM(delta),0) AS sum").
		Where("account IN ?", []string{"paid", "bonus"}).
		Group("user_id").
		Scan(&ledgerRows).Error; err != nil {
		return
	}
	ledgerByUser := map[int]int64{}
	for _, r := range ledgerRows {
		ledgerByUser[r.UserId] = r.Sum
	}

	var unrolledRows []userAgg
	if err = logDB.Table("logs").
		Select("user_id, COALESCE(SUM(quota),0) AS sum").
		Where("type = ? AND created_at >= ? AND created_at < ? AND quota > 0",
			LogTypeConsume, hourStartUnix, hourEndUnix).
		Group("user_id").
		Scan(&unrolledRows).Error; err != nil {
		return
	}
	unrolledByUser := map[int]int64{}
	for _, r := range unrolledRows {
		unrolledByUser[r.UserId] = r.Sum
		unrolledTotal += r.Sum
	}

	candidates := map[int]struct{}{}
	for uid := range ledgerByUser {
		candidates[uid] = struct{}{}
	}
	for uid := range unrolledByUser {
		candidates[uid] = struct{}{}
	}
	var bonusUsers []struct {
		UserId  int   `gorm:"column:user_id"`
		Balance int64 `gorm:"column:balance"`
	}
	_ = DB.Model(&UserBonusWallet{}).Select("user_id, balance").Where("balance <> 0").Scan(&bonusUsers)
	for _, b := range bonusUsers {
		candidates[b.UserId] = struct{}{}
	}

	for uid := range candidates {
		var paid int64
		if err = DB.Model(&User{}).Where("id = ?", uid).Select("COALESCE(quota,0)").Scan(&paid).Error; err != nil {
			return
		}
		var bonus int64
		_ = DB.Model(&UserBonusWallet{}).Where("user_id = ?", uid).Select("COALESCE(balance,0)").Scan(&bonus)

		ls := ledgerByUser[uid]
		ur := unrolledByUser[uid]
		expected := ls - ur
		actual := paid + bonus
		d := expected - actual
		diffs[uid] = d
		ledgerSum += ls
		paidSum += paid
		bonusSum += bonus
	}
	return
}

func listOrderNosForUsers(userIDs []int, limit int) []string {
	if len(userIDs) == 0 || limit <= 0 {
		return nil
	}
	var nos []string
	_ = DB.Model(&TopupOrder{}).
		Where("user_id IN ? AND status = ?", userIDs, TopupOrderStatusPaid).
		Order("id DESC").
		Limit(limit).
		Pluck("order_no", &nos)
	return nos
}

func computeRefundReconcile(out *ReconcileResult, dayStart, dayEnd time.Time) error {
	type refAgg struct {
		Registered  int64 `gorm:"column:registered"`
		BonusClawed int64 `gorm:"column:bonus_clawed"`
		AffClawed   int64 `gorm:"column:aff_clawed"`
		BonusShort  int64 `gorm:"column:bonus_short"`
	}
	// registered：各退款 Amount 转 quota 后求和（与退款时 DollarsToQuota 一致）
	var refunds []TopupRefund
	if err := DB.Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Find(&refunds).Error; err != nil {
		return err
	}
	var registered, bonusClawed, affClawed, bonusShort int64
	requestIDs := make([]string, 0, len(refunds))
	for _, r := range refunds {
		q, err := DollarsToQuota(r.Amount)
		if err != nil {
			return err
		}
		registered += int64(q)
		bonusClawed += r.BonusClawed
		affClawed += r.AffClawed
		bonusShort += r.BonusShortfall
		requestIDs = append(requestIDs, r.RequestId)
	}

	var paidClawed int64
	if len(requestIDs) > 0 {
		// 实充扣回：ledger account=paid, kind=topup_refund, ref_id=request_id → delta 为负，取绝对值合计
		if err := DB.Model(&LedgerEntry{}).
			Select("COALESCE(SUM(-delta),0)").
			Where("account = ? AND kind = ? AND ref_id IN ? AND delta < 0", "paid", "topup_refund", requestIDs).
			Scan(&paidClawed).Error; err != nil {
			return err
		}
	}

	out.Refund = ReconcileRefundCheck{
		RegisteredQuota: registered,
		PaidClawed:      paidClawed,
		Equal:           registered == paidClawed,
		BonusClawed:     bonusClawed,
		AffClawed:       affClawed,
		BonusShortfall:  bonusShort,
	}
	return nil
}

// SeedOpsHourlyRollupOKForTest 验收：为某 hour_bucket 写入/更新 ops_job_runs=ok（不跑真实汇总）。
func SeedOpsHourlyRollupOKForTest(hb time.Time) error {
	key := HourBucketRunKey(TruncateToHourUTC(hb))
	now := time.Now().UTC()
	var existing OpsJobRun
	err := DB.Where("job = ? AND run_key = ?", OpsJobHourlyRollup, key).First(&existing).Error
	if err == nil {
		return DB.Model(&existing).Updates(map[string]any{
			"status":       OpsJobStatusOK,
			"finished_at":  now,
			"heartbeat_at": now,
			"detail":       `{"seed":true}`,
		}).Error
	}
	row := OpsJobRun{
		Job:         OpsJobHourlyRollup,
		RunKey:      key,
		Status:      OpsJobStatusOK,
		Attempt:     1,
		HeartbeatAt: &now,
		StartedAt:   &now,
		FinishedAt:  &now,
		Detail:      `{"seed":true}`,
	}
	return DB.Create(&row).Error
}

// DeleteOpsJobRunForTest 验收清理。
func DeleteOpsJobRunForTest(job, runKey string) error {
	return DB.Where("job = ? AND run_key = ?", job, runKey).Delete(&OpsJobRun{}).Error
}

// CleanupAcceptReconcileDataForTest 验收专用：清掉 accept_reconcile 历次自造的数据
// （用户名前缀 accept-recon-、订单号前缀 ACCEPT-RECON-），保证用例可重复跑。
// 账本流水只增不改，这里与 DeleteHourlyRollupRowsForTest 一样临时关触发器再删。
// 注意：清理期间账本防篡改触发器处于关闭状态，有短暂窗口；
// 不能和联调写入（真实充值/消费/退款流水）同时跑，仅限验收脚本单独执行。
func CleanupAcceptReconcileDataForTest(usernamePrefix, orderPrefix string) (int, error) {
	if usernamePrefix == "" || orderPrefix == "" {
		return 0, fmt.Errorf("cleanup: empty prefix")
	}
	// 与 CleanupAcceptUsersForTest 同一守卫：没有近期可读备份就拒跑（跳过开关默认关）
	if err := RequireRecentAcceptBackup(); err != nil {
		return 0, err
	}
	logDB := LOG_DB
	separateLogDB := logDB != nil && logDB != DB
	var uids []int
	// 全部删除（含关/开触发器）放同一事务：中途任何一步失败整体回滚，不会留下半清理状态
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Model(&User{}).Where("username LIKE ?", usernamePrefix+"%").Pluck("id", &uids).Error; err != nil {
			return err
		}
		if err := tx.Where("order_no LIKE ?", orderPrefix+"%").Delete(&TopupOrder{}).Error; err != nil {
			return err
		}
		if len(uids) == 0 {
			return nil
		}
		var entryIDs []int64
		if err := tx.Model(&LedgerEntry{}).Where("user_id IN ?", uids).Pluck("id", &entryIDs).Error; err != nil {
			return err
		}
		if len(entryIDs) > 0 {
			if err := DisableLedgerImmutableTrigger(tx); err != nil {
				return err
			}
			if err := tx.Where("user_id IN ?", uids).Delete(&LedgerEntry{}).Error; err != nil {
				return err
			}
			if err := EnableLedgerImmutableTrigger(tx); err != nil {
				return err
			}
			if err := tx.Where("entry_id IN ?", entryIDs).Delete(&LedgerIdempotency{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id IN ?", uids).Delete(&TopupOrder{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", uids).Delete(&UserBonusWallet{}).Error; err != nil {
			return err
		}
		if !separateLogDB {
			if err := tx.Where("user_id IN ?", uids).Delete(&Log{}).Error; err != nil {
				return err
			}
		}
		return tx.Unscoped().Where("id IN ?", uids).Delete(&User{}).Error
	})
	if err != nil {
		return 0, err
	}
	// 独立日志库无法和主库同事务，主事务提交后单独删
	if separateLogDB && len(uids) > 0 {
		if err := logDB.Where("user_id IN ?", uids).Delete(&Log{}).Error; err != nil {
			return len(uids), err
		}
	}
	return len(uids), nil
}

// AllMismatchUserIDsForTest 验收专用：返回全部不平用户（不截 50），口径与 computeBalanceReconcile 相同。
func AllMismatchUserIDsForTest() ([]int, error) {
	diffs, _, _, _, _, err := collectUserBalanceDiffs()
	if err != nil {
		return nil, err
	}
	out := make([]int, 0)
	for uid, d := range diffs {
		if d != 0 {
			out = append(out, uid)
		}
	}
	return out, nil
}

// OrderNosForUsersForTest 验收专用：按用户精确取订单号，口径与对账结果 order_nos 相同。
func OrderNosForUsersForTest(userIDs []int, limit int) []string {
	return listOrderNosForUsers(userIDs, limit)
}
