package model

import "github.com/shopspring/decimal"

const (
	AdtySiteWithdrawPending   = "pending"
	AdtySiteWithdrawPaying    = "paying"
	AdtySiteWithdrawPaid      = "paid"
	AdtySiteWithdrawRejected  = "rejected"
	AdtySiteWithdrawCancelled = "cancelled"
)

// AdtySiteWithdrawal is one site-owner earning withdrawal. Amounts are USD.
// Rate is the snapshot taken when the request was accepted.
type AdtySiteWithdrawal struct {
	Id             int64           `json:"id"`
	SiteId         int             `json:"site_id" gorm:"index"`
	OwnerUserId    int             `json:"owner_user_id" gorm:"index"`
	AmountUSD      decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	FeeUSD         decimal.Decimal `json:"fee_usd" gorm:"type:decimal(20,8);not null"`
	NetUSD         decimal.Decimal `json:"net_usd" gorm:"type:decimal(20,8);not null"`
	Method         string          `json:"method" gorm:"type:varchar(16)"`
	Network        string          `json:"network" gorm:"type:varchar(16)"`
	AccountNo      string          `json:"account_no" gorm:"type:varchar(128)"`
	AccountName    string          `json:"account_name" gorm:"type:varchar(128)"`
	Rate           string          `json:"rate" gorm:"type:varchar(32)"`
	PayoutAmount   string          `json:"payout_amount" gorm:"type:varchar(32)"`
	PayoutCurrency string          `json:"payout_currency" gorm:"type:varchar(8)"`
	Status         string          `json:"status" gorm:"type:varchar(16);index"`
	IdempotencyKey string          `json:"-" gorm:"uniqueIndex;type:varchar(128)"`
	TxID           string          `json:"tx_id" gorm:"type:varchar(128)"`
	ProofNote      string          `json:"proof_note" gorm:"type:varchar(255)"`
	ReviewerId     int             `json:"reviewer_id"`
	ReviewedAt     int64           `json:"reviewed_at"`
	PaidAt         int64           `json:"paid_at"`
	CreatedAt      int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteWithdrawal) TableName() string { return "adty_site_withdrawals" }
