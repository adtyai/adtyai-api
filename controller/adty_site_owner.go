package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 站长中心（任务 11.6、11.7）。每个处理函数先取「当前用户是 active、paused 或 closing 站点的站长」，否则 403；
// 查询一律带 site_id = 本站。改价、改加赠、送试用额度、删除域名要重新验证身份（X-Security-Proof，
// scope site.owner.manage，绑定操作和目标）。

// adtyOwnerSiteOr403 取当前用户的站；不是站长时写 403。
func adtyOwnerSiteOr403(c *gin.Context) (*model.AdtySite, bool) {
	site, err := service.AdtyOwnerSite(c.GetInt("id"))
	if errors.Is(err, service.ErrAdtySiteNotOwner) {
		common.ApiErrorStatus(c, http.StatusForbidden, service.ErrAdtySiteNotOwner)
		return nil, false
	}
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if site.AgreementVer != service.AdtyCurrentSiteAgreementVersion() {
		common.ApiErrorStatus(c, http.StatusForbidden, service.ErrAdtySiteAgreement)
		return nil, false
	}
	return site, true
}

// AcceptAdtySiteTerms 接受当前开站协议：POST /api/adty/site/terms/accept。不走站长中心的协议门。
func AcceptAdtySiteTerms(c *gin.Context) {
	var body struct {
		Version string `json:"version"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.AcceptAdtySiteTerms(c.GetInt("id"), body.Version); err != nil {
		if errors.Is(err, service.ErrAdtySiteNotOwner) || errors.Is(err, service.ErrAdtySiteAgreement) {
			common.ApiErrorStatus(c, http.StatusForbidden, err)
			return
		}
		common.ApiError(c, err)
		return
	}
	required, version := service.AdtySiteTermsStatus(c.GetInt("id"))
	common.ApiSuccess(c, gin.H{
		"site_terms_required": required,
		"site_terms_version":  version,
	})
}

// adtyRequireOwnerProof 要求这次站长操作带上有效的二次验证 proof；失败时已经写好响应。
func adtyRequireOwnerProof(c *gin.Context, action string, target int) bool {
	return middleware.RequireSecurityProof(c, service.AdtySiteOwnerOperation(action, target)) != nil
}

func adtyPathId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	return id, true
}

func adtyPage(c *gin.Context) (*common.PageInfo, int, int) {
	pageInfo := common.GetPageQuery(c)
	return pageInfo, pageInfo.GetStartIdx(), pageInfo.GetPageSize()
}

// GetAdtyOwnerSite 是站点资料：GET /api/adty/site/self。站长能看到自己的 d（要会算利润），看不到别人的。
func GetAdtyOwnerSite(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	d, _ := service.AdtySiteCostFactor(site)
	limit := ""
	if site.MaxMarkup != nil {
		limit = site.MaxMarkup.String()
	} else if cfg := adty_setting.GetResellerSetting(); cfg.MaxMarkup != nil {
		limit = decimal.NewFromFloat(*cfg.MaxMarkup).String()
	}
	// 站长编辑用的原始品牌字段（站名没填时为空，不像 AdtyBrandFor 那样回退成子域名）。
	var brand service.AdtySiteBrandInput
	if strings.TrimSpace(site.Brand) != "" {
		_ = common.UnmarshalJsonStr(site.Brand, &brand)
	}
	var prices []model.AdtySitePrice
	if err := model.DB.Where("site_id = ? AND user_id = 0", site.Id).Order("id").Find(&prices).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var version int64
	for _, price := range prices {
		version = max(version, price.Version)
	}
	domains, err := service.ListAdtySiteDomains(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg := adty_setting.GetResellerSetting()
	common.ApiSuccess(c, gin.H{
		"id": site.Id, "status": site.Status, "subdomain": site.Subdomain, "root_domain": site.RootDomain,
		"host": service.AdtySiteDisplayHost(site), "base_url": service.AdtySiteBaseURL(site),
		"cost_factor": d.String(), "markup_min": "1", "markup_max": limit,
		"markup_increase_delay_hours": cfg.MarkupIncreaseDelayHours, "custom_domain_enabled": cfg.CustomDomainEnabled,
		"sender_domain_enabled": cfg.SenderDomainEnabled, "brand": brand, "bonus_rates": service.AdtySiteBonusRates(site),
		"seo_index": site.SeoIndex, "agreement_version": site.AgreementVer, "agreement_at": site.AgreementAt,
		"prices": prices, "pricing_version": version, "domains": domains, "visibility": service.AdtySiteVisibility(site),
	})
}

// PutAdtyOwnerSite 改站点资料：PUT /api/adty/site/self {brand?, bonus_rates?, seo_index?}。
// 改加赠要二次验证（target = 站点 id）；品牌和 SEO 开关不用。站点 paused 时不能改加赠。
func PutAdtyOwnerSite(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		Brand      *service.AdtySiteBrandInput `json:"brand"`
		BonusRates *map[int]float64            `json:"bonus_rates"`
		SeoIndex   *bool                       `json:"seo_index"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if (body.Brand != nil || body.SeoIndex != nil) && site.Status == model.AdtySiteStatusClosing {
		common.ApiError(c, service.ErrAdtySiteClosing)
		return
	}
	if body.BonusRates != nil {
		if err := service.ValidateAdtySiteBonusRates(*body.BonusRates); err != nil {
			common.ApiError(c, err)
			return
		}
		if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionBonus, site.Id) {
			return
		}
		if err := service.SaveAdtySiteBonusRates(site.Id, *body.BonusRates); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	updates := map[string]any{}
	if body.Brand != nil {
		// 品牌字段的校验和整理在 11.3 的 NormalizeAdtySiteBrand：纯文本、字数上限、Logo 只能是本站上传的图片。
		brand, err := service.NormalizeAdtySiteBrand(*body.Brand)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		encoded, err := common.Marshal(brand)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		updates["brand"] = string(encoded)
	}
	if body.SeoIndex != nil {
		updates["seo_index"] = *body.SeoIndex
	}
	if len(updates) > 0 {
		result := model.DB.Model(&model.AdtySite{}).Where("id = ? AND status <> ?", site.Id, model.AdtySiteStatusClosing).Updates(updates)
		if result.Error != nil {
			common.ApiError(c, result.Error)
			return
		}
		if result.RowsAffected == 0 {
			common.ApiError(c, service.ErrAdtySiteClosing)
			return
		}
		service.InvalidateAdtySiteCache()
	}
	GetAdtyOwnerSite(c)
}

// GetAdtyOwnerStats 是经营数据：GET /api/adty/site/stats?from&to（日期 2006-01-02）。
func GetAdtyOwnerStats(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	stats, err := service.GetAdtySiteStats(site.Id, c.Query("from"), c.Query("to"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

// adtyOwnerEarningRow 是站长看到的一条收益：skipped 只给原因类别（self_use | migration），不给判断依据。
type adtyOwnerEarningRow struct {
	Day       string          `json:"day"`
	UserId    int             `json:"user_id"`
	Kind      string          `json:"kind"`
	Reason    string          `json:"reason,omitempty"`
	Revenue   decimal.Decimal `json:"revenue_usd"`
	Earning   decimal.Decimal `json:"earning_usd"`
	Status    string          `json:"status"`
	SettledAt int64           `json:"settled_at"`
}

// GetAdtyOwnerEarnings 是收益流水：GET /api/adty/site/earnings?kind&p&page_size，带待结算、可提现、已提现。
func GetAdtyOwnerEarnings(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	pageInfo, offset, limit := adtyPage(c)
	query := model.DB.Model(&model.AdtySiteEarningDaily{}).Where("site_id = ?", site.Id)
	if kind := strings.TrimSpace(c.Query("kind")); kind != "" {
		query = query.Where("kind = ?", kind)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var rows []model.AdtySiteEarningDaily
	if err := query.Order("day DESC, id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]adtyOwnerEarningRow, 0, len(rows))
	for _, row := range rows {
		items = append(items, adtyOwnerEarningRow{Day: row.Day, UserId: row.UserId, Kind: row.Kind, Reason: row.Reason,
			Revenue: row.Revenue, Earning: row.Earning, Status: row.Status, SettledAt: row.SettledAt})
	}
	balances, err := service.GetAdtySiteBalances(site)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, gin.H{"balances": balances, "page": pageInfo})
}

// GetAdtyOwnerCustomers 按注册时间分页列出客户：GET /api/adty/site/customers。
func GetAdtyOwnerCustomers(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	pageInfo, offset, limit := adtyPage(c)
	items, total, err := service.ListAdtyOwnerCustomers(site, offset, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// LookupAdtyOwnerCustomer 精确查找：GET /api/adty/site/customers/lookup?q=（完整用户名或邮箱，有次数限制）。
func LookupAdtyOwnerCustomer(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	customer, err := service.LookupAdtyOwnerCustomer(site, c.GetInt("id"), c.Query("q"))
	if errors.Is(err, service.ErrAdtySiteLookupLimited) {
		common.ApiErrorStatus(c, http.StatusTooManyRequests, err)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, customer)
}

// GetAdtyOwnerCustomer 是客户详情：GET /api/adty/site/customers/:id（带密钥列表，customer_tokens 关着时不带）。
func GetAdtyOwnerCustomer(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	customer, err := service.GetAdtyOwnerCustomer(site, id)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusNotFound, err)
		return
	}
	result := gin.H{"customer": customer}
	if tokens, err := service.ListAdtyOwnerCustomerTokens(site, id); err == nil {
		result["tokens"] = tokens
	}
	// 客户单独定价和它的版本号（保存时带 expected_version）。
	var prices []model.AdtySitePrice
	if err := model.DB.Where("site_id = ? AND user_id = ?", site.Id, id).Order("id").Find(&prices).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var version int64
	for _, price := range prices {
		version = max(version, price.Version)
	}
	result["prices"], result["pricing_version"] = prices, version
	common.ApiSuccess(c, result)
}

func adtyOwnerHiddenOrError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAdtySiteHidden):
		common.ApiErrorStatus(c, http.StatusForbidden, err)
	default:
		common.ApiErrorStatus(c, http.StatusNotFound, err)
	}
}

// GetAdtyOwnerCustomerLogs 是客户的请求记录：GET /api/adty/site/customers/:id/logs（request_logs 打开时）。
func GetAdtyOwnerCustomerLogs(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	pageInfo, offset, limit := adtyPage(c)
	items, total, err := service.ListAdtyOwnerCustomerLogs(site, id, offset, limit)
	if err != nil {
		adtyOwnerHiddenOrError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GetAdtyOwnerCustomerTopUps 是客户的充值：GET /api/adty/site/customers/:id/topups（customer_topups 打开时）。
func GetAdtyOwnerCustomerTopUps(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	pageInfo, offset, limit := adtyPage(c)
	items, total, err := service.ListAdtyOwnerCustomerTopUps(site, id, offset, limit)
	if err != nil {
		adtyOwnerHiddenOrError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// PostAdtyOwnerTrial 送试用额度：POST /api/adty/site/customers/:id/trial {amount_usd, idempotency_key}，要二次验证。
func PostAdtyOwnerTrial(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	var body struct {
		AmountUSD      string `json:"amount_usd"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(body.AmountUSD))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionTrial, id) {
		return
	}
	grant, err := service.GrantAdtySiteTrial(site.Id, id, amount, body.IdempotencyKey)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "adty.site.trial", map[string]any{"site_id": site.Id, "customer_id": id, "amount_usd": grant.AmountUSD.String()})
	common.ApiSuccess(c, grant)
}

// PostAdtyOwnerReport 举报客户：POST /api/adty/site/report {user_id, reason}。
func PostAdtyOwnerReport(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		UserId int    `json:"user_id"`
		Reason string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.UserId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	report, err := service.ReportAdtySiteCustomer(site, c.GetInt("id"), body.UserId, body.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": report.Id, "status": report.Status})
}

// AdtyOwnerExportMaxRows 是一次导出最多的行数。超过就拒绝，不返回缺了较新记录的 CSV。
var AdtyOwnerExportMaxRows = 100000

func adtyOwnerExportOverLimit(c *gin.Context, total int64) bool {
	if total <= int64(AdtyOwnerExportMaxRows) {
		return false
	}
	common.ApiErrorT(c, "This export has more than {{limit}} rows", map[string]any{"limit": AdtyOwnerExportMaxRows})
	return true
}

// GetAdtyOwnerExport 导出：GET /api/adty/site/export?kind=earnings|stats|customers&from&to，CSV。
// 只能导出收益流水、经营报表、按客户汇总；邮箱一律打码（不看 customer_email 设置）。
func GetAdtyOwnerExport(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var b strings.Builder
	switch c.Query("kind") {
	case "earnings":
		var total int64
		earnings := model.DB.Model(&model.AdtySiteEarningDaily{}).Where("site_id = ?", site.Id)
		if err := earnings.Count(&total).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if adtyOwnerExportOverLimit(c, total) {
			return
		}
		var rows []model.AdtySiteEarningDaily
		if err := model.DB.Where("site_id = ?", site.Id).Order("day, id").Find(&rows).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		b.WriteString("day,user_id,kind,reason,revenue_usd,earning_usd,status\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "%s,%d,%s,%s,%s,%s,%s\n", row.Day, row.UserId, row.Kind, row.Reason, row.Revenue.StringFixed(2), row.Earning.StringFixed(2), row.Status)
		}
	case "stats":
		stats, err := service.GetAdtySiteStats(site.Id, c.Query("from"), c.Query("to"))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		b.WriteString("day,new_customers,paid_customers,active_customers,topup_usd,revenue_usd,earning_usd,bonus_usd,refund_usd\n")
		for _, day := range stats.Series {
			fmt.Fprintf(&b, "%s,%d,%d,%d,%s,%s,%s,%s,%s\n", day.Day, day.NewCustomers, day.PaidCustomers, day.ActiveCustomer,
				day.TopUpUSD.StringFixed(2), day.RevenueUSD.StringFixed(2), day.EarningUSD.StringFixed(2), day.BonusUSD.StringFixed(2), day.RefundUSD.StringFixed(2))
		}
	case "customers":
		var total int64
		if err := model.DB.Model(&model.User{}).Where("site_id = ?", site.Id).Count(&total).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if adtyOwnerExportOverLimit(c, total) {
			return
		}
		var users []model.User
		if err := model.DB.Select("id", "username", "email", "created_at").Where("site_id = ?", site.Id).Order("id").Find(&users).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		var sums []struct {
			UserId  int
			Revenue decimal.Decimal
			Earning decimal.Decimal
		}
		if err := model.DB.Model(&model.AdtySiteEarningDaily{}).Select("user_id, COALESCE(SUM(revenue),0) AS revenue, COALESCE(SUM(earning),0) AS earning").
			Where("site_id = ? AND status <> ?", site.Id, model.AdtySiteEarningVoid).Group("user_id").Scan(&sums).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		byUser := map[int]int{}
		for i, sum := range sums {
			byUser[sum.UserId] = i
		}
		b.WriteString("user_id,username,email,created_at,revenue_usd,earning_usd\n")
		for _, user := range users {
			revenue, earning := decimal.Zero, decimal.Zero
			if i, ok := byUser[user.Id]; ok {
				revenue, earning = sums[i].Revenue, sums[i].Earning
			}
			fmt.Fprintf(&b, "%d,%s,%s,%d,%s,%s\n", user.Id, adtyCSVField(user.Username), adtyCSVField(service.AdtyMaskEmail(user.Email)), user.CreatedAt,
				revenue.StringFixed(2), earning.StringFixed(2))
		}
	default:
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="site-%d-%s.csv"`, site.Id, c.Query("kind")))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(b.String()))
}

// adtyCSVField 给 CSV 字段加引号，并防止表格软件把 =、+、-、@ 开头的内容当公式执行。
func adtyCSVField(value string) string {
	if value != "" && strings.ContainsAny(value[:1], "=+-@\t\r") {
		value = "'" + value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// PutAdtySitePricingWithProof、PutAdtySiteCustomerPricingWithProof、RemoveAdtySiteDomainWithProof
// 在 11.4、11.8 的接口前面加一层二次验证（router/adty-router.go 注册时用这几个）。

// PutAdtySitePricingWithProof：全站定价，target = 0。
func PutAdtySitePricingWithProof(c *gin.Context) {
	if _, ok := adtyOwnerSiteOr403(c); !ok {
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionPricing, 0) {
		return
	}
	PutAdtySitePricing(c)
}

// PutAdtySiteCustomerPricingWithProof：单个客户的定价，target = 客户 id。
func PutAdtySiteCustomerPricingWithProof(c *gin.Context) {
	if _, ok := adtyOwnerSiteOr403(c); !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionPricing, id) {
		return
	}
	PutAdtySiteCustomerPricing(c)
}

// RemoveAdtySiteSenderDomainWithProof：删除发件域名，target = 站点 id（一个站最多一个发件域名）。
func RemoveAdtySiteSenderDomainWithProof(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionSender, site.Id) {
		return
	}
	RemoveAdtySiteSenderDomain(c)
}

// RemoveAdtySiteDomainWithProof：删除自定义域名，target = 域名 id。先过开站协议，和删除发件域名一样。
func RemoveAdtySiteDomainWithProof(c *gin.Context) {
	if _, ok := adtyOwnerSiteOr403(c); !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionDomain, id) {
		return
	}
	RemoveAdtySiteDomain(c)
}

// UploadAdtySiteLogo 上传 Logo：POST /api/adty/site/logo（multipart，字段 file）。返回地址，站长再用 PUT /self
// 把它写进 brand.logo_url。只收 PNG、JPEG、WebP，不收 SVG，最大 200 KB。
func UploadAdtySiteLogo(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	if site.Status == model.AdtySiteStatusClosing {
		common.ApiError(c, service.ErrAdtySiteClosing)
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorT(c, "Choose an image to upload")
		return
	}
	if header.Size > service.AdtySiteLogoMaxBytes {
		common.ApiError(c, service.ErrAdtySiteLogoTooLarge)
		return
	}
	file, err := header.Open()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, service.AdtySiteLogoMaxBytes+1))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	url, err := service.SaveAdtySiteLogo(site.Id, data)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"url": url})
}

// GetAdtySiteAsset 返回站长上传的 Logo（公开，分站页面和邮件都要用）。SVG 加 CSP 禁止脚本，所有类型都 nosniff。
func GetAdtySiteAsset(c *gin.Context) {
	path, contentType, err := service.OpenAdtySiteAsset(c.Param("name"))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("Content-Type", contentType)
	c.File(path)
}
