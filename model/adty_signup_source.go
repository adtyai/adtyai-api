package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// AdtySignupSource 是一个用户注册时的来源（任务 12.5），一个用户一行，注册时写一次。
// 没有来源信息的注册也写一行（各字段为空，统计里算「直接访问」）。
type AdtySignupSource struct {
	UserId        int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	InviterUserId int    `json:"inviter_user_id" gorm:"index;not null"` // P6 推广人，0 表示没有
	SiteId        int    `json:"site_id" gorm:"index;not null"`         // P11 站点，0 是主站
	UtmSource     string `json:"utm_source" gorm:"type:varchar(64);index;not null"`
	UtmMedium     string `json:"utm_medium" gorm:"type:varchar(64);not null"`
	UtmCampaign   string `json:"utm_campaign" gorm:"type:varchar(64);not null"`
	LandingPath   string `json:"landing_path" gorm:"type:varchar(255);not null"`
	ReferrerHost  string `json:"referrer_host" gorm:"type:varchar(255);index;not null"`
	CreatedAt     int64  `json:"created_at" gorm:"index;not null"`
}

func (AdtySignupSource) TableName() string { return "adty_signup_sources" }

func init() {
	adtyMarketingTables = append(adtyMarketingTables, &AdtySignupSource{})
}

// AdtySignupSourceDimensions 是能按哪一列统计，键是接口参数，值是列名（白名单，拼进 SQL）。
var AdtySignupSourceDimensions = map[string]string{
	"utm_source":    "adty_signup_sources.utm_source",
	"utm_medium":    "adty_signup_sources.utm_medium",
	"utm_campaign":  "adty_signup_sources.utm_campaign",
	"landing_path":  "adty_signup_sources.landing_path",
	"referrer_host": "adty_signup_sources.referrer_host",
	"inviter":       "adty_signup_sources.inviter_user_id",
	"site":          "adty_signup_sources.site_id",
}

// AdtySignupSourceFilter 是统计条件：注册时间在 [From, To) 内（0 不限），SiteId 为 nil 时不限站点。
type AdtySignupSourceFilter struct {
	Dimension string
	From, To  int64
	SiteId    *int
	Limit     int
}

// AdtySignupSourceStat 是一个来源的统计：注册数、付过费的人数、充值额（美元面额）、消费额（美元）。
type AdtySignupSourceStat struct {
	Key        string          `json:"key"`
	Signups    int64           `json:"signups"`
	Paying     int64           `json:"paying"`
	TopUpUSD   decimal.Decimal `json:"topup_usd"`
	ConsumeUSD decimal.Decimal `json:"consume_usd"`
}

// AdtySignupSourceStats 按维度汇总这段时间注册的人。充值额和消费额是这些人到现在的累计。
// 充值额取 top_ups.amount（我们的订单里是美元面额，执行层决定 2），消费额是 users.used_quota 换算成美元。
func AdtySignupSourceStats(f AdtySignupSourceFilter) ([]AdtySignupSourceStat, error) {
	column, ok := AdtySignupSourceDimensions[f.Dimension]
	if !ok {
		column = AdtySignupSourceDimensions["utm_source"]
	}
	topups := DB.Table("top_ups").Select("user_id, SUM(amount) AS amount").
		Where("status = ?", common.TopUpStatusSuccess).Group("user_id")
	query := DB.Table("adty_signup_sources").
		Joins("JOIN users ON users.id = adty_signup_sources.user_id").
		Joins("LEFT JOIN (?) AS paid ON paid.user_id = adty_signup_sources.user_id", topups)
	if f.From > 0 {
		query = query.Where("adty_signup_sources.created_at >= ?", f.From)
	}
	if f.To > 0 {
		query = query.Where("adty_signup_sources.created_at < ?", f.To)
	}
	if f.SiteId != nil {
		query = query.Where("adty_signup_sources.site_id = ?", *f.SiteId)
	}
	var rows []struct {
		GroupKey  string
		Signups   int64
		Paying    int64
		TopUp     decimal.Decimal
		UsedQuota decimal.Decimal
	}
	err := query.Select("CAST(" + column + " AS VARCHAR(255)) AS group_key, COUNT(*) AS signups, " +
		"COUNT(paid.user_id) AS paying, COALESCE(SUM(paid.amount), 0) AS top_up, COALESCE(SUM(users.used_quota), 0) AS used_quota").
		Group(column).Order("signups DESC").Limit(max(f.Limit, 1)).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	perUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	out := make([]AdtySignupSourceStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, AdtySignupSourceStat{
			Key: row.GroupKey, Signups: row.Signups, Paying: row.Paying,
			TopUpUSD: row.TopUp.Round(2), ConsumeUSD: row.UsedQuota.Div(perUnit).Round(2),
		})
	}
	return out, nil
}
