package controller

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 主站的分站管理（任务 11.9 的后端）。全部用 handlePermissionRoute 按「分站」权限注册（router/adty-router.go）。

func siteIdParam(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	return id, true
}

// parseOptionalDecimal 把请求里的字符串数字转成 decimal；空串或没给返回 nil。
func parseOptionalDecimal(raw *string) (*decimal.Decimal, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	value, err := decimal.NewFromString(strings.TrimSpace(*raw))
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// adtySiteAdminView 是后台列表和明细里的一行：站点加站长、客户数、本月数字和生效的 d、k 上限。
type adtySiteAdminView struct {
	model.AdtySite
	OwnerUsername       string `json:"owner_username"`
	Host                string `json:"host"`
	Customers           int64  `json:"customers"`
	EffectiveCostFactor string `json:"effective_cost_factor"` // 当前生效的 d（单站 > 档位 > 全局，不低于下限）
	EffectiveMaxMarkup  string `json:"effective_max_markup"`
	service.AdtySiteMonthTotals
}

func buildSiteAdminViews(sites []model.AdtySite) ([]adtySiteAdminView, error) {
	ownerIds := make([]int, 0, len(sites))
	siteIds := make([]int, 0, len(sites))
	for _, site := range sites {
		ownerIds = append(ownerIds, site.OwnerUserId)
		siteIds = append(siteIds, site.Id)
	}
	names := map[int]string{}
	if len(ownerIds) > 0 {
		var owners []model.User
		if err := model.DB.Unscoped().Select("id", "username").Where("id IN ?", ownerIds).Find(&owners).Error; err != nil {
			return nil, err
		}
		for _, owner := range owners {
			names[owner.Id] = owner.Username
		}
	}
	counts := map[int]int64{}
	if len(siteIds) > 0 {
		var rows []struct {
			SiteId int
			Total  int64
		}
		if err := model.DB.Model(&model.User{}).Select("site_id, COUNT(*) AS total").Where("site_id IN ?", siteIds).Group("site_id").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			counts[row.SiteId] = row.Total
		}
	}
	totals, err := service.AdtySitesMonthTotals(siteIds, time.Now())
	if err != nil {
		return nil, err
	}
	globalMax := ""
	if cfg := adty_setting.GetResellerSetting(); cfg.MaxMarkup != nil {
		globalMax = decimal.NewFromFloat(*cfg.MaxMarkup).String()
	}
	views := make([]adtySiteAdminView, 0, len(sites))
	for _, site := range sites {
		view := adtySiteAdminView{AdtySite: site, OwnerUsername: names[site.OwnerUserId], Host: service.AdtySiteDisplayHost(&site),
			Customers: counts[site.Id], AdtySiteMonthTotals: totals[site.Id], EffectiveMaxMarkup: globalMax}
		if d, ok := service.AdtySiteCostFactor(&site); ok {
			view.EffectiveCostFactor = d.String()
		}
		if site.MaxMarkup != nil {
			view.EffectiveMaxMarkup = site.MaxMarkup.String()
		}
		views = append(views, view)
	}
	return views, nil
}

// adtySiteSortKeys 是列表能排序的列：sort=字段，order=asc|desc（默认 desc）。
var adtySiteSortKeys = map[string]func(a, b adtySiteAdminView) int{
	"id":        func(a, b adtySiteAdminView) int { return cmp.Compare(a.Id, b.Id) },
	"customers": func(a, b adtySiteAdminView) int { return cmp.Compare(a.Customers, b.Customers) },
	"topup":     func(a, b adtySiteAdminView) int { return a.TopUpUSD.Cmp(b.TopUpUSD) },
	"revenue":   func(a, b adtySiteAdminView) int { return a.RevenueUSD.Cmp(b.RevenueUSD) },
	"earning":   func(a, b adtySiteAdminView) int { return a.EarningUSD.Cmp(b.EarningUSD) },
	"growth": func(a, b adtySiteAdminView) int {
		// 上月没有消费（没有增长率）的排在最小的一端。
		switch {
		case a.GrowthPct == nil && b.GrowthPct == nil:
			return 0
		case a.GrowthPct == nil:
			return -1
		case b.GrowthPct == nil:
			return 1
		}
		return cmp.Compare(*a.GrowthPct, *b.GrowthPct)
	},
}

// GetAdtyAdminSites 列出分站（分站·查看）：按状态、子域名或站长用户名筛选；带客户数、本月充值、消费、
// 平台收入、站长收益和比上月的增长；sort 可按 id、customers、topup、revenue、earning、growth 排序，order=asc|desc。
// 本月数字要从日汇总算，分站数量有限，所以先取出筛选后的站点（最多 5000 个），排序后再分页。
func GetAdtyAdminSites(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	query := model.DB.Model(&model.AdtySite{})
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		op := "LIKE"
		if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
			op = "ILIKE"
		}
		like := "%" + keyword + "%"
		query = query.Where("subdomain "+op+" ? OR owner_user_id IN (?)", like,
			model.DB.Model(&model.User{}).Select("id").Where("username "+op+" ?", like))
	}
	compare, ok := adtySiteSortKeys[strings.TrimSpace(c.DefaultQuery("sort", "id"))]
	if !ok {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var sites []model.AdtySite
	if err := query.Order("id DESC").Limit(5000).Find(&sites).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	views, err := buildSiteAdminViews(sites)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ascending := c.Query("order") == "asc"
	slices.SortStableFunc(views, func(a, b adtySiteAdminView) int {
		if ascending {
			return compare(a, b)
		}
		return compare(b, a)
	})
	start := min(pageInfo.GetStartIdx(), len(views))
	end := min(start+pageInfo.GetPageSize(), len(views))
	pageInfo.SetTotal(len(views))
	pageInfo.SetItems(views[start:end])
	common.ApiSuccess(c, pageInfo)
}

// GetAdtyAdminSiteStats 是一个站的经营数据（分站·查看）：GET /api/adty/admin/sites/:id/stats?from&to。
// 和站长中心同一份按天的数据，另外带按模型、分组的消费和按月的序列。
func GetAdtyAdminSiteStats(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	site, err := service.GetAdtySite(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	stats, err := service.GetAdtySiteStats(id, c.Query("from"), c.Query("to"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	breakdown, err := service.GetAdtySiteUsageBreakdown(id, stats.From, stats.To)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	body := gin.H{"stats": stats, "breakdown": breakdown, "monthly": service.AdtyMonthlySeries(stats.Series)}
	// 提现属于推广账户。没有推广·查看时不返回（方案 11.9）。
	if service.AdtyAdminCan(c.GetInt("id"), authz.AdtyReferralRead) {
		balances, err := service.GetAdtySiteBalances(site)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		body["withdrawn_usd"] = balances.WithdrawnUSD
	}
	common.ApiSuccess(c, body)
}

// GetAdtyAdminSitesOverview 是全部分站的汇总，和主站直客对比（分站·查看）：GET /api/adty/admin/sites/overview?from&to。
func GetAdtyAdminSitesOverview(c *gin.Context) {
	overview, err := service.GetAdtySitesOverview(c.Query("from"), c.Query("to"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, overview)
}

// GetAdtyAdminFlaggedCustomers 列出风控客户（分站·查看）：GET /api/adty/admin/sites/flagged-customers?all=1。
// 默认只列没复核的；后台能看到判断依据（risk_reason），站长看不到。
func GetAdtyAdminFlaggedCustomers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	query := model.DB.Model(&model.AdtySiteCustomer{}).Where("risk_flag <> ''")
	if c.Query("all") != "1" {
		query = query.Where("reviewed = ?", false)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var rows []model.AdtySiteCustomer
	if err := query.Order("joined_at DESC").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	userIds := make([]int, 0, len(rows))
	for _, row := range rows {
		userIds = append(userIds, row.UserId)
	}
	skipped := map[int]decimal.Decimal{}
	if len(userIds) > 0 {
		var sums []struct {
			UserId int
			Total  decimal.Decimal
		}
		if err := model.DB.Model(&model.AdtySiteEarningDaily{}).Select("user_id, COALESCE(SUM(earning),0) AS total").
			Where("user_id IN ? AND kind = ? AND status = ?", userIds, model.AdtySiteEarningSkipped, model.AdtySiteEarningPending).
			Group("user_id").Scan(&sums).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		for _, sum := range sums {
			skipped[sum.UserId] = sum.Total
		}
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"user_id": row.UserId, "site_id": row.SiteId, "risk_flag": row.RiskFlag, "risk_reason": row.RiskReason,
			"reviewed": row.Reviewed, "joined_at": row.JoinedAt, "skipped_usd": skipped[row.UserId].StringFixed(2)})
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GetAdtyAdminSiteReports 列出站长的举报（分站·查看）：GET /api/adty/admin/sites/reports?status=pending|resolved|dismissed|all。
func GetAdtyAdminSiteReports(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	query := model.DB.Model(&model.AdtySiteReport{})
	if status := strings.TrimSpace(c.DefaultQuery("status", model.AdtySiteReportPending)); status != "all" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var reports []model.AdtySiteReport
	if err := query.Order("id DESC").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&reports).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(reports)
	common.ApiSuccess(c, pageInfo)
}

// HandleAdtyAdminSiteReport 处理举报（分站·审核）：POST /api/adty/admin/sites/reports/:id/handle {status: resolved|dismissed, note}。
func HandleAdtyAdminSiteReport(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	report, err := service.HandleAdtySiteReport(id, c.GetInt("id"), body.Status, body.Note)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, report.UserId, "adty.site.report", map[string]any{"report_id": report.Id, "status": report.Status})
	common.ApiSuccess(c, report)
}

// GetAdtyAdminSite 是一个分站的明细（分站·查看），带最近的操作记录。
func GetAdtyAdminSite(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	site, err := service.GetAdtySite(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views, err := buildSiteAdminViews([]model.AdtySite{*site})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var audits []model.AdtySiteAudit
	if err := model.DB.Where("site_id = ?", id).Order("id DESC").Limit(50).Find(&audits).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"site": views[0], "audits": audits})
}

// GetAdtyAdminSiteCustomers 列出一个分站的客户（分站·查看）。没有用户查看权限时邮箱打码。
func GetAdtyAdminSiteCustomers(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	query := model.DB.Model(&model.User{}).Where("site_id = ?", id)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var users []model.User
	if err := query.Select("id", "username", "email", "quota", "used_quota", "status", "group", "created_at").
		Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&users).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	userIds := make([]int, 0, len(users))
	for _, user := range users {
		userIds = append(userIds, user.Id)
	}
	risks := map[int]model.AdtySiteCustomer{}
	if len(userIds) > 0 {
		var rows []model.AdtySiteCustomer
		if err := model.DB.Where("user_id IN ?", userIds).Find(&rows).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		for _, row := range rows {
			risks[row.UserId] = row
		}
	}
	showEmail := authz.Can(c.GetInt("id"), c.GetInt("role"), authz.AdtyUserRead)
	items := make([]gin.H, 0, len(users))
	for _, user := range users {
		risk := risks[user.Id]
		email := service.AdtyMaskEmail(user.Email)
		if showEmail {
			email = user.Email
		}
		items = append(items, gin.H{
			"id": user.Id, "username": user.Username, "email": email, "quota": user.Quota, "used_quota": user.UsedQuota,
			"status": user.Status, "group": user.Group, "created_at": user.CreatedAt,
			"risk_flag": risk.RiskFlag, "risk_reason": risk.RiskReason, "reviewed": risk.Reviewed, "joined_at": risk.JoinedAt,
		})
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// CreateAdtyAdminSite 是管理员直接开站（分站·调整）：{user_id, subdomain, root_domain?, cost_factor?, max_markup?, contact_info?}。
func CreateAdtyAdminSite(c *gin.Context) {
	var body struct {
		UserId      int     `json:"user_id"`
		Subdomain   string  `json:"subdomain"`
		RootDomain  string  `json:"root_domain"`
		CostFactor  *string `json:"cost_factor"`
		MaxMarkup   *string `json:"max_markup"`
		ContactInfo string  `json:"contact_info"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.UserId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	costFactor, err1 := parseOptionalDecimal(body.CostFactor)
	maxMarkup, err2 := parseOptionalDecimal(body.MaxMarkup)
	if err1 != nil || err2 != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	site, err := service.OpenAdtySite(body.UserId, body.Subdomain, model.AdtySiteOpenedByAdmin, service.AdtySitePreset{
		CostFactor: costFactor, MaxMarkup: maxMarkup, ContactInfo: body.ContactInfo, OperatorId: c.GetInt("id"),
		ExplicitRoot: strings.ToLower(strings.TrimSpace(body.RootDomain)),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, body.UserId, "adty.site.open", map[string]any{"site_id": site.Id, "subdomain": site.Subdomain})
	common.ApiSuccess(c, site)
}

// ReviewAdtyAdminSite 通过或驳回开站申请（分站·审核）：{approve, cost_factor?, max_markup?, note}。
func ReviewAdtyAdminSite(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	var body struct {
		Approve    bool    `json:"approve"`
		CostFactor *string `json:"cost_factor"`
		MaxMarkup  *string `json:"max_markup"`
		Note       string  `json:"note"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var costFactor, maxMarkup *decimal.Decimal
	if body.Approve {
		var err1, err2 error
		costFactor, err1 = parseOptionalDecimal(body.CostFactor)
		maxMarkup, err2 = parseOptionalDecimal(body.MaxMarkup)
		if err1 != nil || err2 != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	site, err := service.ReviewAdtySiteApplication(id, c.GetInt("id"), body.Approve, costFactor, maxMarkup, body.Note)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, site.OwnerUserId, "adty.site.review", map[string]any{"site_id": site.Id, "approve": body.Approve})
	common.ApiSuccess(c, site)
}

// AdjustAdtyAdminSite 改单站 d、k 上限、可见范围（分站·调整）。clear 列出要改回「用全局」的项：cost_factor、max_markup。
func AdjustAdtyAdminSite(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	var body struct {
		CostFactor *string  `json:"cost_factor"`
		MaxMarkup  *string  `json:"max_markup"`
		Visibility *string  `json:"visibility"`
		Clear      []string `json:"clear"`
		Reason     string   `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	costFactor, err1 := parseOptionalDecimal(body.CostFactor)
	maxMarkup, err2 := parseOptionalDecimal(body.MaxMarkup)
	if err1 != nil || err2 != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	for _, key := range body.Clear {
		if key != "cost_factor" && key != "max_markup" {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	site, err := service.AdjustAdtySite(id, c.GetInt("id"), service.AdtySiteAdjust{
		CostFactor: costFactor, ClearCostFactor: slices.Contains(body.Clear, "cost_factor"),
		MaxMarkup: maxMarkup, ClearMaxMarkup: slices.Contains(body.Clear, "max_markup"),
		Visibility: body.Visibility, Reason: body.Reason,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, site.OwnerUserId, "adty.site.adjust", map[string]any{"site_id": site.Id})
	common.ApiSuccess(c, site)
}

// SetAdtyAdminSiteStatus 暂停、恢复或关站（分站·调整）：
// {status: active|paused|closed, transfer_to?, claw_back?, immediate?, reason}。关站必须写清客户去向（transfer_to 0 是主站）。
func SetAdtyAdminSiteStatus(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	var body struct {
		Status     string `json:"status"`
		TransferTo *int   `json:"transfer_to"`
		ClawBack   bool   `json:"claw_back"`
		Immediate  bool   `json:"immediate"`
		Reason     string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var site *model.AdtySite
	var err error
	switch body.Status {
	case model.AdtySiteStatusActive, model.AdtySiteStatusPaused:
		site, err = service.SetAdtySiteStatus(id, c.GetInt("id"), body.Status, body.Reason)
	case model.AdtySiteStatusClosed:
		if body.TransferTo == nil {
			common.ApiErrorT(c, "Choose where the customers go before closing the site")
			return
		}
		site, err = service.ScheduleAdtySiteClose(id, c.GetInt("id"), service.AdtySiteClosePlan{
			TransferTo: *body.TransferTo, ClawBack: body.ClawBack, Immediate: body.Immediate, Reason: body.Reason,
		})
	default:
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, site.OwnerUserId, "adty.site.status", map[string]any{"site_id": site.Id, "status": site.Status})
	common.ApiSuccess(c, site)
}

// ResumeAdtySiteSettlement 清掉对账暂停并立刻再结一次。数字还是对不上时响应里仍有 settle_hold_reason。
func ResumeAdtySiteSettlement(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	site, err := service.ResumeAdtySiteSettlement(id, c.GetInt("id"), time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, site.OwnerUserId, "adty.site.settlement_resume", map[string]any{
		"site_id": site.Id, "settle_hold_reason": site.SettleHoldReason,
	})
	common.ApiSuccess(c, site)
}

// adtyResellerSettingsView 是后台读到的分站设置：密钥清空，只返回「已配置」；另给出各开关依赖的项是否齐全，
// 后台在没配好的项旁边显示「未配置」。
type adtyResellerSettingsView struct {
	adty_setting.ResellerSetting
	CloudflareTokenConfigured bool            `json:"cloudflare_token_configured"`
	WidgetSecretConfigured    map[string]bool `json:"turnstile_secret_configured"`
	CloudflareConfigured      bool            `json:"cloudflare_configured"`
	ResendConfigured          bool            `json:"resend_configured"`
	MoneyConfigured           bool            `json:"money_configured"`
	SiteAgreementVersion      int             `json:"site_agreement_version"`
	SiteCustomerTermsVersion  int             `json:"site_customer_terms_version"`
}

func adtyResendConfigured() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.OptionMap["ResendApiKey"]) != ""
}

// GetAdtyResellerSettings 读分站设置（分站·设置）。
func GetAdtyResellerSettings(c *gin.Context) {
	cfg := *adty_setting.GetResellerSetting()
	view := adtyResellerSettingsView{
		CloudflareTokenConfigured: strings.TrimSpace(cfg.CloudflareApiToken) != "",
		WidgetSecretConfigured:    map[string]bool{},
		CloudflareConfigured:      cfg.CloudflareConfigured(),
		ResendConfigured:          adtyResendConfigured(),
		MoneyConfigured:           cfg.ResellerMoneyConfigured(),
		SiteAgreementVersion:      adty_setting.GetLegalSetting().SiteAgreementVersion,
		SiteCustomerTermsVersion:  adty_setting.GetLegalSetting().SiteCustomerTermsVersion,
	}
	for _, widget := range cfg.TurnstileWidgets {
		view.WidgetSecretConfigured[widget.Id] = strings.TrimSpace(cfg.TurnstileWidgetSecret[widget.Id]) != ""
	}
	cfg.CloudflareApiToken, cfg.TurnstileWidgetSecret = "", nil
	view.ResellerSetting = cfg
	common.ApiSuccess(c, view)
}

// UpdateAdtyResellerSettings 保存分站设置（分站·设置）。请求体是要改的键（json 标签 → JSON 值），
// 金额项给 null 表示改回未设置。合并到当前值上整体校验，通过后一次写进 options，保存后立即生效。
// 密钥项给空串表示不改；要清空密钥时在 clear_secrets 里列出键名。
func UpdateAdtyResellerSettings(c *gin.Context) {
	var body map[string]common.RawMessage
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var clearSecrets []string
	if raw, ok := body["clear_secrets"]; ok {
		if err := common.Unmarshal(raw, &clearSecrets); err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		delete(body, "clear_secrets")
	}
	current := *adty_setting.GetResellerSetting()
	var previousCostFactor *float64
	if current.CostFactor != nil {
		copied := *current.CostFactor
		previousCostFactor = &copied
	}
	var previousMin *float64
	if current.MinCostFactor != nil {
		copied := *current.MinCostFactor
		previousMin = &copied
	}
	known, err := config.ConfigToMap(&current)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	values := map[string]string{}
	for key, raw := range body {
		if _, ok := known[key]; !ok {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		text := strings.TrimSpace(string(raw))
		var asString string
		if strings.HasPrefix(text, `"`) && common.Unmarshal(raw, &asString) == nil {
			text = asString
		}
		if (key == adty_setting.ResellerKeyCloudflareTok || key == adty_setting.ResellerKeyWidgetSecret) && (text == "" || text == "null") {
			continue
		}
		if key == adty_setting.ResellerKeyWidgetSecret {
			// 只合并给出的 widget，其余 secret 保留。
			merged := map[string]string{}
			for id, secret := range current.TurnstileWidgetSecret {
				merged[id] = secret
			}
			incoming := map[string]string{}
			if err := common.UnmarshalJsonStr(text, &incoming); err != nil {
				common.ApiErrorT(c, "Invalid parameters")
				return
			}
			for id, secret := range incoming {
				if strings.TrimSpace(secret) != "" {
					merged[id] = strings.TrimSpace(secret)
				}
			}
			encoded, _ := common.Marshal(merged)
			text = string(encoded)
		}
		values[key] = text
	}
	for _, key := range clearSecrets {
		switch key {
		case adty_setting.ResellerKeyCloudflareTok:
			values[key] = ""
		case adty_setting.ResellerKeyWidgetSecret:
			values[key] = "{}"
		default:
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	next := current
	// 切片和 map 要换成新的，不能和当前设置共享底层数组。
	next.RootDomains, next.ReservedSubdomains, next.CostFactorTiers = nil, nil, nil
	next.Visibility, next.TurnstileWidgets, next.TurnstileWidgetSecret = nil, nil, nil
	merged := map[string]string{}
	for key, value := range known {
		merged[key] = value
	}
	for key, value := range values {
		merged[key] = value
	}
	if err := config.UpdateConfigFromMap(&next, merged); err != nil {
		common.ApiError(c, err)
		return
	}
	// UpdateConfigFromMap 遇到解析不了的值会静默跳过，这里逐项比对，防止写进去的字符串和生效的值不一致。
	parsed, _ := config.ConfigToMap(&next)
	for key, value := range values {
		if !adtyConfigValueEqual(parsed[key], value) {
			common.ApiErrorT(c, "Invalid value for {{key}}", map[string]any{"key": key})
			return
		}
	}
	for i := range next.RootDomains {
		next.RootDomains[i] = strings.ToLower(strings.TrimSpace(next.RootDomains[i]))
	}
	for i := range next.ReservedSubdomains {
		next.ReservedSubdomains[i] = strings.ToLower(strings.TrimSpace(next.ReservedSubdomains[i]))
	}
	baseline := current
	baseline.MinCostFactor = previousMin
	if adty_setting.HoldMinCostFactorIncrease(&next, &baseline, time.Now().Unix()) {
		values["min_cost_factor"] = ""
		values["pending_min_cost_factor"] = ""
		values["pending_min_cost_factor_at"] = ""
		if next.PendingMinCostFactor != nil {
			service.NotifySiteOwnersGlobalCostFactorPending(decimal.NewFromFloat(*next.PendingMinCostFactor), next.PendingMinCostFactorAt)
		}
	}
	if err := adty_setting.ValidateResellerSetting(&next, adtyResendConfigured()); err != nil {
		common.ApiError(c, err)
		return
	}
	previous := current
	previous.CostFactor = previousCostFactor
	if adty_setting.HoldGlobalCostFactorIncrease(&next, &previous, time.Now().Unix()) {
		values["pending_cost_factor"] = ""
		values["pending_cost_factor_at"] = ""
		values["cost_factor"] = ""
		if next.PendingCostFactor != nil {
			service.NotifySiteOwnersGlobalCostFactorPending(decimal.NewFromFloat(*next.PendingCostFactor), next.PendingCostFactorAt)
		}
	}
	if len(values) > 0 {
		normalized, _ := config.ConfigToMap(&next)
		options := make(map[string]string, len(values))
		for key := range values {
			options[adty_setting.ResellerSettingModule+"."+key] = normalized[key]
		}
		if err := model.UpdateOptionsBulk(options); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	service.InvalidateAdtySiteCache()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	recordManageAudit(c, "adty.site.settings", map[string]any{"keys": strings.Join(keys, ",")})
	GetAdtyResellerSettings(c)
}

// adtyConfigValueEqual 比较请求里的值和解析后再导出的值（JSON 的空白、数字写法可能不同）。
func adtyConfigValueEqual(parsed, requested string) bool {
	if parsed == requested {
		return true
	}
	var a, b any
	if common.UnmarshalJsonStr(parsed, &a) != nil || common.UnmarshalJsonStr(requested, &b) != nil {
		return false
	}
	left, _ := common.Marshal(a)
	right, _ := common.Marshal(b)
	return string(left) == string(right)
}

// GetAdtySiteInviteCodes 列出开站邀请码（分站·设置）。
func GetAdtySiteInviteCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var total int64
	if err := model.DB.Model(&model.AdtySiteInviteCode{}).Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var codes []model.AdtySiteInviteCode
	if err := model.DB.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&codes).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

type adtySiteInviteBody struct {
	Code       string  `json:"code"`
	MaxUses    int     `json:"max_uses"`
	ExpireAt   int64   `json:"expire_at"`
	CostFactor *string `json:"cost_factor"`
	MaxMarkup  *string `json:"max_markup"`
	Note       string  `json:"note"`
}

func saveAdtySiteInvite(c *gin.Context, id int) {
	var body adtySiteInviteBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	costFactor, err1 := parseOptionalDecimal(body.CostFactor)
	maxMarkup, err2 := parseOptionalDecimal(body.MaxMarkup)
	if err1 != nil || err2 != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	code := &model.AdtySiteInviteCode{Id: id, Code: body.Code, MaxUses: body.MaxUses, ExpireAt: body.ExpireAt,
		CostFactor: costFactor, MaxMarkup: maxMarkup, Note: body.Note, CreatedBy: c.GetInt("id")}
	if id > 0 {
		var existing model.AdtySiteInviteCode
		if err := model.DB.First(&existing, id).Error; err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		code.Code = existing.Code
	}
	if err := service.SaveAdtySiteInviteCode(code); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.site.invite", map[string]any{"invite_id": code.Id, "code": code.Code})
	var stored model.AdtySiteInviteCode
	if err := model.DB.First(&stored, code.Id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stored)
}

// CreateAdtySiteInviteCode 新建开站邀请码（分站·设置），code 为空就自动生成。
func CreateAdtySiteInviteCode(c *gin.Context) { saveAdtySiteInvite(c, 0) }

// UpdateAdtySiteInviteCode 改开站邀请码的次数、有效期、预设和备注（码本身不改）。
func UpdateAdtySiteInviteCode(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	saveAdtySiteInvite(c, id)
}

// DeleteAdtySiteInviteCode 删除开站邀请码；已经用它开的站不受影响。
func DeleteAdtySiteInviteCode(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	result := model.DB.Delete(&model.AdtySiteInviteCode{}, id)
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	recordManageAudit(c, "adty.site.invite_delete", map[string]any{"invite_id": id})
	common.ApiSuccess(c, gin.H{"deleted_at": time.Now().Unix()})
}

// ReviewAdtySiteCustomer 复核风控客户（分站·审核）：POST /api/adty/admin/sites/customers/:id/review {convert_skipped}。
// 认可后之后的消费正常计收益；convert_skipped 时还没结算的 skipped 收益改成 usage。
func ReviewAdtySiteCustomer(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	var body struct {
		ConvertSkipped bool `json:"convert_skipped"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.ReviewAdtySiteCustomer(id, c.GetInt("id"), body.ConvertSkipped); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, id, "adty.site.customer_review", map[string]any{"user_id": id, "convert_skipped": body.ConvertSkipped})
	common.ApiSuccess(c, gin.H{"user_id": id, "reviewed": true})
}

// ClearAdtyAdminSiteNotice 一键下线分站公告（分站·调整）：DELETE /api/adty/admin/sites/:id/notice {reason?}。
func ClearAdtyAdminSiteNotice(c *gin.Context) {
	id, ok := siteIdParam(c)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if c.Request.ContentLength > 0 {
		if err := common.DecodeJson(c.Request.Body, &body); err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	old, err := service.ClearAdtySiteNotice(id, c.GetInt("id"), body.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.site.notice_takedown", map[string]any{"site_id": id, "had_notice": old != ""})
	common.ApiSuccess(c, gin.H{"site_id": id, "cleared": old != ""})
}

// GetAdtyResellerUsage 是 Resend 发件域名和 Turnstile widget 的名额用量（分站·查看）：GET /api/adty/admin/reseller/usage。
func GetAdtyResellerUsage(c *gin.Context) {
	usage, err := service.GetAdtyResellerUsage()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, usage)
}
