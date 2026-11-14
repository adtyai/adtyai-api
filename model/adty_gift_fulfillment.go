package model

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AdtyGiftFulfillmentPending   = "pending"
	AdtyGiftFulfillmentFulfilled = "fulfilled"
	AdtyGiftFulfillmentCanceled  = "canceled"
)

// AdtyGiftFulfillment is the official A.8 待履约 record for a site-funded
// gift that was promised on a verified payment but is not yet spendable
// because the guarantee could not be re-acquired after cancel or expiry
// released the original reserve.
type AdtyGiftFulfillment struct {
	Id              int64           `json:"id"`
	TradeNo         string          `json:"trade_no" gorm:"uniqueIndex;type:varchar(64);not null"`
	UserId          int             `json:"user_id" gorm:"index"`
	SiteId          int             `json:"site_id" gorm:"index"`
	BonusUSD        decimal.Decimal `json:"bonus_usd" gorm:"type:decimal(20,8);not null;default:0"`
	BonusQuota      int64           `json:"bonus_quota" gorm:"not null;default:0"`
	OriginalSource  string          `json:"original_source" gorm:"type:varchar(32)"`
	OriginalEarning decimal.Decimal `json:"original_earning" gorm:"type:decimal(20,8);not null;default:0"`
	OriginalMargin  decimal.Decimal `json:"original_margin" gorm:"type:decimal(20,8);not null;default:0"`
	Status          string          `json:"status" gorm:"type:varchar(32);not null;index"`
	Reason          string          `json:"reason" gorm:"type:varchar(255)"`
	History         string          `json:"history" gorm:"type:text"`
	CreatedAt       int64           `json:"created_at"`
	UpdatedAt       int64           `json:"updated_at"`
}

func (AdtyGiftFulfillment) TableName() string { return "adty_gift_fulfillments" }

func (row *AdtyGiftFulfillment) BeforeCreate(tx *gorm.DB) error {
	now := time.Now().Unix()
	if row.CreatedAt == 0 {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	return nil
}

func (row *AdtyGiftFulfillment) BeforeUpdate(tx *gorm.DB) error {
	row.UpdatedAt = time.Now().Unix()
	return nil
}
