package model

import "github.com/shopspring/decimal"

// AdtySiteEarningLine is one site-owner earning fact. It keeps the model and
// billing group and never stores an upstream channel name.
type AdtySiteEarningLine struct {
	Id         int64           `json:"id"`
	SiteId     int             `json:"site_id" gorm:"index"`
	UserId     int             `json:"user_id" gorm:"index"`
	ModelName  string          `json:"model_name" gorm:"type:varchar(128)"`
	Group      string          `json:"group" gorm:"type:varchar(64)"`
	RequestId  string          `json:"request_id" gorm:"type:varchar(64)"`
	Quota      int64           `json:"quota"`
	Markup     decimal.Decimal `json:"markup" gorm:"type:decimal(10,4)"`
	CostFactor decimal.Decimal `json:"cost_factor" gorm:"type:decimal(10,6)"`
	Revenue    decimal.Decimal `json:"revenue_usd" gorm:"type:decimal(20,8)"`
	Platform   decimal.Decimal `json:"platform_usd" gorm:"type:decimal(20,8)"`
	Earning    decimal.Decimal `json:"earning_usd" gorm:"type:decimal(20,8)"`
	CreatedAt  int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteEarningLine) TableName() string { return "adty_site_earning_lines" }
