package model

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/shopspring/decimal"
)

const (
	AdtyVoucherPurposeMainUser     = "main_user"
	AdtyVoucherPurposeSiteMargin   = "site_margin"
	AdtyVoucherPurposeSiteCustomer = "site_customer"
	AdtyVoucherPurposeSiteOwner    = "site_owner"

	AdtyVoucherStatusPending   = "pending"
	AdtyVoucherStatusFrozen    = "frozen"
	AdtyVoucherStatusRedeemed  = "redeemed"
	AdtyVoucherStatusCancelled = "cancelled"

	AdtyVoucherSourcePlatform = "platform"
	AdtyVoucherSourceEarnings = "earnings"
	AdtyVoucherSourceMargin   = "margin"

	AdtyVoucherProvider   = "adty_voucher"
	AdtyLedgerTypeVoucher = "voucher"
)

// AdtyVoucher is the one recharge-quota code. The plaintext is not stored.
type AdtyVoucher struct {
	Id              int64           `json:"id"`
	CodeHash        string          `json:"-" gorm:"uniqueIndex;type:varchar(64);not null"`
	CodeTail        string          `json:"code_tail" gorm:"type:varchar(8);not null"`
	Purpose         string          `json:"purpose" gorm:"type:varchar(32);index;not null"`
	Status          string          `json:"status" gorm:"type:varchar(16);index;not null"`
	AmountUSD       decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null"`
	Quota           int64           `json:"quota"`
	SourceKind      string          `json:"source_kind" gorm:"type:varchar(16);not null"`
	SourceSiteId    int             `json:"source_site_id" gorm:"index"`
	TargetSiteId    int             `json:"target_site_id" gorm:"index"`
	IssuerId        int             `json:"issuer_id" gorm:"index"`
	RedeemerId      int             `json:"redeemer_id" gorm:"index"`
	InheritedByMain bool            `json:"inherited_by_main"`
	// TradeNo is empty until redeem. Uniqueness of non-empty values is enforced
	// by ux_adty_vouchers_trade_no, not by a full-column uniqueIndex, so pending
	// codes can share an empty trade number.
	TradeNo    string `json:"trade_no" gorm:"type:varchar(64);index"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
	RedeemedAt int64  `json:"redeemed_at"`
	UpdatedAt  int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyVoucher) TableName() string { return "adty_vouchers" }

// AdtyVoucherLiability is the Root-configured issuance cap for main-site codes.
// It is an obligation record, not a withdrawable wallet.
type AdtyVoucherLiability struct {
	Id        int64           `json:"id"`
	Scope     string          `json:"scope" gorm:"uniqueIndex;type:varchar(32);not null"`
	LimitUSD  decimal.Decimal `json:"limit_usd" gorm:"type:decimal(20,8);not null;default:0"`
	FrozenUSD decimal.Decimal `json:"frozen_usd" gorm:"type:decimal(20,8);not null;default:0"`
	SpentUSD  decimal.Decimal `json:"spent_usd" gorm:"type:decimal(20,8);not null;default:0"`
}

func (AdtyVoucherLiability) TableName() string { return "adty_voucher_liabilities" }

// AdtyVoucherAuthorization records whether a site may freeze margin to issue codes.
type AdtyVoucherAuthorization struct {
	SiteId      int   `json:"site_id" gorm:"uniqueIndex"`
	MarginIssue bool  `json:"margin_issue"`
	UpdatedAt   int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyVoucherAuthorization) TableName() string { return "adty_voucher_authorizations" }

// AdtyVoucherCloseGrant keeps the original site audience after the site closes.
type AdtyVoucherCloseGrant struct {
	Id     int64 `json:"id"`
	SiteId int   `json:"site_id" gorm:"uniqueIndex:idx_adty_voucher_close_grant"`
	UserId int   `json:"user_id" gorm:"uniqueIndex:idx_adty_voucher_close_grant"`
}

func (AdtyVoucherCloseGrant) TableName() string { return "adty_voucher_close_grants" }

func AdtyVoucherCodeHash(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
