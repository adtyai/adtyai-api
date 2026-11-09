package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 充值对账（任务 3.5）：从旧项目改写，只看 top_ups LEFT JOIN adty_topup_orders，金额一律美元，
// 去掉了订阅订单的 UNION、人民币折算和 Creem/Stripe 的特判。

type rechargeAuditOrder struct {
	Id              int                  `json:"id"`
	UserId          int                  `json:"user_id"`
	Username        string               `json:"username"`
	SiteId          *int                 `json:"site_id"`
	SiteName        string               `json:"site_name" gorm:"-"`
	OrderType       string               `json:"order_type" gorm:"-"`
	TradeNo         string               `json:"trade_no"`
	PaymentMethod   string               `json:"payment_method"`
	PaymentProvider string               `json:"payment_provider"`
	Status          string               `json:"status"`
	CreateTime      int64                `json:"create_time"`
	CompleteTime    int64                `json:"complete_time"`
	Amount          int64                `json:"amount"`
	Money           float64              `json:"money"`
	HasExt          bool                 `json:"has_ext"`
	FaceUSD         decimal.Decimal      `json:"face_usd"`
	BonusUSD        decimal.Decimal      `json:"bonus_usd"`
	CreditQuota     int64                `json:"credit_quota"`
	PayAmount       decimal.Decimal      `json:"pay_amount"`
	PayCurrency     string               `json:"pay_currency"`
	PaidAmount      decimal.Decimal      `json:"paid_amount"`
	PaidCurrency    string               `json:"paid_currency"`
	PaidTxHash      string               `json:"paid_tx_hash"`
	PaidSource      string               `json:"paid_source"`
	ChannelId       int                  `json:"channel_id"`
	ChannelName     string               `json:"channel_name"`
	Anomaly         string               `json:"anomaly"`
	Actions         rechargeAuditActions `json:"actions" gorm:"-"`
}

type rechargeAuditActions struct {
	Complete bool `json:"complete"`
	Refund   bool `json:"refund"`
	Query    bool `json:"query"`
}

var rechargeAuditStatuses = []string{
	common.TopUpStatusPending, common.TopUpStatusSuccess, common.TopUpStatusExpired,
	common.TopUpStatusFailed, model.TopUpStatusCanceled, model.TopUpStatusRefunded,
}

// rechargeAuditQuery 建对账查询，筛选条件都来自查询参数。
func rechargeAuditQuery(c *gin.Context, ownerSiteID *int) (*gorm.DB, bool) {
	query := model.DB.Table("top_ups AS t").
		Joins("LEFT JOIN adty_topup_orders a ON a.trade_no = t.trade_no").
		Joins("LEFT JOIN users u ON u.id = t.user_id")
	if ownerSiteID != nil {
		query = query.Where("a.site_id = ?", *ownerSiteID)
	}
	if raw := strings.TrimSpace(c.Query("site_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id < 0 {
			common.ApiErrorT(c, "Invalid parameters")
			return nil, false
		}
		query = query.Where("a.site_id = ?", id)
	}
	if kind := strings.TrimSpace(c.Query("order_type")); kind != "" && kind != "topup" {
		common.ApiErrorT(c, "Invalid parameters")
		return nil, false
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("(t.trade_no LIKE ? OR u.username LIKE ? OR a.paid_tx_hash LIKE ? OR a.gateway_trade_id LIKE ?)", like, like, like, like)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		valid := false
		for _, allowed := range rechargeAuditStatuses {
			if allowed == status {
				valid = true
			}
		}
		if !valid {
			common.ApiErrorT(c, "Invalid parameters")
			return nil, false
		}
		query = query.Where("t.status = ?", status)
	}
	if provider := strings.TrimSpace(c.Query("provider")); provider != "" {
		query = query.Where("t.payment_provider = ?", provider)
	}
	if method := strings.TrimSpace(c.Query("method")); method != "" {
		query = query.Where("t.payment_method = ?", method)
	}
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return nil, false
		}
		query = query.Where("a.channel_id = ?", id)
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return nil, false
		}
		query = query.Where("t.user_id = ?", id)
	}
	if start, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64); err == nil && start > 0 {
		query = query.Where("t.create_time >= ?", start)
	}
	if end, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64); err == nil && end > 0 {
		query = query.Where("t.create_time <= ?", end)
	}
	return query, true
}

const rechargeAuditSelect = `t.id, t.user_id, COALESCE(u.username, '') AS username, a.site_id, t.trade_no, t.payment_method, t.payment_provider,
	t.status, t.create_time, t.complete_time, t.amount, t.money,
	(a.id IS NOT NULL) AS has_ext,
	COALESCE(a.face_usd, 0) AS face_usd, COALESCE(a.bonus_usd, 0) AS bonus_usd, COALESCE(a.credit_quota, 0) AS credit_quota,
	COALESCE(a.pay_amount, 0) AS pay_amount, COALESCE(a.pay_currency, '') AS pay_currency,
	COALESCE(a.paid_amount, 0) AS paid_amount, COALESCE(a.paid_currency, '') AS paid_currency,
	COALESCE(a.paid_tx_hash, '') AS paid_tx_hash, COALESCE(a.paid_source, '') AS paid_source,
	COALESCE(a.channel_id, 0) AS channel_id`

// GetRechargeAudit 分页列出充值订单，带面额、加赠、应付、实付和异常标记。
func GetRechargeAudit(c *gin.Context) {
	getRechargeAudit(c, nil)
}

// GetAdtyOwnerOrders uses the order's frozen site, even after a customer moves.
func GetAdtyOwnerOrders(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if ok {
		getRechargeAudit(c, &site.Id)
	}
}

func getRechargeAudit(c *gin.Context, ownerSiteID *int) {
	query, ok := rechargeAuditQuery(c, ownerSiteID)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var orders []rechargeAuditOrder
	if err := query.Select(rechargeAuditSelect).Order("t.create_time DESC, t.id DESC").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Scan(&orders).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	siteIDs := make([]int, 0, len(orders))
	for _, order := range orders {
		if order.SiteId != nil {
			siteIDs = append(siteIDs, *order.SiteId)
		}
	}
	siteNames, err := service.AdtySiteNames(siteIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelNames := adtyEpayChannelNames()
	capabilities := make(map[string]service.AdtyPaymentCapability)
	for _, capability := range service.AdtyPaymentCapabilities() {
		capabilities[capability.Provider] = capability
	}
	for i := range orders {
		if ownerSiteID != nil && orders[i].PaidSource == model.AdtyPaymentSourceManual {
			orders[i].PaidTxHash = ""
		}
		orders[i].OrderType = "topup"
		if orders[i].SiteId != nil {
			orders[i].SiteName = siteNames[*orders[i].SiteId]
		}
		orders[i].Anomaly = rechargeAuditAnomaly(orders[i])
		orders[i].ChannelName = channelNames[orders[i].ChannelId]
		capability, supported := capabilities[orders[i].PaymentProvider]
		if ownerSiteID != nil || !supported || !orders[i].HasExt {
			continue
		}
		switch orders[i].Status {
		case common.TopUpStatusPending, common.TopUpStatusExpired, common.TopUpStatusFailed, model.TopUpStatusCanceled:
			orders[i].Actions.Complete = capability.Complete
			orders[i].Actions.Query = capability.Query
		case common.TopUpStatusSuccess:
			orders[i].Actions.Refund = capability.Refund
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(orders)
	common.ApiSuccess(c, pageInfo)
}

// adtyEpayChannelNames 由 P5 换成通道表的名字；没有通道时是空表。
var adtyEpayChannelNames = func() map[int]string { return map[int]string{} }

// rechargeAuditAnomaly 标出两类异常：成功订单缺扩展记录，实付和应付对不上。
func rechargeAuditAnomaly(order rechargeAuditOrder) string {
	ours := model.IsAdtyPaymentProvider(order.PaymentProvider)
	if !ours {
		return ""
	}
	if !order.HasExt {
		return "missing_ext"
	}
	if order.Status != common.TopUpStatusSuccess && order.Status != model.TopUpStatusRefunded {
		return ""
	}
	if order.PaidSource == model.AdtyPaymentSourceManual {
		return ""
	}
	if order.PaymentProvider == model.PaymentProviderAdtyEpay && !order.PaidAmount.Round(2).Equal(order.PayAmount.Round(2)) {
		return "paid_mismatch"
	}
	if order.PaymentProvider == model.PaymentProviderAdtyBEpusdt && order.PaidAmount.LessThan(order.PayAmount) {
		return "paid_mismatch"
	}
	return ""
}

type rechargeAuditSummaryRow struct {
	Status   string          `json:"status"`
	Provider string          `json:"provider"`
	Count    int64           `json:"count"`
	FaceUSD  decimal.Decimal `json:"face_usd"`
	BonusUSD decimal.Decimal `json:"bonus_usd"`
}

// GetRechargeAuditSummary 按状态和支付商汇总，金额都是美元。
func GetRechargeAuditSummary(c *gin.Context) {
	getRechargeAuditSummary(c, nil)
}

func GetAdtyOwnerOrderSummary(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if ok {
		getRechargeAuditSummary(c, &site.Id)
	}
}

func getRechargeAuditSummary(c *gin.Context, ownerSiteID *int) {
	query, ok := rechargeAuditQuery(c, ownerSiteID)
	if !ok {
		return
	}
	var rows []rechargeAuditSummaryRow
	if err := query.Select(`t.status AS status, t.payment_provider AS provider, COUNT(*) AS count,
		COALESCE(SUM(COALESCE(a.face_usd, t.amount)), 0) AS face_usd, COALESCE(SUM(COALESCE(a.bonus_usd, 0)), 0) AS bonus_usd`).
		Group("t.status, t.payment_provider").Scan(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	credited := decimal.Zero
	bonus := decimal.Zero
	var orders int64
	for _, row := range rows {
		orders += row.Count
		if row.Status == common.TopUpStatusSuccess {
			credited = credited.Add(row.FaceUSD)
			bonus = bonus.Add(row.BonusUSD)
		}
	}
	capabilities := service.AdtyPaymentCapabilities()
	providers := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		providers = append(providers, capability.Provider)
	}
	// Keep choices independent of active filters and within the caller's scope.
	sitesQuery := model.DB.Table("top_ups AS t").Joins("JOIN adty_topup_orders a ON a.trade_no = t.trade_no")
	if ownerSiteID != nil {
		sitesQuery = sitesQuery.Where("a.site_id = ?", *ownerSiteID)
	}
	var siteIDs []int
	if err := sitesQuery.Distinct("a.site_id").Order("a.site_id").Pluck("a.site_id", &siteIDs).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	siteNames, err := service.AdtySiteNames(siteIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	type siteOption struct {
		SiteId   int    `json:"site_id"`
		SiteName string `json:"site_name"`
	}
	siteOptions := make([]siteOption, 0, len(siteIDs))
	for _, id := range siteIDs {
		siteOptions = append(siteOptions, siteOption{SiteId: id, SiteName: siteNames[id]})
	}
	common.ApiSuccess(c, gin.H{
		"rows":                  rows,
		"orders":                orders,
		"credited_usd":          credited,
		"bonus_usd":             bonus,
		"currency":              "USD",
		"status_options":        rechargeAuditStatuses,
		"provider_option":       providers,
		"provider_capabilities": capabilities,
		"site_options":          siteOptions,
	})
}
