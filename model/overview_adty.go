package model

import (
	"fmt"
	"time"
)

// OverviewDayStats 单日经营摘要（Asia/Shanghai 自然日）。
// 无毛利/成本字段。日报等内部汇总复用；运营总览 HTTP 已下线。
type OverviewDayStats struct {
	Date              string  `json:"date"`
	TopupPaidUSD      float64 `json:"topup_paid_usd"`      // topup_orders.status=paid 的 paid_amount_usd 合计
	TopupPaidOrders   int64   `json:"topup_paid_orders"`   // 同上订单数
	ConsumeQuota      int64   `json:"consume_quota"`       // 小时汇总扣费 |delta| 合计（paid+bonus）
	ConsumeQuotaPaid  int64   `json:"consume_quota_paid"`  // account=paid
	ConsumeQuotaBonus int64   `json:"consume_quota_bonus"` // account=bonus
}

// pendingAffWithdrawStatuses 待处理/未完结提现口径：
// - pending：待审
// - approved：已审待打款
// - pay_failed：打款失败可重试
// - pay_unknown：打款结果不明需人工
// 不含 rejected / paid / pay_failed_final（已终结）。
var pendingAffWithdrawStatuses = []string{
	AffWithdrawStatusPending,
	AffWithdrawStatusApproved,
	AffWithdrawStatusPayFailed,
	AffWithdrawStatusPayUnknown,
}

func buildOverviewDayStats(dayStartShanghai time.Time) (*OverviewDayStats, error) {
	loc := ShanghaiLocation()
	start := time.Date(dayStartShanghai.Year(), dayStartShanghai.Month(), dayStartShanghai.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	dateStr := start.Format("2006-01-02")

	paidUSD, paidOrders, err := sumPaidTopupsInRange(start.Unix(), end.Unix())
	if err != nil {
		return nil, fmt.Errorf("topup day %s: %w", dateStr, err)
	}
	consumePaid, consumeBonus, err := sumHourlyConsumeInBusinessDay(start)
	if err != nil {
		return nil, fmt.Errorf("consume day %s: %w", dateStr, err)
	}

	return &OverviewDayStats{
		Date:              dateStr,
		TopupPaidUSD:      paidUSD,
		TopupPaidOrders:   paidOrders,
		ConsumeQuota:      consumePaid + consumeBonus,
		ConsumeQuotaPaid:  consumePaid,
		ConsumeQuotaBonus: consumeBonus,
	}, nil
}

// sumPaidTopupsInRange 已入账充值：status=paid 且 complete_time ∈ [startUnix, endUnix)。
func sumPaidTopupsInRange(startUnix, endUnix int64) (paidUSD float64, orders int64, err error) {
	type row struct {
		PaidUSD float64
		Orders  int64
	}
	var r row
	err = DB.Model(&TopupOrder{}).
		Select("COALESCE(SUM(paid_amount_usd),0) AS paid_usd, COUNT(*) AS orders").
		Where("status = ? AND complete_time >= ? AND complete_time < ?", TopupOrderStatusPaid, startUnix, endUnix).
		Scan(&r).Error
	if err != nil {
		return 0, 0, err
	}
	return r.PaidUSD, r.Orders, nil
}

// sumHourlyConsumeInBusinessDay 上海自然日对应 24 个 UTC hour_bucket 上
// kind=api_consume_hourly 的扣费额度（取 -SUM(delta)，delta 为负）。
func sumHourlyConsumeInBusinessDay(dayStartShanghai time.Time) (paid, bonus int64, err error) {
	buckets := BusinessDayHourBuckets(dayStartShanghai)
	if len(buckets) == 0 {
		return 0, 0, nil
	}
	startHB := buckets[0]
	endHB := buckets[len(buckets)-1].Add(time.Hour) // exclusive upper on hour_bucket

	type row struct {
		Account string
		Total   int64
	}
	var rows []row
	err = DB.Model(&LedgerEntry{}).
		Select("account, COALESCE(SUM(-delta),0) AS total").
		Where("kind = ? AND hour_bucket IS NOT NULL AND hour_bucket >= ? AND hour_bucket < ?",
			LedgerKindAPIConsumeHourly, startHB, endHB).
		Group("account").
		Scan(&rows).Error
	if err != nil {
		return 0, 0, err
	}
	for _, r := range rows {
		switch r.Account {
		case "paid":
			paid = r.Total
		case "bonus":
			bonus = r.Total
		}
	}
	return paid, bonus, nil
}

func countPendingAffWithdrawals() (int64, error) {
	var n int64
	err := DB.Model(&AffWithdrawal{}).
		Where("status IN ?", pendingAffWithdrawStatuses).
		Count(&n).Error
	return n, err
}
