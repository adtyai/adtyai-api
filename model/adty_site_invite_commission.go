package model

import "github.com/shopspring/decimal"

const (
	AdtySiteCommissionPendingCoverage = "pending_coverage"
	AdtySiteCommissionCovered         = "covered"
	AdtySiteCommissionAvailable       = "available"
	AdtySiteCommissionClawedBack      = "clawed_back"
)

// AdtySiteInviteCommission is one site invite commission for one top-up.
// It is not withdrawable until coverage and the Root observation period finish.
type AdtySiteInviteCommission struct {
	Id           int64           `json:"id"`
	SiteId       int             `json:"site_id" gorm:"index"`
	InviterId    int             `json:"inviter_id" gorm:"index"`
	InviteeId    int             `json:"invitee_id" gorm:"index"`
	TradeNo      string          `json:"trade_no" gorm:"type:varchar(64);uniqueIndex"`
	FaceUSD      decimal.Decimal `json:"face_usd" gorm:"type:decimal(20,8);not null"`
	Rate         decimal.Decimal `json:"rate" gorm:"type:decimal(10,6);not null"`
	AmountUSD    decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	CoveredUSD   decimal.Decimal `json:"covered_usd" gorm:"type:decimal(20,8);not null;default:0"`
	ClawedUSD    decimal.Decimal `json:"clawed_usd" gorm:"type:decimal(20,8);not null;default:0"`
	RecoveredUSD decimal.Decimal `json:"recovered_usd" gorm:"type:decimal(20,8);not null;default:0"`
	ConvertedUSD decimal.Decimal `json:"converted_usd" gorm:"type:decimal(20,8);not null;default:0"`
	RecoveryUSD  decimal.Decimal `json:"recovery_usd" gorm:"type:decimal(20,8);not null;default:0"`
	Status       string          `json:"status" gorm:"type:varchar(24);index"`
	CoveredAt    int64           `json:"covered_at"`
	AvailableAt  int64           `json:"available_at"`
	CreatedAt    int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteInviteCommission) TableName() string { return "adty_site_invite_commissions" }
