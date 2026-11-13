package model

import (
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AdtyStaffBalanceIncrease = "increase"
	AdtyStaffBalanceDecrease = "decrease"
	AdtyStaffBalancePending  = "pending"
	AdtyStaffBalanceApproved = "approved"
	AdtyStaffBalanceRejected = "rejected"
)

// AdtyStaffBalanceRequest is an employee request to change a customer balance.
// It does not move money until a site owner or main-site admin approves it.
type AdtyStaffBalanceRequest struct {
	Id             int64           `json:"id"`
	SiteId         int             `json:"site_id" gorm:"index"`
	StaffUserId    int             `json:"staff_user_id" gorm:"index"`
	CustomerUserId int             `json:"customer_user_id" gorm:"index"`
	Direction      string          `json:"direction" gorm:"type:varchar(16)"`
	AmountUSD      decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty" gorm:"type:varchar(64);uniqueIndex"`
	Status         string          `json:"status" gorm:"type:varchar(16);index"`
	ReviewerId     int             `json:"reviewer_id"`
	ReviewerRole   string          `json:"reviewer_role" gorm:"type:varchar(16)"`
	CreatedAt      int64           `json:"created_at" gorm:"autoCreateTime"`
	ReviewedAt     int64           `json:"reviewed_at"`
}

func (AdtyStaffBalanceRequest) TableName() string { return "adty_staff_balance_requests" }

// AdtyPriceRevision records one price change. It is append-only history.
type AdtyPriceRevision struct {
	Id        int64  `json:"id"`
	SiteId    int    `json:"site_id" gorm:"index"`
	ActorId   int    `json:"actor_id"`
	Kind      string `json:"kind" gorm:"type:varchar(32);index"`
	Before    string `json:"before" gorm:"type:varchar(64)"`
	After     string `json:"after" gorm:"type:varchar(64)"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyPriceRevision) TableName() string { return "adty_price_revisions" }

func (AdtyPriceRevision) BeforeUpdate(*gorm.DB) error { return ErrAdtySiteWalletLedgerImmutable }

func (AdtyPriceRevision) BeforeDelete(*gorm.DB) error { return ErrAdtySiteWalletLedgerImmutable }

// AdtyNotificationHide is a user's manual removal of one inbox item.
type AdtyNotificationHide struct {
	Id        int64  `json:"id"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex:idx_adty_notice_hide,priority:1"`
	Key       string `json:"key" gorm:"uniqueIndex:idx_adty_notice_hide,priority:2;type:varchar(128)"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyNotificationHide) TableName() string { return "adty_notification_hides" }

// AdtyGlossaryTerm is one protected translation pair. Placeholders and amounts stay unchanged.
type AdtyGlossaryTerm struct {
	Id        int64  `json:"id"`
	Source    string `json:"source" gorm:"uniqueIndex:idx_adty_glossary,priority:1;type:varchar(128)"`
	Locale    string `json:"locale" gorm:"uniqueIndex:idx_adty_glossary,priority:2;type:varchar(16)"`
	Target    string `json:"target" gorm:"type:varchar(128)"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyGlossaryTerm) TableName() string { return "adty_glossary_terms" }

// AdtySiteReferralRate is the one-level invite rate for a site. It is a decimal fraction.
type AdtySiteReferralRate struct {
	SiteId  int             `json:"site_id" gorm:"primaryKey"`
	Rate    decimal.Decimal `json:"rate" gorm:"type:decimal(10,6);not null"`
	Enabled bool            `json:"enabled"`
}

func (AdtySiteReferralRate) TableName() string { return "adty_site_referral_rates" }

// AdtySiteInvite is one direct site invitation. Invitees cannot be bound twice.
type AdtySiteInvite struct {
	Id        int64           `json:"id"`
	SiteId    int             `json:"site_id" gorm:"index"`
	InviterId int             `json:"inviter_id" gorm:"index"`
	InviteeId int             `json:"invitee_id" gorm:"uniqueIndex"`
	Rate      decimal.Decimal `json:"rate" gorm:"type:decimal(10,6);not null"`
	IpHash    string          `json:"-" gorm:"type:varchar(64);index"`
	CreatedAt int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteInvite) TableName() string { return "adty_site_invites" }

// AdtyAnomalyDisposition records a review of a fund anomaly.
type AdtyAnomalyDisposition struct {
	Id         int64  `json:"id"`
	Kind       string `json:"kind" gorm:"type:varchar(32);index"`
	Ref        string `json:"ref" gorm:"type:varchar(128);index"`
	Status     string `json:"status" gorm:"type:varchar(16)"`
	Note       string `json:"note" gorm:"type:varchar(255)"`
	OperatorId int    `json:"operator_id"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyAnomalyDisposition) TableName() string { return "adty_anomaly_dispositions" }

// AdtyEmailChange is a pending login-email change. It is not applied until confirmed.
type AdtyEmailChange struct {
	UserId   int    `json:"user_id" gorm:"primaryKey"`
	NewEmail string `json:"new_email" gorm:"type:varchar(128)"`
	Status   string `json:"status" gorm:"type:varchar(16)"`
}

func (AdtyEmailChange) TableName() string { return "adty_email_changes" }

// AdtyAccountClosure is a closure request waiting for email confirmation.
type AdtyAccountClosure struct {
	UserId int    `json:"user_id" gorm:"primaryKey"`
	Status string `json:"status" gorm:"type:varchar(24)"`
}

func (AdtyAccountClosure) TableName() string { return "adty_account_closures" }

// AdtyNoticeTemplate is a fixed bilingual system notice.
type AdtyNoticeTemplate struct {
	Key string `json:"key" gorm:"column:notice_key;primaryKey;type:varchar(64)"`
	Zh  string `json:"zh" gorm:"type:varchar(255)"`
	En  string `json:"en" gorm:"type:varchar(255)"`
}

func (AdtyNoticeTemplate) TableName() string { return "adty_notice_templates" }

// AdtyKeyLimit is the per-minute cap for one API key.
type AdtyKeyLimit struct {
	TokenId   int `json:"token_id" gorm:"primaryKey"`
	PerMinute int `json:"per_minute"`
}

func (AdtyKeyLimit) TableName() string { return "adty_key_limits" }

// AdtyModelAlias maps a public name to an upstream model and context window.
type AdtyModelAlias struct {
	Alias   string `json:"alias" gorm:"primaryKey;type:varchar(80)"`
	Target  string `json:"target" gorm:"type:varchar(80)"`
	Context int    `json:"context"`
	Enabled bool   `json:"enabled"`
}

func (AdtyModelAlias) TableName() string { return "adty_model_aliases" }

// AdtyGroupConfig is one token group, its ratio, and the models it may call.
type AdtyGroupConfig struct {
	Name   string          `json:"name" gorm:"primaryKey;type:varchar(64)"`
	SiteId int             `json:"site_id" gorm:"primaryKey"`
	Ratio  decimal.Decimal `json:"ratio" gorm:"type:decimal(10,4);not null"`
	Models string          `json:"models" gorm:"type:text"`
}

func (AdtyGroupConfig) TableName() string { return "adty_group_configs" }
