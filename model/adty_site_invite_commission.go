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
	Id                 int64           `json:"id"`
	SiteId             int             `json:"site_id" gorm:"index"`
	InviterId          int             `json:"inviter_id" gorm:"index"`
	InviteeId          int             `json:"invitee_id" gorm:"index"`
	TradeNo            string          `json:"trade_no" gorm:"type:varchar(64);uniqueIndex"`
	FaceUSD            decimal.Decimal `json:"face_usd" gorm:"type:decimal(20,8);not null"`
	Rate               decimal.Decimal `json:"rate" gorm:"type:decimal(10,6);not null"`
	AmountUSD          decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	CoveredUSD         decimal.Decimal `json:"covered_usd" gorm:"type:decimal(20,8);not null;default:0"`
	EarningsHoldUSD    decimal.Decimal `json:"earnings_hold_usd" gorm:"type:decimal(20,8);not null;default:0"`
	MarginHoldUSD      decimal.Decimal `json:"margin_hold_usd" gorm:"type:decimal(20,8);not null;default:0"`
	ConvertedClawedUSD decimal.Decimal `json:"converted_clawed_usd" gorm:"type:decimal(20,8);not null;default:0"`
	ClawedUSD          decimal.Decimal `json:"clawed_usd" gorm:"type:decimal(20,8);not null;default:0"`
	RecoveredUSD       decimal.Decimal `json:"recovered_usd" gorm:"type:decimal(20,8);not null;default:0"`
	ConvertedUSD       decimal.Decimal `json:"converted_usd" gorm:"type:decimal(20,8);not null;default:0"`
	WithdrawnUSD       decimal.Decimal `json:"withdrawn_usd" gorm:"type:decimal(20,8);not null;default:0"`
	FrozenUSD          decimal.Decimal `json:"frozen_usd" gorm:"type:decimal(20,8);not null;default:0"`
	RecoveryUSD        decimal.Decimal `json:"recovery_usd" gorm:"type:decimal(20,8);not null;default:0"`
	Status             string          `json:"status" gorm:"type:varchar(24);index"`
	ObservationDays    int64           `json:"observation_days"`
	CoveredAt          int64           `json:"covered_at"`
	AvailableAt        int64           `json:"available_at"`
	CreatedAt          int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteInviteCommission) TableName() string { return "adty_site_invite_commissions" }

// SiteInviteCommissionThisClaw is the claw for this refund ratio, minus what was already clawed.
// The ratio is refunded principal over the original principal, capped at 1.
func SiteInviteCommissionThisClaw(amount, already decimal.Decimal, refundQuota, faceQuota int64) decimal.Decimal {
	ratio := decimal.NewFromInt(1)
	if faceQuota > 0 {
		ratio = decimal.NewFromInt(refundQuota).Div(decimal.NewFromInt(faceQuota))
		if ratio.IsNegative() {
			ratio = decimal.Zero
		}
		if ratio.GreaterThan(decimal.NewFromInt(1)) {
			ratio = decimal.NewFromInt(1)
		}
	}
	target := amount.Mul(ratio).Round(8)
	if !ratio.LessThan(decimal.NewFromInt(1)) || target.GreaterThan(amount) {
		target = amount
	}
	thisClaw := target.Sub(already)
	if thisClaw.IsNegative() {
		return decimal.Zero
	}
	return thisClaw
}

// SiteInviteCommissionRefundPreview reports issued commission, withdrawn commission,
// and the clawback this refund ratio would record. It does not move money.
func SiteInviteCommissionRefundPreview(amount, withdrawn, already decimal.Decimal, refundQuota, faceQuota int64) (issued, withdrawnOut, expected string) {
	return amount.String(), withdrawn.String(), SiteInviteCommissionThisClaw(amount, already, refundQuota, faceQuota).String()
}

// AdtyCommissionRecovery is the liability for commission that was already withdrawn.
// It does not debit a customer balance.
type AdtyCommissionRecovery struct {
	Id             int64           `json:"id"`
	SiteId         int             `json:"site_id" gorm:"index"`
	CommissionId   int64           `json:"commission_id" gorm:"index"`
	TradeNo        string          `json:"trade_no" gorm:"type:varchar(64);index"`
	AmountUSD      decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	Kind           string          `json:"kind" gorm:"type:varchar(16)"`
	Status         string          `json:"status" gorm:"type:varchar(16)"`
	IdempotencyKey string          `json:"-" gorm:"uniqueIndex;type:varchar(128)"`
	CreatedAt      int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyCommissionRecovery) TableName() string { return "adty_commission_recoveries" }
