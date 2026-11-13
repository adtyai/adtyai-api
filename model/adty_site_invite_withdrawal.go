package model

import "github.com/shopspring/decimal"

const (
	AdtySiteInviteWithdrawPending  = "pending"
	AdtySiteInviteWithdrawApproved = "approved"
	AdtySiteInviteWithdrawRejected = "rejected"
	AdtySiteInviteWithdrawCanceled = "canceled"
	AdtySiteInviteWithdrawPaid     = "paid"

	AdtySiteInviteWithdrawItemFrozen   = "frozen"
	AdtySiteInviteWithdrawItemReleased = "released"
	AdtySiteInviteWithdrawItemPaid     = "paid"
)

// AdtySiteInviteWithdrawal is one site-customer invite-commission withdrawal.
// It is not a site-owner earnings withdrawal and not a main-site referral withdrawal.
type AdtySiteInviteWithdrawal struct {
	Id             int64           `json:"id"`
	SiteId         int             `json:"site_id" gorm:"index"`
	UserId         int             `json:"user_id" gorm:"index"`
	AmountUSD      decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	FeeUSD         decimal.Decimal `json:"fee_usd" gorm:"type:decimal(20,8);not null;default:0"`
	NetUSD         decimal.Decimal `json:"net_usd" gorm:"type:decimal(20,8);not null"`
	Method         string          `json:"method" gorm:"type:varchar(16)"`
	Network        string          `json:"network" gorm:"type:varchar(16)"`
	AccountNo      string          `json:"account_no" gorm:"type:varchar(128)"`
	AccountName    string          `json:"account_name" gorm:"type:varchar(128)"`
	QRImageURL     string          `json:"qr_image_url" gorm:"type:text"`
	Rate           string          `json:"rate" gorm:"type:varchar(32)"`
	PayoutAmount   string          `json:"payout_amount" gorm:"type:varchar(32)"`
	PayoutCurrency string          `json:"payout_currency" gorm:"type:varchar(8)"`
	Status         string          `json:"status" gorm:"type:varchar(16);index"`
	IdempotencyKey string          `json:"-" gorm:"uniqueIndex;type:varchar(128)"`
	ReviewedBy     int             `json:"reviewed_by"`
	ReviewedAt     int64           `json:"reviewed_at"`
	PaidAt         int64           `json:"paid_at"`
	TxID           string          `json:"tx_id" gorm:"type:varchar(128)"`
	ProofNote      string          `json:"proof_note" gorm:"type:varchar(255)"`
	RejectReason   string          `json:"reject_reason" gorm:"type:varchar(255)"`
	CreatedAt      int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteInviteWithdrawal) TableName() string { return "adty_site_invite_withdrawals" }

// AdtySiteInviteWithdrawalItem ties a withdrawal to the commission rows it holds.
type AdtySiteInviteWithdrawalItem struct {
	Id           int64           `json:"id"`
	WithdrawalId int64           `json:"withdrawal_id" gorm:"index"`
	CommissionId int64           `json:"commission_id" gorm:"index"`
	AmountUSD    decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	Status       string          `json:"status" gorm:"type:varchar(16)"`
}

func (AdtySiteInviteWithdrawalItem) TableName() string { return "adty_site_invite_withdrawal_items" }
