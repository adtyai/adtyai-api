package model

import (
	"database/sql/driver"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 营销群发（任务 12.2）。一次群发按「站点 × 语言」拆成几次 Resend broadcast，每次一行 AdtyMarketingCampaignSend；
// 实际发给了谁记在 AdtyMarketingCampaignRecipient，退订统计和审计都用它。

// 群发状态。draft 和模板可以改；preparing 是正在建 segment、加联系人；scheduled、sent 已经交给 Resend。
const (
	AdtyCampaignStatusDraft     = "draft"
	AdtyCampaignStatusPreparing = "preparing"
	AdtyCampaignStatusScheduled = "scheduled"
	AdtyCampaignStatusSent      = "sent"
	AdtyCampaignStatusCanceled  = "canceled"
	AdtyCampaignStatusFailed    = "failed"
)

// AdtyMarketingAudience 是选收件人的条件。时间是秒级时间戳，0 表示不限；余额是美元字符串，空表示不限。
type AdtyMarketingAudience struct {
	Langs                []string `json:"langs"`
	RegisteredAfter      int64    `json:"registered_after"`
	RegisteredBefore     int64    `json:"registered_before"`
	ActiveAfter          int64    `json:"active_after"` // 最后活跃按 users.last_login_at
	ActiveBefore         int64    `json:"active_before"`
	ToppedUp             *bool    `json:"topped_up"` // nil 不限；true 充过值；false 没充过
	BalanceMinUSD        string   `json:"balance_min_usd"`
	BalanceMaxUSD        string   `json:"balance_max_usd"`
	Groups               []string `json:"groups"`
	IncludeSiteCustomers bool     `json:"include_site_customers"`
	SiteIds              []int    `json:"site_ids"` // 只在含分站客户时生效；0 是主站；空表示所有站点
}

func (a AdtyMarketingAudience) Value() (driver.Value, error) {
	raw, err := common.Marshal(a)
	return string(raw), err
}

func (a *AdtyMarketingAudience) Scan(value any) error {
	*a = AdtyMarketingAudience{}
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		if len(typed) == 0 {
			return nil
		}
		return common.Unmarshal(typed, a)
	case string:
		if typed == "" {
			return nil
		}
		return common.UnmarshalJsonStr(typed, a)
	}
	return nil
}

// AdtyMarketingCampaign 是一次群发（或模板）。Subjects、Bodies 按内容语言存，没写的语言用英文版。
type AdtyMarketingCampaign struct {
	Id          int                   `json:"id"`
	Name        string                `json:"name" gorm:"type:varchar(128);not null"`
	IsTemplate  bool                  `json:"is_template" gorm:"not null;index"`
	Status      string                `json:"status" gorm:"type:varchar(16);not null;index"`
	Subjects    AdtyLangText          `json:"subjects" gorm:"type:text;not null"`
	Bodies      AdtyLangText          `json:"bodies" gorm:"type:text;not null"` // HTML
	Audience    AdtyMarketingAudience `json:"audience" gorm:"type:text;not null"`
	ScheduledAt int64                 `json:"scheduled_at" gorm:"not null"` // 0 立即发
	Recipients  int                   `json:"recipients" gorm:"not null"`
	Error       string                `json:"error" gorm:"type:varchar(255)"`
	CreatedBy   int                   `json:"created_by" gorm:"not null"`
	SentBy      int                   `json:"sent_by" gorm:"not null"`
	CreatedAt   int64                 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64                 `json:"updated_at" gorm:"autoUpdateTime"`
	SentAt      int64                 `json:"sent_at" gorm:"not null"`
}

func (AdtyMarketingCampaign) TableName() string { return "adty_marketing_campaigns" }

// AdtyMarketingCampaignSend 是群发里的一次 broadcast（一个站点的一种语言）。
type AdtyMarketingCampaignSend struct {
	Id          int    `json:"id"`
	CampaignId  int    `json:"campaign_id" gorm:"index;not null"`
	SiteId      int    `json:"site_id" gorm:"not null"`
	Lang        string `json:"lang" gorm:"type:varchar(16);not null"`
	SegmentId   string `json:"segment_id" gorm:"type:varchar(64)"`
	BroadcastId string `json:"broadcast_id" gorm:"type:varchar(64);index"`
	Recipients  int    `json:"recipients" gorm:"not null"`
	Status      string `json:"status" gorm:"type:varchar(16);not null"`
	Error       string `json:"error" gorm:"type:varchar(255)"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyMarketingCampaignSend) TableName() string { return "adty_marketing_campaign_sends" }

// AdtyMarketingCampaignRecipient 是群发实际加进 segment 的人。
type AdtyMarketingCampaignRecipient struct {
	CampaignId int    `json:"campaign_id" gorm:"primaryKey;autoIncrement:false"`
	UserId     int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	SendId     int    `json:"send_id" gorm:"index;not null"`
	Email      string `json:"email" gorm:"type:varchar(255);not null"`
}

func (AdtyMarketingCampaignRecipient) TableName() string { return "adty_marketing_campaign_recipients" }

func init() {
	adtyMarketingTables = append(adtyMarketingTables, &AdtyMarketingCampaign{}, &AdtyMarketingCampaignSend{}, &AdtyMarketingCampaignRecipient{})
}

// AdtyMarketingAudienceRow 是一个收件人。
type AdtyMarketingAudienceRow struct {
	UserId          int
	Email           string
	Lang            string
	SiteId          int
	ResendContactId string
	Synced          bool
}

// adtyMarketingUSDToQuota 把美元换成额度单位（向下取整）。
func adtyMarketingUSDToQuota(raw string) (int64, bool) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return value.Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Floor().IntPart(), true
}

// AdtyMarketingAudienceQuery 返回符合条件、可以发的收件人查询（联系人表 JOIN 用户表）。
// 只含同意、没退订、没退信、没投诉、账号启用的人；不含分站客户时只取主站（site_id = 0）。
func AdtyMarketingAudienceQuery(a AdtyMarketingAudience) *gorm.DB {
	query := DB.Table("adty_marketing_contacts").
		Joins("JOIN users ON users.id = adty_marketing_contacts.user_id AND users.deleted_at IS NULL").
		Where("users.status = ?", common.UserStatusEnabled).
		Scopes(AdtyMarketingSendableScope)
	if len(a.Langs) > 0 {
		query = query.Where("adty_marketing_contacts.lang IN ?", a.Langs)
	}
	if a.RegisteredAfter > 0 {
		query = query.Where("users.created_at >= ?", a.RegisteredAfter)
	}
	if a.RegisteredBefore > 0 {
		query = query.Where("users.created_at < ?", a.RegisteredBefore)
	}
	if a.ActiveAfter > 0 {
		query = query.Where("users.last_login_at >= ?", a.ActiveAfter)
	}
	if a.ActiveBefore > 0 {
		query = query.Where("users.last_login_at < ?", a.ActiveBefore)
	}
	if a.ToppedUp != nil {
		paid := "EXISTS (SELECT 1 FROM top_ups WHERE top_ups.user_id = users.id AND top_ups.status = ?)"
		if !*a.ToppedUp {
			paid = "NOT " + paid
		}
		query = query.Where(paid, common.TopUpStatusSuccess)
	}
	if quota, ok := adtyMarketingUSDToQuota(a.BalanceMinUSD); ok {
		query = query.Where("users.quota >= ?", quota)
	}
	if quota, ok := adtyMarketingUSDToQuota(a.BalanceMaxUSD); ok {
		query = query.Where("users.quota < ?", quota)
	}
	if len(a.Groups) > 0 {
		query = query.Where("users."+commonGroupCol+" IN ?", a.Groups)
	}
	if !a.IncludeSiteCustomers {
		query = query.Where("users.site_id = ?", 0)
	} else if len(a.SiteIds) > 0 {
		query = query.Where("users.site_id IN ?", a.SiteIds)
	}
	return query
}

// AdtyMarketingAudienceRows 列出收件人，按用户 id 排序。
func AdtyMarketingAudienceRows(a AdtyMarketingAudience) ([]AdtyMarketingAudienceRow, error) {
	var rows []AdtyMarketingAudienceRow
	err := AdtyMarketingAudienceQuery(a).
		Select("adty_marketing_contacts.user_id, adty_marketing_contacts.email, adty_marketing_contacts.lang, users.site_id, " +
			"adty_marketing_contacts.resend_contact_id, adty_marketing_contacts.updated_at <= adty_marketing_contacts.synced_at AS synced").
		Order("adty_marketing_contacts.user_id").Scan(&rows).Error
	return rows, err
}

// AdtyMarketingAudienceCounts 按语言数收件人。
func AdtyMarketingAudienceCounts(a AdtyMarketingAudience) (map[string]int64, error) {
	var rows []struct {
		Lang  string
		Count int64
	}
	if err := AdtyMarketingAudienceQuery(a).Select("adty_marketing_contacts.lang AS lang, COUNT(*) AS count").
		Group("adty_marketing_contacts.lang").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.Lang] += row.Count
	}
	return out, nil
}
