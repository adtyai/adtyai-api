package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// B13 经营日报：每天 DailyReportTime（上海）推送前一自然日摘要到 Telegram 待处理通道。
// 不含毛利/成本/利润字段。ops_job_runs job=daily_report run_key=报告日期 去重。

const dailyReportKind = "daily_report"

// DailyReport 日报数据（JSON key 即验收口径）。
type DailyReport struct {
	Date               string  `json:"date"`
	Timezone           string  `json:"timezone"`
	TopupPaidUSD       float64 `json:"topup_paid_usd"`
	TopupPaidOrders    int64   `json:"topup_paid_orders"`
	ConsumeQuota       int64   `json:"consume_quota"`
	NewUsers           int64   `json:"new_users"`
	ReconcileStatus    string  `json:"reconcile_status"` // ok/mismatch/incomplete/failed/none
	ReconcileBalanced  *bool   `json:"reconcile_balanced"`
	ChannelAbnormal    int64   `json:"channel_abnormal"`
	PendingWithdrawals int64   `json:"pending_withdrawals"`
	PendingRiskReviews int64   `json:"pending_risk_reviews"`
}

// DailyReportResult system task 返回。
type DailyReportResult struct {
	Date    string       `json:"date"`
	Skipped bool         `json:"skipped"`
	Reason  string       `json:"reason,omitempty"`
	Status  string       `json:"status,omitempty"`
	Report  *DailyReport `json:"report,omitempty"`
	Text    string       `json:"text,omitempty"`
}

// DailyReportDue 当前上海时间是否已过 DailyReportTime。
func DailyReportDue(now time.Time) bool {
	hhmm := GetDailyReportTime()
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		t, _ = time.Parse("15:04", "09:00")
	}
	n := now.In(ShanghaiLocation())
	return n.Hour()*60+n.Minute() >= t.Hour()*60+t.Minute()
}

// BuildDailyReport 只读汇总某上海自然日。
func BuildDailyReport(dateStr string) (*DailyReport, error) {
	day, dateStr, err := ParseBusinessDate(dateStr)
	if err != nil {
		return nil, err
	}
	stats, err := buildOverviewDayStats(day)
	if err != nil {
		return nil, err
	}
	start, end := day, day.Add(24*time.Hour)
	r := &DailyReport{
		Date:            dateStr,
		Timezone:        ReconcileTimezone,
		TopupPaidUSD:    stats.TopupPaidUSD,
		TopupPaidOrders: stats.TopupPaidOrders,
		ConsumeQuota:    stats.ConsumeQuota,
		ReconcileStatus: "none",
	}
	if err := DB.Model(&User{}).Where("created_at >= ? AND created_at < ?", start.Unix(), end.Unix()).
		Count(&r.NewUsers).Error; err != nil {
		return nil, fmt.Errorf("new users: %w", err)
	}
	if row, e := GetOpsJobRun(OpsJobDailyReconcile, dateStr); e == nil && row != nil {
		r.ReconcileStatus = row.Status
		var d map[string]any
		if json.Unmarshal([]byte(row.Detail), &d) == nil {
			if s, ok := d["status"].(string); ok && s != "" {
				r.ReconcileStatus = s
			}
			if b, ok := d["balanced"].(bool); ok {
				r.ReconcileBalanced = &b
			}
		}
	}
	if err := DB.Model(&Channel{}).Where("status = ?", common.ChannelStatusAutoDisabled).
		Count(&r.ChannelAbnormal).Error; err != nil {
		return nil, fmt.Errorf("channels: %w", err)
	}
	if r.PendingWithdrawals, err = countPendingAffWithdrawals(); err != nil {
		return nil, fmt.Errorf("withdrawals: %w", err)
	}
	if err := DB.Model(&RiskReview{}).Where("status = ?", RiskReviewStatusPending).
		Count(&r.PendingRiskReviews).Error; err != nil {
		return nil, fmt.Errorf("risk: %w", err)
	}
	return r, nil
}

var reconcileStatusZh = map[string]string{
	"ok": "已平", "mismatch": "不平", "incomplete": "数据不全", "failed": "失败", "none": "未跑", "running": "进行中",
}

// FormatDailyReportText Telegram 文案（中文）。
func FormatDailyReportText(r *DailyReport) string {
	rs := reconcileStatusZh[r.ReconcileStatus]
	if rs == "" {
		rs = r.ReconcileStatus
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[adtyai] 经营日报 %s（北京时间）\n", r.Date)
	fmt.Fprintf(&b, "充值实付：$%.2f（%d 笔）\n", r.TopupPaidUSD, r.TopupPaidOrders)
	fmt.Fprintf(&b, "调用量（额度）：%d\n", r.ConsumeQuota)
	fmt.Fprintf(&b, "新用户：%d\n", r.NewUsers)
	fmt.Fprintf(&b, "对账：%s\n", rs)
	fmt.Fprintf(&b, "渠道异常：%d\n", r.ChannelAbnormal)
	fmt.Fprintf(&b, "未处理提现：%d\n", r.PendingWithdrawals)
	fmt.Fprintf(&b, "风控待审：%d", r.PendingRiskReviews)
	return b.String()
}

// RunDailyReport 生成并推送 dateStr（空=昨天）日报。
// send=nil 用 OpsTelegramSender（pending 通道）；仍为 nil 时落 ops_alert_events。
func RunDailyReport(ctx context.Context, dateStr string, force bool, send func(text string) error) (*DailyReportResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dateStr == "" {
		dateStr = YesterdayBusinessDate()
	}
	if _, d, err := ParseBusinessDate(dateStr); err != nil {
		return nil, err
	} else {
		dateStr = d
	}
	out := &DailyReportResult{Date: dateStr}
	row, skip, lock, err := beginOpsJobRun(ctx, OpsJobDailyReport, dateStr, force)
	if err != nil {
		return nil, err
	}
	if skip {
		out.Skipped = true
		out.Reason = "already_sent_or_locked"
		return out, nil
	}
	defer lock.Release()

	rep, err := BuildDailyReport(dateStr)
	if err != nil {
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"stage": "build", "error": err.Error()})
		out.Status = OpsJobStatusFailed
		return out, err
	}
	out.Report = rep
	out.Text = FormatDailyReportText(rep)
	if send == nil {
		send = func(text string) error {
			if OpsTelegramSender == nil {
				return RecordOpsAlert(OpsAlertLevelPending, dailyReportKind, map[string]any{"text": text, "sent": false})
			}
			return OpsTelegramSender(OpsAlertLevelPending, dailyReportKind, text)
		}
	}
	if err := send(out.Text); err != nil {
		// 推送失败：failed，下次 tick 重试
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{"stage": "send", "error": err.Error(), "report": rep})
		out.Status = OpsJobStatusFailed
		return out, errors.New("daily report send failed: " + err.Error())
	}
	if err := finishOpsJobRun(row, OpsJobStatusOK, map[string]any{"report": rep}); err != nil {
		return out, err
	}
	out.Status = OpsJobStatusOK
	return out, nil
}
