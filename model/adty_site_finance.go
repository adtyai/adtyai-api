package model

import (
	"errors"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AdtySiteWalletOperating = "operating"
	AdtySiteWalletEarnings  = "earnings"
	AdtySiteWalletMargin    = "margin"
)

var ErrAdtySiteWalletLedgerImmutable = errors.New("site wallet ledger is immutable")

// AdtySiteTier is a root-managed, data-driven site level definition.
// Monetary values are USD and remain configurable instead of being embedded
// in code so the same table can support future commercial policies.
type AdtySiteTier struct {
	Id                int64            `json:"id"`
	Code              string           `json:"code" gorm:"uniqueIndex;type:varchar(32)"`
	Name              string           `json:"name" gorm:"type:varchar(128)"`
	MinMonthlyUSD     decimal.Decimal  `json:"min_monthly_usd" gorm:"type:decimal(20,8);not null;default:0"` // retained for compatibility; auto evaluation ignores it
	MinEffectiveUsers int64            `json:"min_effective_users" gorm:"not null;default:0"`
	MinPrincipalUSD   decimal.Decimal  `json:"min_principal_usd" gorm:"type:decimal(20,8);not null;default:0"`
	MinConsumptionUSD decimal.Decimal  `json:"min_consumption_usd" gorm:"type:decimal(20,8);not null;default:0"`
	CostFactor        *decimal.Decimal `json:"cost_factor" gorm:"type:decimal(10,6)"`
	MaxMarkup         *decimal.Decimal `json:"max_markup" gorm:"type:decimal(10,4)"`
	DepositUSD        decimal.Decimal  `json:"deposit_usd" gorm:"type:decimal(20,8);not null;default:0"`
	Enabled           bool             `json:"enabled"`
	SortOrder         int              `json:"sort_order"`
	CreatedAt         int64            `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64            `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtySiteTier) TableName() string { return "adty_site_tiers" }

// AdtySiteTierEvaluation is one idempotent auto-evaluation of a site's lifetime grade.
// Re-running the same site and period must not repeat the grade write, margin mutation, or warning.
type AdtySiteTierEvaluation struct {
	Id               int64           `json:"id"`
	SiteId           int             `json:"site_id" gorm:"uniqueIndex:idx_adty_site_tier_eval;index"`
	PeriodId         string          `json:"period_id" gorm:"uniqueIndex:idx_adty_site_tier_eval;type:varchar(32)"`
	StatAt           int64           `json:"stat_at"`
	EffectiveUsers   int64           `json:"effective_users"`
	PrincipalUSD     decimal.Decimal `json:"principal_usd" gorm:"type:decimal(20,8);not null;default:0"`
	ConsumptionUSD   decimal.Decimal `json:"consumption_usd" gorm:"type:decimal(20,8);not null;default:0"`
	TierCode         string          `json:"tier_code" gorm:"type:varchar(32)"`
	PreviousTierCode string          `json:"previous_tier_code" gorm:"type:varchar(32)"`
	Warned           bool            `json:"warned"`
	CreatedAt        int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteTierEvaluation) TableName() string { return "adty_site_tier_evaluations" }

type AdtySiteWallet struct {
	Id              int64           `json:"id"`
	SiteId          int             `json:"site_id" gorm:"uniqueIndex:idx_adty_site_wallet;index"`
	Kind            string          `json:"kind" gorm:"uniqueIndex:idx_adty_site_wallet;type:varchar(16)"`
	Currency        string          `json:"currency" gorm:"type:varchar(16)"`
	Balance         decimal.Decimal `json:"balance" gorm:"type:decimal(28,8);not null;default:0"`
	FrozenUSD       decimal.Decimal `json:"frozen_usd" gorm:"type:decimal(28,8);not null;default:0"`
	RequiredBalance decimal.Decimal `json:"required_balance" gorm:"type:decimal(28,8);not null;default:0"`
	CycleTierUSD    decimal.Decimal `json:"cycle_tier_usd" gorm:"type:decimal(20,8);not null;default:0"`
	CycleUsedUSD    decimal.Decimal `json:"cycle_used_usd" gorm:"type:decimal(20,8);not null;default:0"`
	CreatedAt       int64           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64           `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtySiteWallet) TableName() string { return "adty_site_wallets" }

type AdtySiteWalletLedger struct {
	Id             int64           `json:"id"`
	SiteId         int             `json:"site_id" gorm:"index"`
	WalletId       int64           `json:"wallet_id" gorm:"index"`
	WalletKind     string          `json:"wallet_kind" gorm:"type:varchar(16);index"`
	Delta          decimal.Decimal `json:"delta" gorm:"type:decimal(28,8);not null"`
	HoldDelta      decimal.Decimal `json:"hold_delta" gorm:"type:decimal(28,8);not null;default:0"`
	BalanceBefore  decimal.Decimal `json:"balance_before" gorm:"type:decimal(28,8);not null"`
	BalanceAfter   decimal.Decimal `json:"balance_after" gorm:"type:decimal(28,8);not null"`
	Type           string          `json:"type" gorm:"type:varchar(32);index"`
	RefType        string          `json:"ref_type" gorm:"type:varchar(32)"`
	RefId          string          `json:"ref_id" gorm:"type:varchar(128);index"`
	Reason         string          `json:"reason" gorm:"type:text"`
	OperatorId     int             `json:"operator_id"`
	AllowOverdraft bool            `json:"allow_overdraft"`
	IdempotencyKey string          `json:"-" gorm:"uniqueIndex;type:varchar(128)"`
	CreatedAt      int64           `json:"created_at" gorm:"autoCreateTime;index"`
}

func (AdtySiteWalletLedger) TableName() string { return "adty_site_wallet_ledgers" }

func (AdtySiteWalletLedger) BeforeUpdate(*gorm.DB) error { return ErrAdtySiteWalletLedgerImmutable }

func (AdtySiteWalletLedger) BeforeDelete(*gorm.DB) error { return ErrAdtySiteWalletLedgerImmutable }
