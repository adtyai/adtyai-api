package service

import (
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 分站经营数据（站长中心 11.6、主站分站管理 11.9）。
// 消费额、平台收入、站长收益都从收益日汇总（11.5）算，所以「客户付 = 平台收 + 站长赚」在报表里同样成立；
// 充值额从订单表（adty_topup_orders.site_id）算。金额一律美元、decimal，返回字符串保留 2 位。
// 日汇总的 Day 可能带槽位后缀（"2026-10-31+1"），按日期汇总时去掉。

// AdtySiteDayStats 是一天的经营数据。
type AdtySiteDayStats struct {
	Day            string          `json:"day"`
	NewCustomers   int64           `json:"new_customers"`
	PaidCustomers  int64           `json:"paid_customers"`
	ActiveCustomer int64           `json:"active_customers"`
	TopUpUSD       decimal.Decimal `json:"topup_usd"`
	RevenueUSD     decimal.Decimal `json:"revenue_usd"`  // 客户消费额（扣掉任务退款）
	PlatformUSD    decimal.Decimal `json:"platform_usd"` // 平台收入
	EarningUSD     decimal.Decimal `json:"earning_usd"`  // 站长收益（含加赠、试用的负数，不含 skipped）
	BonusUSD       decimal.Decimal `json:"bonus_usd"`    // 站长出的加赠（负数是出了钱，退款扣回为正）
	RefundUSD      decimal.Decimal `json:"refund_usd"`   // 任务退款冲掉的消费额
	SkippedUSD     decimal.Decimal `json:"skipped_usd"`  // 风控不计的收益
}

// AdtySiteStats 是一段时间的汇总和按天的序列。
type AdtySiteStats struct {
	From   string             `json:"from"`
	To     string             `json:"to"`
	Total  AdtySiteDayStats   `json:"total"`
	Series []AdtySiteDayStats `json:"series"`
}

// adtyStatsRange 规范化日期范围：默认最近 30 天，最长 366 天。
func adtyStatsRange(from, to string) (string, string, error) {
	now := time.Now()
	if to == "" {
		to = now.Format("2006-01-02")
	}
	if from == "" {
		from = now.AddDate(0, 0, -29).Format("2006-01-02")
	}
	start, err1 := time.ParseInLocation("2006-01-02", from, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02", to, time.Local)
	if err1 != nil || err2 != nil || end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return "", "", common.NewMessage("Invalid date range")
	}
	return from, to, nil
}

// adtyCreditedTopUpTrades 是已经入过账的订单号：成功的，以及退过款的。
// 退款会把状态改成 refunded，经营数据不能因此把整笔充值额丢掉。
func adtyCreditedTopUpTrades() *gorm.DB {
	return model.DB.Model(&model.TopUp{}).Select("trade_no").Where("status IN ?", []string{common.TopUpStatusSuccess, model.TopUpStatusRefunded})
}

// adtyRetainedTopUpUSD 是这笔订单还留在充值额里的面额。
// 没退本金时是整笔面额；退过一部分时按还没退的本金比例留下，应退本金为 0 时整笔仍在。
func adtyRetainedTopUpUSD(order *model.AdtyTopupOrder) decimal.Decimal {
	if order == nil || !order.FaceUSD.IsPositive() {
		return decimal.Zero
	}
	kept := decimal.NewFromInt(1).Sub(order.RefundRatio())
	if !kept.IsPositive() {
		return decimal.Zero
	}
	return order.FaceUSD.Mul(kept)
}

func adtyDayUnix(day string, endOfDay bool) int64 {
	parsed, _ := time.ParseInLocation("2006-01-02", day, time.Local)
	if endOfDay {
		return parsed.AddDate(0, 0, 1).Unix() - 1
	}
	return parsed.Unix()
}

// adtySiteEarningSums 按日期汇总收益行：siteIds 为空表示全部分站。
func adtySiteEarningRows(siteIds []int, from, to string) ([]model.AdtySiteEarningDaily, error) {
	query := model.DB.Where("day >= ? AND day <= ? AND status <> ?", from, to+"+~", model.AdtySiteEarningVoid)
	if len(siteIds) > 0 {
		query = query.Where("site_id IN ?", siteIds)
	}
	var rows []model.AdtySiteEarningDaily
	err := query.Find(&rows).Error
	return rows, err
}

// adtyApplyEarningRow 把一行收益加进一天的数据。
func adtyApplyEarningRow(stats *AdtySiteDayStats, row model.AdtySiteEarningDaily) {
	switch row.Kind {
	case model.AdtySiteEarningUsage, model.AdtySiteEarningRefund:
		stats.RevenueUSD = stats.RevenueUSD.Add(row.Revenue)
		stats.PlatformUSD = stats.PlatformUSD.Add(row.Platform)
		stats.EarningUSD = stats.EarningUSD.Add(row.Earning)
		if row.Kind == model.AdtySiteEarningRefund {
			stats.RefundUSD = stats.RefundUSD.Add(row.Revenue.Neg())
		}
	case model.AdtySiteEarningBonus, model.AdtySiteEarningBonusReversal:
		stats.BonusUSD = stats.BonusUSD.Add(row.Earning)
		stats.EarningUSD = stats.EarningUSD.Add(row.Earning)
	case model.AdtySiteEarningTrial:
		stats.EarningUSD = stats.EarningUSD.Add(row.Earning)
	case model.AdtySiteEarningSkipped:
		// 风控不计收益：客户照常付钱，平台照常收，只是站长这份不结算。
		stats.RevenueUSD = stats.RevenueUSD.Add(row.Revenue)
		stats.PlatformUSD = stats.PlatformUSD.Add(row.Platform)
		stats.SkippedUSD = stats.SkippedUSD.Add(row.Earning)
	}
}

func adtyAddDayStats(total *AdtySiteDayStats, day AdtySiteDayStats) {
	total.NewCustomers += day.NewCustomers
	total.TopUpUSD = total.TopUpUSD.Add(day.TopUpUSD)
	total.RevenueUSD = total.RevenueUSD.Add(day.RevenueUSD)
	total.PlatformUSD = total.PlatformUSD.Add(day.PlatformUSD)
	total.EarningUSD = total.EarningUSD.Add(day.EarningUSD)
	total.BonusUSD = total.BonusUSD.Add(day.BonusUSD)
	total.RefundUSD = total.RefundUSD.Add(day.RefundUSD)
	total.SkippedUSD = total.SkippedUSD.Add(day.SkippedUSD)
}

// GetAdtySiteStats 是一个站在 [from, to] 的经营数据（站长中心和后台明细共用）。
func GetAdtySiteStats(siteId int, from, to string) (*AdtySiteStats, error) {
	from, to, err := adtyStatsRange(from, to)
	if err != nil {
		return nil, err
	}
	days := map[string]*AdtySiteDayStats{}
	dayOf := func(day string) *AdtySiteDayStats {
		if days[day] == nil {
			days[day] = &AdtySiteDayStats{Day: day}
		}
		return days[day]
	}
	active := map[string]map[int]bool{}
	rows, err := adtySiteEarningRows([]int{siteId}, from, to)
	if err != nil {
		return nil, err
	}
	allActive := map[int]bool{}
	for _, row := range rows {
		day := adtyEarningDayOf(row.Day)
		adtyApplyEarningRow(dayOf(day), row)
		if row.Kind == model.AdtySiteEarningUsage || row.Kind == model.AdtySiteEarningSkipped {
			if active[day] == nil {
				active[day] = map[int]bool{}
			}
			active[day][row.UserId] = true
			allActive[row.UserId] = true
		}
	}
	for day, users := range active {
		dayOf(day).ActiveCustomer = int64(len(users))
	}

	startUnix, endUnix := adtyDayUnix(from, false), adtyDayUnix(to, true)
	var joined []model.AdtySiteCustomer
	if err := model.DB.Select("user_id", "joined_at").Where("site_id = ? AND joined_at >= ? AND joined_at <= ?", siteId, startUnix, endUnix).Find(&joined).Error; err != nil {
		return nil, err
	}
	for _, customer := range joined {
		dayOf(time.Unix(customer.JoinedAt, 0).Format("2006-01-02")).NewCustomers++
	}

	var orders []model.AdtyTopupOrder
	credited := adtyCreditedTopUpTrades()
	if err := model.DB.Select("user_id", "face_usd", "credit_quota", "refund_quota", "created_at").
		Where("site_id = ? AND created_at >= ? AND created_at <= ? AND trade_no IN (?)", siteId, startUnix, endUnix, credited).
		Find(&orders).Error; err != nil {
		return nil, err
	}
	payers := map[string]map[int]bool{}
	allPayers := map[int]bool{}
	for _, order := range orders {
		kept := adtyRetainedTopUpUSD(&order)
		if !kept.IsPositive() {
			continue
		}
		day := time.Unix(order.CreatedAt, 0).Format("2006-01-02")
		stats := dayOf(day)
		stats.TopUpUSD = stats.TopUpUSD.Add(kept)
		if payers[day] == nil {
			payers[day] = map[int]bool{}
		}
		payers[day][order.UserId] = true
		allPayers[order.UserId] = true
	}
	for day, users := range payers {
		dayOf(day).PaidCustomers = int64(len(users))
	}

	result := &AdtySiteStats{From: from, To: to, Series: make([]AdtySiteDayStats, 0, len(days)), Total: AdtySiteDayStats{Day: ""}}
	for _, stats := range days {
		result.Series = append(result.Series, *stats)
		adtyAddDayStats(&result.Total, *stats)
	}
	sort.Slice(result.Series, func(i, j int) bool { return result.Series[i].Day < result.Series[j].Day })
	result.Total.ActiveCustomer = int64(len(allActive))
	result.Total.PaidCustomers = int64(len(allPayers))
	return result, nil
}

// AdtySiteMonthTotals 是后台列表上每个站本月的数字。
type AdtySiteMonthTotals struct {
	TopUpUSD    decimal.Decimal `json:"month_topup_usd"`
	RevenueUSD  decimal.Decimal `json:"month_revenue_usd"`
	EarningUSD  decimal.Decimal `json:"month_earning_usd"`
	PlatformUSD decimal.Decimal `json:"month_platform_usd"`
	LastRevenue decimal.Decimal `json:"last_month_revenue_usd"`
	GrowthPct   *float64        `json:"growth_pct"` // 本月消费额比上月的增长（%），上月为 0 时为空
}

// AdtySitesMonthTotals 算这些站本月（和上月消费额，用来算增长）的数字。
func AdtySitesMonthTotals(siteIds []int, now time.Time) (map[int]AdtySiteMonthTotals, error) {
	result := map[int]AdtySiteMonthTotals{}
	if len(siteIds) == 0 {
		return result, nil
	}
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	lastStart := monthStart.AddDate(0, -1, 0)
	rows, err := adtySiteEarningRows(siteIds, lastStart.Format("2006-01-02"), now.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	thisMonth := monthStart.Format("2006-01-02")
	for _, row := range rows {
		totals := result[row.SiteId]
		var day AdtySiteDayStats
		adtyApplyEarningRow(&day, row)
		if adtyEarningDayOf(row.Day) >= thisMonth {
			totals.RevenueUSD = totals.RevenueUSD.Add(day.RevenueUSD)
			totals.EarningUSD = totals.EarningUSD.Add(day.EarningUSD)
			totals.PlatformUSD = totals.PlatformUSD.Add(day.PlatformUSD)
		} else {
			totals.LastRevenue = totals.LastRevenue.Add(day.RevenueUSD)
		}
		result[row.SiteId] = totals
	}
	var orders []model.AdtyTopupOrder
	credited := adtyCreditedTopUpTrades()
	if err := model.DB.Select("site_id", "face_usd", "credit_quota", "refund_quota").Where("site_id IN ? AND created_at >= ? AND trade_no IN (?)", siteIds, monthStart.Unix(), credited).
		Find(&orders).Error; err != nil {
		return nil, err
	}
	for _, order := range orders {
		kept := adtyRetainedTopUpUSD(&order)
		if !kept.IsPositive() {
			continue
		}
		totals := result[order.SiteId]
		totals.TopUpUSD = totals.TopUpUSD.Add(kept)
		result[order.SiteId] = totals
	}
	for id, totals := range result {
		if totals.LastRevenue.IsPositive() {
			growth := totals.RevenueUSD.Sub(totals.LastRevenue).Div(totals.LastRevenue).Mul(decimal.NewFromInt(100)).Round(2).InexactFloat64()
			totals.GrowthPct = &growth
			result[id] = totals
		}
	}
	return result, nil
}

// AdtySitesOverview 是全部分站的汇总，和主站直客对比（后台 overview）。
type AdtySitesOverview struct {
	From              string          `json:"from"`
	To                string          `json:"to"`
	Sites             map[string]int  `json:"sites"` // 状态 → 站点数
	SiteCustomers     int64           `json:"site_customers"`
	MainCustomers     int64           `json:"main_customers"`
	SiteTopUpUSD      decimal.Decimal `json:"site_topup_usd"`
	MainTopUpUSD      decimal.Decimal `json:"main_topup_usd"`
	SiteRevenueUSD    decimal.Decimal `json:"site_revenue_usd"`
	SitePlatformUSD   decimal.Decimal `json:"site_platform_usd"`
	SiteEarningUSD    decimal.Decimal `json:"site_earning_usd"`
	SiteSkippedUSD    decimal.Decimal `json:"site_skipped_usd"`
	SiteBonusUSD      decimal.Decimal `json:"site_bonus_usd"`
	PendingReports    int64           `json:"pending_reports"`
	FlaggedCustomers  int64           `json:"flagged_customers"`
	PendingApplicants int64           `json:"pending_applications"`
	// 自定义主机名占 Cloudflare for SaaS 名额。含 100 个，超出的每个每月 0.10 美元（方案 11.8、11.9）。
	CustomHostnames       int64                `json:"custom_hostnames"`
	HostnameMonthlyFeeUSD decimal.Decimal      `json:"hostname_monthly_fee_usd"`
	EarningHookFailures   int64                `json:"earning_hook_failures"`
	SettlementHolds       []AdtySettlementHold `json:"settlement_holds"`
}

// AdtySettlementHold 是对账对不上、正收益暂停结算的站。
type AdtySettlementHold struct {
	SiteId           int    `json:"site_id"`
	Subdomain        string `json:"subdomain"`
	SettleHoldReason string `json:"settle_hold_reason"`
}

const (
	adtyIncludedCustomHostnames = 100
	adtyExtraHostnameMonthlyUSD = "0.10"
)

func adtyHostnameMonthlyFee(count int64) decimal.Decimal {
	extra := count - adtyIncludedCustomHostnames
	if extra < 0 {
		extra = 0
	}
	return decimal.NewFromInt(extra).Mul(decimal.RequireFromString(adtyExtraHostnameMonthlyUSD))
}

// GetAdtySitesOverview 汇总 [from, to] 内所有分站，各项是日汇总的直接合计。
func GetAdtySitesOverview(from, to string) (*AdtySitesOverview, error) {
	from, to, err := adtyStatsRange(from, to)
	if err != nil {
		return nil, err
	}
	result := &AdtySitesOverview{From: from, To: to, Sites: map[string]int{}}
	var statuses []struct {
		Status string
		Total  int
	}
	if err := model.DB.Model(&model.AdtySite{}).Select("status, COUNT(*) AS total").Group("status").Scan(&statuses).Error; err != nil {
		return nil, err
	}
	for _, row := range statuses {
		result.Sites[row.Status] = row.Total
		if row.Status == model.AdtySiteStatusPending {
			result.PendingApplicants = int64(row.Total)
		}
	}
	if err := model.DB.Model(&model.User{}).Where("site_id > 0").Count(&result.SiteCustomers).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Model(&model.User{}).Where("site_id = 0").Count(&result.MainCustomers).Error; err != nil {
		return nil, err
	}
	rows, err := adtySiteEarningRows(nil, from, to)
	if err != nil {
		return nil, err
	}
	var total AdtySiteDayStats
	for _, row := range rows {
		adtyApplyEarningRow(&total, row)
	}
	result.SiteRevenueUSD, result.SitePlatformUSD, result.SiteEarningUSD = total.RevenueUSD, total.PlatformUSD, total.EarningUSD
	result.SiteSkippedUSD, result.SiteBonusUSD = total.SkippedUSD, total.BonusUSD

	startUnix, endUnix := adtyDayUnix(from, false), adtyDayUnix(to, true)
	var orders []model.AdtyTopupOrder
	if err := model.DB.Select("site_id", "face_usd", "credit_quota", "refund_quota").
		Where("created_at >= ? AND created_at <= ? AND trade_no IN (?)", startUnix, endUnix, adtyCreditedTopUpTrades()).
		Find(&orders).Error; err != nil {
		return nil, err
	}
	for _, order := range orders {
		kept := adtyRetainedTopUpUSD(&order)
		if order.SiteId > 0 {
			result.SiteTopUpUSD = result.SiteTopUpUSD.Add(kept)
		} else {
			result.MainTopUpUSD = result.MainTopUpUSD.Add(kept)
		}
	}
	if err := model.DB.Model(&model.AdtySiteReport{}).Where("status = ?", model.AdtySiteReportPending).Count(&result.PendingReports).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Model(&model.AdtySiteCustomer{}).Where("risk_flag <> '' AND reviewed = ?", false).Count(&result.FlaggedCustomers).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Model(&model.AdtySiteDomain{}).
		Where("status IN ?", []string{model.AdtySiteDomainPending, model.AdtySiteDomainActive}).
		Count(&result.CustomHostnames).Error; err != nil {
		return nil, err
	}
	result.HostnameMonthlyFeeUSD = adtyHostnameMonthlyFee(result.CustomHostnames)
	var hookSum struct {
		Total int64
	}
	if err := model.DB.Model(&model.AdtySite{}).Select("COALESCE(SUM(earning_hook_failures),0) AS total").Scan(&hookSum).Error; err != nil {
		return nil, err
	}
	result.EarningHookFailures = hookSum.Total
	var held []model.AdtySite
	if err := model.DB.Select("id", "subdomain", "settle_hold_reason").Where("settle_hold_reason <> ''").Order("id").Find(&held).Error; err != nil {
		return nil, err
	}
	result.SettlementHolds = make([]AdtySettlementHold, 0, len(held))
	for _, site := range held {
		result.SettlementHolds = append(result.SettlementHolds, AdtySettlementHold{
			SiteId: site.Id, Subdomain: site.Subdomain, SettleHoldReason: site.SettleHoldReason,
		})
	}
	return result, nil
}

// AdtySiteBalances 是站长中心收益页顶部的数字：待结算、可提现、已提现、风控不计。
type AdtySiteBalances struct {
	PendingUSD   decimal.Decimal `json:"pending_usd"`
	SkippedUSD   decimal.Decimal `json:"skipped_usd"`
	AvailableUSD decimal.Decimal `json:"available_usd"` // 佣金账户的可提现（和 P6 同一个账户，可以为负）
	FrozenUSD    decimal.Decimal `json:"frozen_usd"`
	WithdrawnUSD decimal.Decimal `json:"withdrawn_usd"`
}

// GetAdtySiteBalances 取站长的收益余额。
func GetAdtySiteBalances(site *model.AdtySite) (*AdtySiteBalances, error) {
	result := &AdtySiteBalances{}
	var sums []struct {
		Kind  string
		Total decimal.Decimal
	}
	legacyOwnerID, err := model.ResolveAdtySiteFinancialOwnerTx(model.DB, site.Id, 0)
	if err != nil {
		return nil, err
	}
	ownerIDs := []int{site.OwnerUserId}
	if legacyOwnerID == site.OwnerUserId {
		ownerIDs = append(ownerIDs, 0)
	}
	if err := model.DB.Model(&model.AdtySiteEarningDaily{}).Select("kind, COALESCE(SUM(earning),0) AS total").
		Where("site_id = ? AND site_owner_user_id IN ? AND status = ?", site.Id, ownerIDs, model.AdtySiteEarningPending).Group("kind").Scan(&sums).Error; err != nil {
		return nil, err
	}
	for _, sum := range sums {
		if sum.Kind == model.AdtySiteEarningSkipped {
			result.SkippedUSD = result.SkippedUSD.Add(sum.Total)
		} else {
			result.PendingUSD = result.PendingUSD.Add(sum.Total)
		}
	}
	var accounts []model.ReferralCommissionAccount
	if err := model.DB.Where("user_id = ?", site.OwnerUserId).Limit(1).Find(&accounts).Error; err != nil {
		return nil, err
	}
	if len(accounts) > 0 {
		result.AvailableUSD = moneyDec(accounts[0].AvailableAmount)
		result.FrozenUSD = moneyDec(accounts[0].FrozenAmount)
		result.WithdrawnUSD = moneyDec(accounts[0].WithdrawnAmount)
	}
	return result, nil
}

// AdtySiteUsageRow 是按模型或分组的一行消费（客户扣费额度和美元）。
type AdtySiteUsageRow struct {
	Key      string          `json:"key"`
	Quota    int64           `json:"quota"`
	USD      decimal.Decimal `json:"usd"`
	Requests int64           `json:"requests"`
}

// AdtySiteUsageBreakdown 是后台明细里按模型、按分组的消费。
type AdtySiteUsageBreakdown struct {
	ByModel []AdtySiteUsageRow `json:"by_model"`
	ByGroup []AdtySiteUsageRow `json:"by_group"`
}

// GetAdtySiteUsageBreakdown 从消费日志按模型、分组汇总这个站现有客户在 [from, to] 的消费（后台用，站长看不到渠道）。
// 关掉消费日志时这里为空；收益和总额以日汇总为准。客户转走后不再算进原来的站。
func GetAdtySiteUsageBreakdown(siteId int, from, to string) (*AdtySiteUsageBreakdown, error) {
	var userIds []int
	if err := model.DB.Model(&model.User{}).Where("site_id = ?", siteId).Pluck("id", &userIds).Error; err != nil {
		return nil, err
	}
	result := &AdtySiteUsageBreakdown{ByModel: []AdtySiteUsageRow{}, ByGroup: []AdtySiteUsageRow{}}
	if len(userIds) == 0 {
		return result, nil
	}
	perUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	for _, column := range []string{"model_name", "group"} {
		quoted := column
		if column == "group" {
			quoted = model.AdtyGroupColumn()
		}
		var rows []struct {
			Key      string
			Quota    int64
			Requests int64
		}
		if err := model.LOG_DB.Model(&model.Log{}).Select(quoted+" AS key, COALESCE(SUM(quota),0) AS quota, COUNT(*) AS requests").
			Where("type = ? AND user_id IN ? AND created_at >= ? AND created_at <= ?", model.LogTypeConsume, userIds, adtyDayUnix(from, false), adtyDayUnix(to, true)).
			Group(quoted).Order("quota DESC").Limit(100).Scan(&rows).Error; err != nil {
			return nil, err
		}
		items := make([]AdtySiteUsageRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, AdtySiteUsageRow{Key: row.Key, Quota: row.Quota, Requests: row.Requests, USD: decimal.NewFromInt(row.Quota).Div(perUnit).Round(2)})
		}
		if column == "model_name" {
			result.ByModel = items
		} else {
			result.ByGroup = items
		}
	}
	return result, nil
}

// AdtyMonthlySeries 把按天的数据合成按月（2026-10）。付费和活跃客户数是按天去重的，不能相加，按月序列里不给（为 0）；
// 整段时间的去重人数看 AdtySiteStats.Total。
func AdtyMonthlySeries(series []AdtySiteDayStats) []AdtySiteDayStats {
	months := map[string]*AdtySiteDayStats{}
	keys := []string{}
	for _, day := range series {
		month := day.Day
		if len(month) >= 7 {
			month = month[:7]
		}
		if months[month] == nil {
			months[month] = &AdtySiteDayStats{Day: month}
			keys = append(keys, month)
		}
		adtyAddDayStats(months[month], day)
	}
	sort.Strings(keys)
	result := make([]AdtySiteDayStats, 0, len(keys))
	for _, key := range keys {
		result = append(result, *months[key])
	}
	return result
}
