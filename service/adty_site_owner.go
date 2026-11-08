package service

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 站长中心（任务 11.6）和站长能看到什么（任务 11.7）。
// 原则：站长对客户能看到的，不超过客户自己在控制台里看到的，再去掉个人信息；另外加上站长自己的经营数据。
// 每个查询都带 site_id = 本站，别的站和主站的数据一律看不到。

var (
	ErrAdtySiteNotOwner      = common.NewMessage("You do not have an open site")
	ErrAdtySiteLookupLimited = common.NewMessage("Too many lookups. Try again later")
	ErrAdtySiteHidden        = common.NewMessage("The platform does not show this information")
	ErrAdtySiteClosing       = common.NewMessage("This site is closing and can only be viewed")
)

// AdtyOwnerSite 取用户是站长的、对客户开放的站（active、paused、closing）；不是站长返回 ErrAdtySiteNotOwner。
func AdtyOwnerSite(userId int) (*model.AdtySite, error) {
	var sites []model.AdtySite
	if err := model.DB.Where("owner_user_id = ? AND status IN ?", userId,
		[]string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusClosing}).Limit(1).Find(&sites).Error; err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return nil, ErrAdtySiteNotOwner
	}
	return &sites[0], nil
}

// AdtyIsSiteOwner 说明用户现在有没有对客户开放的站（站长中心入口的显示条件）。分站功能关着时一律 false，不查库。
func AdtyIsSiteOwner(userId int) bool {
	if userId <= 0 || !resellerSetting().Enabled {
		return false
	}
	var count int64
	if err := model.DB.Model(&model.AdtySite{}).Where("owner_user_id = ? AND status IN ?", userId,
		[]string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusClosing}).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// AdtySiteVisibility 是这个站长能看到的范围：全局默认 adty_reseller_setting.visibility，站点的 Visibility 逐项覆盖（只有管理员能改）。
// 没设的项用 11.7 的初始值。
func AdtySiteVisibility(site *model.AdtySite) map[string]string {
	result := make(map[string]string, len(adty_setting.ResellerVisibilityKeys))
	for key, value := range adty_setting.ResellerVisibilityKeys {
		result[key] = value
	}
	for key, value := range resellerSetting().Visibility {
		if _, ok := result[key]; ok {
			result[key] = value
		}
	}
	if site != nil && strings.TrimSpace(site.Visibility) != "" {
		override := map[string]string{}
		if err := common.UnmarshalJsonStr(site.Visibility, &override); err == nil {
			for key, value := range override {
				if _, ok := result[key]; ok {
					result[key] = value
				}
			}
		}
	}
	return result
}

// adtyVisible 说明这一项是不是对站长打开。
func adtyVisible(visibility map[string]string, key string) bool {
	return visibility[key] == "on"
}

// AdtyMaskEmail 把邮箱打码成 a***@gmail.com：本地部分只留第一个字符；不是邮箱时只留第一个字符。
func AdtyMaskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	local, domain, ok := strings.Cut(email, "@")
	first := []rune(local)
	head := ""
	if len(first) > 0 {
		head = string(first[0])
	}
	if !ok {
		return head + "***"
	}
	return head + "***@" + domain
}

// adtyOwnerEmail follows customer_email: full only when that visibility is explicitly full.
// AdtyOwnerVisibleEmail is the email a site owner may see under the current visibility map.
func AdtyOwnerVisibleEmail(email string, visibility map[string]string) string {
	if visibility["customer_email"] == "full" {
		return email
	}
	return AdtyMaskEmail(email)
}

func adtyOwnerEmail(email string, visibility map[string]string) string {
	return AdtyOwnerVisibleEmail(email, visibility)
}

// AdtyOwnerCustomer 是站长看到的一个客户：没有 IP、设备、第三方登录、手机号、风控依据。
type AdtyOwnerCustomer struct {
	Id         int    `json:"id"`
	Username   string `json:"username"`
	Email      string `json:"email"`
	Status     int    `json:"status"`
	Group      string `json:"group"`
	CreatedAt  int64  `json:"created_at"`
	Balance    *int   `json:"balance,omitempty"`    // customer_balance 关着时不返回
	UsedQuota  *int   `json:"used_quota,omitempty"` // customer_usage 关着时不返回
	RiskFlag   string `json:"risk_flag,omitempty"`  // 只给类别（self_use | migration），不给判断依据
	Reviewed   bool   `json:"reviewed"`
	LastActive int64  `json:"last_active,omitempty"`
}

func adtyOwnerCustomerView(visibility map[string]string, user model.User, customer model.AdtySiteCustomer) (AdtyOwnerCustomer, error) {
	view := AdtyOwnerCustomer{
		Id: user.Id, Username: user.Username, Email: adtyOwnerEmail(user.Email, visibility), Status: user.Status,
		Group: user.Group, CreatedAt: user.CreatedAt, RiskFlag: customer.RiskFlag, Reviewed: customer.Reviewed,
	}
	if adtyVisible(visibility, "customer_balance") {
		balance := user.ToBaseUser().Quota
		view.Balance = &balance
	}
	if adtyVisible(visibility, "customer_usage") {
		used, err := adtyUsageSinceJoin(user.Id, customer.JoinedAt, user.UsedQuota)
		if err != nil {
			return view, err
		}
		view.UsedQuota = &used
	}
	return view, nil
}

// adtyUsageSinceJoin 只统计客户加入本站之后的用量。转站会把 JoinedAt 改成转站时间，
// users.used_quota 含转入前的累计，不能直接给新站长。
func adtyOwnerJoinedAt(userId int) (int64, error) {
	rows, err := adtySiteCustomerRows([]int{userId})
	if err != nil {
		return 0, err
	}
	return rows[userId].JoinedAt, nil
}

func adtyUsageSinceJoin(userId int, joinedAt int64, fallback int) (int, error) {
	if joinedAt <= 0 {
		return fallback, nil
	}
	if model.LOG_DB == nil {
		return 0, nil
	}
	var consume, refund int64
	base := model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND created_at >= ?", userId, joinedAt)
	if err := base.Where("type = ?", model.LogTypeConsume).Select("COALESCE(SUM(quota),0)").Scan(&consume).Error; err != nil {
		return 0, err
	}
	if err := model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND created_at >= ? AND type = ?", userId, joinedAt, model.LogTypeRefund).
		Select("COALESCE(SUM(quota),0)").Scan(&refund).Error; err != nil {
		return 0, err
	}
	used := consume - refund
	if used < 0 {
		return 0, nil
	}
	return int(used), nil
}

var adtyOwnerCustomerColumns = []string{"id", "username", "email", "status", "group", "created_at", "quota", "quota_pending", "used_quota"}

func adtySiteCustomerRows(userIds []int) (map[int]model.AdtySiteCustomer, error) {
	rows := map[int]model.AdtySiteCustomer{}
	if len(userIds) == 0 {
		return rows, nil
	}
	var customers []model.AdtySiteCustomer
	if err := model.DB.Where("user_id IN ?", userIds).Find(&customers).Error; err != nil {
		return nil, err
	}
	for _, customer := range customers {
		rows[customer.UserId] = customer
	}
	return rows, nil
}

// ListAdtyOwnerCustomers 按注册时间分页列出本站客户（不支持模糊搜索，11.7）。
func ListAdtyOwnerCustomers(site *model.AdtySite, offset, limit int) ([]AdtyOwnerCustomer, int64, error) {
	query := model.DB.Model(&model.User{}).Where("site_id = ?", site.Id)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []model.User
	if err := query.Select(adtyOwnerCustomerColumns).Order("id DESC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	rows, err := adtySiteCustomerRows(ids)
	if err != nil {
		return nil, 0, err
	}
	visibility := AdtySiteVisibility(site)
	items := make([]AdtyOwnerCustomer, 0, len(users))
	for _, user := range users {
		view, err := adtyOwnerCustomerView(visibility, user, rows[user.Id])
		if err != nil {
			return nil, 0, err
		}
		items = append(items, view)
	}
	return items, total, nil
}

// GetAdtyOwnerCustomer 取本站的一个客户；别的站和主站的用户当作不存在。
func GetAdtyOwnerCustomer(site *model.AdtySite, userId int) (*AdtyOwnerCustomer, error) {
	var users []model.User
	if err := model.DB.Select(adtyOwnerCustomerColumns).Where("id = ? AND site_id = ?", userId, site.Id).Limit(1).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, common.NewMessage("Customer not found")
	}
	rows, err := adtySiteCustomerRows([]int{userId})
	if err != nil {
		return nil, err
	}
	view, err := adtyOwnerCustomerView(AdtySiteVisibility(site), users[0], rows[userId])
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// adtyLookupNow 是查找限流用的时间，测试可以替换。
var adtyLookupNow = func() time.Time { return time.Now() }

// LookupAdtyOwnerCustomer 精确查找客户（11.7）：q 必须是完整的用户名或完整邮箱，完全匹配本站客户才返回。
// 每个站长每小时最多 customer_lookup_per_hour 次（查到查不到都算），每次都记审计。
func LookupAdtyOwnerCustomer(site *model.AdtySite, ownerId int, q string) (*AdtyOwnerCustomer, error) {
	q = strings.TrimSpace(q)
	if q == "" || len(q) > 255 {
		return nil, common.NewMessage("Enter the full username or email")
	}
	limit := resellerSetting().CustomerLookupPerHour
	now := adtyLookupNow().Unix()
	var users []model.User
	email := model.NormalizeEmail(q)
	if err := model.DB.Select(adtyOwnerCustomerColumns).Where("site_id = ? AND (username = ? OR email = ?)", site.Id, q, email).
		Order("id").Limit(1).Find(&users).Error; err != nil {
		return nil, err
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(site.Id)+0x4c4b500000000000).Error; err != nil {
				return err
			}
		}
		var used int64
		if err := tx.Model(&model.AdtySiteLookup{}).Where("site_id = ? AND created_at > ?", site.Id, now-3600).Count(&used).Error; err != nil {
			return err
		}
		if limit <= 0 || used >= int64(limit) {
			return ErrAdtySiteLookupLimited
		}
		return tx.Create(&model.AdtySiteLookup{SiteId: site.Id, OwnerId: ownerId, Query: truncateText(q, 255), Found: len(users) > 0, CreatedAt: now}).Error
	})
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, common.NewMessage("Customer not found")
	}
	rows, err := adtySiteCustomerRows([]int{users[0].Id})
	if err != nil {
		return nil, err
	}
	view, err := adtyOwnerCustomerView(AdtySiteVisibility(site), users[0], rows[users[0].Id])
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// AdtyOwnerLog 是站长看到的一条请求记录（request_logs 打开时）：走 7.1 的客户投影，
// 再去掉客户 IP、请求 ID 和 Other 里的明细。扣费是客户价（日志里的额度本来就是客户扣的）。
type AdtyOwnerLog struct {
	Id               int    `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	ModelName        string `json:"model_name"`
	TokenName        string `json:"token_name"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	UseTime          int    `json:"use_time"`
	IsStream         bool   `json:"is_stream"`
	Content          string `json:"content"`
	Other            string `json:"other"`
}

// ListAdtyOwnerCustomerLogs 列出本站一个客户的消费、错误和退款记录（11.7）。
func ListAdtyOwnerCustomerLogs(site *model.AdtySite, userId, offset, limit int) ([]AdtyOwnerLog, int64, error) {
	if !adtyVisible(AdtySiteVisibility(site), "request_logs") {
		return nil, 0, ErrAdtySiteHidden
	}
	if _, err := GetAdtyOwnerCustomer(site, userId); err != nil {
		return nil, 0, err
	}
	query := model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type IN ?", userId, []int{model.LogTypeConsume, model.LogTypeError, model.LogTypeRefund})
	since, err := adtyOwnerJoinedAt(userId)
	if err != nil {
		return nil, 0, err
	}
	if since > 0 {
		query = query.Where("created_at >= ?", since)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.Log
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	items := make([]AdtyOwnerLog, 0, len(logs))
	for i := range logs {
		model.AdtyProjectUserLog(&logs[i])
		log := logs[i]
		items = append(items, AdtyOwnerLog{
			Id: log.Id, CreatedAt: log.CreatedAt, Type: log.Type, ModelName: log.ModelName, TokenName: log.TokenName,
			Quota: log.Quota, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens,
			UseTime: log.UseTime, IsStream: log.IsStream, Content: log.Content, Other: log.Other,
		})
	}
	return items, total, nil
}

// AdtyOwnerTopUp 是站长看到的一笔充值（customer_topups 打开时）：不含支付平台订单号、付款账户、钱包地址。
type AdtyOwnerTopUp struct {
	TradeNo     string          `json:"trade_no"`
	FaceUSD     decimal.Decimal `json:"face_usd"`
	BonusUSD    decimal.Decimal `json:"bonus_usd"`
	CreditQuota int64           `json:"credit_quota"`
	Method      string          `json:"method"`
	Status      string          `json:"status"`
	CreatedAt   int64           `json:"created_at"`
}

// ListAdtyOwnerCustomerTopUps 列出本站一个客户的充值。
func ListAdtyOwnerCustomerTopUps(site *model.AdtySite, userId, offset, limit int) ([]AdtyOwnerTopUp, int64, error) {
	if !adtyVisible(AdtySiteVisibility(site), "customer_topups") {
		return nil, 0, ErrAdtySiteHidden
	}
	if _, err := GetAdtyOwnerCustomer(site, userId); err != nil {
		return nil, 0, err
	}
	query := model.DB.Model(&model.AdtyTopupOrder{}).Where("user_id = ? AND site_id = ?", userId, site.Id)
	since, err := adtyOwnerJoinedAt(userId)
	if err != nil {
		return nil, 0, err
	}
	if since > 0 {
		query = query.Where("created_at >= ?", since)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var orders []model.AdtyTopupOrder
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	statuses := map[string]string{}
	if len(orders) > 0 {
		tradeNos := make([]string, 0, len(orders))
		for _, order := range orders {
			tradeNos = append(tradeNos, order.TradeNo)
		}
		var topUps []model.TopUp
		if err := model.DB.Select("trade_no", "status").Where("trade_no IN ?", tradeNos).Find(&topUps).Error; err != nil {
			return nil, 0, err
		}
		for _, topUp := range topUps {
			statuses[topUp.TradeNo] = topUp.Status
		}
	}
	items := make([]AdtyOwnerTopUp, 0, len(orders))
	for _, order := range orders {
		items = append(items, AdtyOwnerTopUp{
			TradeNo: order.TradeNo, FaceUSD: order.FaceUSD, BonusUSD: order.BonusUSD, CreditQuota: order.CreditQuota,
			Method: order.Method, Status: statuses[order.TradeNo], CreatedAt: order.CreatedAt,
		})
	}
	return items, total, nil
}

// AdtyOwnerToken 是站长看到的一个客户密钥（customer_tokens 打开时）：看不到密钥本身。
type AdtyOwnerToken struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Status       int    `json:"status"`
	AccessedTime int64  `json:"accessed_time"`
	CreatedTime  int64  `json:"created_time"`
}

// ListAdtyOwnerCustomerTokens 列出本站一个客户的密钥。
func ListAdtyOwnerCustomerTokens(site *model.AdtySite, userId int) ([]AdtyOwnerToken, error) {
	if !adtyVisible(AdtySiteVisibility(site), "customer_tokens") {
		return nil, ErrAdtySiteHidden
	}
	if _, err := GetAdtyOwnerCustomer(site, userId); err != nil {
		return nil, err
	}
	joined, err := adtyOwnerJoinedAt(userId)
	if err != nil {
		return nil, err
	}
	var tokens []model.Token
	query := model.DB.Select("id", "name", "status", "accessed_time", "created_time").Where("user_id = ?", userId)
	if joined > 0 {
		query = query.Where("created_time = 0 OR created_time >= ?", joined)
	}
	if err := query.Order("id").Find(&tokens).Error; err != nil {
		return nil, err
	}
	items := make([]AdtyOwnerToken, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, AdtyOwnerToken{Id: token.Id, Name: token.Name, Status: token.Status, AccessedTime: token.AccessedTime, CreatedTime: token.CreatedTime})
	}
	return items, nil
}

// ReportAdtySiteCustomer 站长举报本站客户，进入主站分站管理的待处理（11.9）。
func ReportAdtySiteCustomer(site *model.AdtySite, ownerId, userId int, reason string) (*model.AdtySiteReport, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, common.NewMessage("Enter a reason")
	}
	if _, err := GetAdtyOwnerCustomer(site, userId); err != nil {
		return nil, err
	}
	report := &model.AdtySiteReport{SiteId: site.Id, UserId: userId, ReporterId: ownerId, Reason: truncateText(reason, 1000), Status: model.AdtySiteReportPending}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var current model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Select("id", "status").Where("id = ?", site.Id).First(&current).Error; err != nil {
			return err
		}
		if current.Status == model.AdtySiteStatusClosing {
			return ErrAdtySiteClosing
		}
		return tx.Create(report).Error
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// HandleAdtySiteReport 管理员处理举报（分站·审核）：resolved 或 dismissed，写审计。
func HandleAdtySiteReport(reportId, adminId int, status, note string) (*model.AdtySiteReport, error) {
	if status != model.AdtySiteReportResolved && status != model.AdtySiteReportDismissed {
		return nil, common.NewMessage("Invalid parameters")
	}
	var report model.AdtySiteReport
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", reportId).First(&report).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewMessage("Report not found")
			}
			return err
		}
		if report.Status != model.AdtySiteReportPending {
			return common.NewMessage("This report has already been handled")
		}
		now := time.Now().Unix()
		if err := tx.Model(&model.AdtySiteReport{}).Where("id = ?", report.Id).Updates(map[string]any{
			"status": status, "handled_by": adminId, "handle_note": truncateText(note, 1000), "handled_at": now,
		}).Error; err != nil {
			return err
		}
		report.Status, report.HandledBy, report.HandleNote, report.HandledAt = status, adminId, truncateText(note, 1000), now
		return recordSiteAuditTx(tx, report.SiteId, "report_"+status, adminId, map[string]any{"report_id": report.Id, "user_id": report.UserId}, nil, note)
	})
	if err != nil {
		return nil, err
	}
	return &report, nil
}
