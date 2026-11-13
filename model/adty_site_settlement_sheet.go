package model

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ErrAdtySettlementSheetImmutable rejects a direct rewrite or delete.
// The daily generator may refresh the same batch's check figures.
var ErrAdtySettlementSheetImmutable = errors.New("settlement sheet identity is immutable")

type adtySettlementRefresh struct{}

// WithAdtySettlementRefresh marks a generator refresh of check figures.
func WithAdtySettlementRefresh(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, adtySettlementRefresh{}, true)
}

func adtySettlementRefreshAllowed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	allowed, _ := ctx.Value(adtySettlementRefresh{}).(bool)
	return allowed
}

// AdtySiteSettlementSheet is a previous-business-day check sheet.
// It stores sums of recorded facts and does not credit a wallet.
type AdtySiteSettlementSheet struct {
	Id                        int64           `json:"id"`
	SiteId                    int             `json:"site_id" gorm:"uniqueIndex:idx_adty_site_settlement"`
	BusinessDay               string          `json:"business_day" gorm:"uniqueIndex:idx_adty_site_settlement;type:varchar(10)"`
	Timezone                  string          `json:"timezone" gorm:"type:varchar(64)"`
	DayStart                  int64           `json:"day_start"`
	DayEnd                    int64           `json:"day_end"`
	EventWatermark            int64           `json:"event_watermark"`
	IncludedCount             int64           `json:"included_count"`
	BatchId                   string          `json:"batch_id" gorm:"type:varchar(64);uniqueIndex"`
	TopupFaceUSD              decimal.Decimal `json:"topup_face_usd" gorm:"type:decimal(20,8);not null;default:0"`
	PaidUSD                   decimal.Decimal `json:"paid_usd" gorm:"type:decimal(20,8);not null;default:0"`
	PaidByCurrency            string          `json:"paid_by_currency" gorm:"type:text"`
	ConsumptionUSD            decimal.Decimal `json:"consumption_usd" gorm:"type:decimal(20,8);not null;default:0"`
	SupplyCostUSD             decimal.Decimal `json:"supply_cost_usd" gorm:"type:decimal(20,8);not null;default:0"`
	SpreadUSD                 decimal.Decimal `json:"spread_usd" gorm:"type:decimal(20,8);not null;default:0"`
	GiftReserveUSD            decimal.Decimal `json:"gift_reserve_usd" gorm:"type:decimal(20,8);not null;default:0"`
	GiftFundingUSD            decimal.Decimal `json:"gift_funding_usd" gorm:"type:decimal(20,8);not null;default:0"`
	GiftReleaseUSD            decimal.Decimal `json:"gift_release_usd" gorm:"type:decimal(20,8);not null;default:0"`
	TrialExpenseUSD           decimal.Decimal `json:"trial_expense_usd" gorm:"type:decimal(20,8);not null;default:0"`
	CommissionUSD             decimal.Decimal `json:"commission_usd" gorm:"type:decimal(20,8);not null;default:0"`
	PendingCoverageUSD        decimal.Decimal `json:"pending_coverage_usd" gorm:"type:decimal(20,8);not null;default:0"`
	MarginDeltaUSD            decimal.Decimal `json:"margin_delta_usd" gorm:"type:decimal(20,8);not null;default:0"`
	EarningsDeltaUSD          decimal.Decimal `json:"earnings_delta_usd" gorm:"type:decimal(20,8);not null;default:0"`
	EarningsAvailableDeltaUSD decimal.Decimal `json:"earnings_available_delta_usd" gorm:"type:decimal(20,8);not null;default:0"`
	EarningsFrozenDeltaUSD    decimal.Decimal `json:"earnings_frozen_delta_usd" gorm:"type:decimal(20,8);not null;default:0"`
	WithdrawalUSD             decimal.Decimal `json:"withdrawal_usd" gorm:"type:decimal(20,8);not null;default:0"`
	WithdrawalFeeUSD          decimal.Decimal `json:"withdrawal_fee_usd" gorm:"type:decimal(20,8);not null;default:0"`
	AdjustmentUSD             decimal.Decimal `json:"adjustment_usd" gorm:"type:decimal(20,8);not null;default:0"`
	AnomalyCount              int64           `json:"anomaly_count"`
	GeneratedAt               int64           `json:"generated_at"`
}

func (AdtySiteSettlementSheet) TableName() string { return "adty_site_settlement_sheets" }

func (sheet *AdtySiteSettlementSheet) BeforeUpdate(tx *gorm.DB) error {
	if !adtySettlementRefreshAllowed(tx.Statement.Context) {
		return ErrAdtySettlementSheetImmutable
	}
	for _, name := range []string{"BatchId", "SiteId", "BusinessDay", "DayStart", "DayEnd", "Timezone"} {
		if tx.Statement.Changed(name) {
			return ErrAdtySettlementSheetImmutable
		}
	}
	return nil
}

func (AdtySiteSettlementSheet) BeforeDelete(*gorm.DB) error {
	return ErrAdtySettlementSheetImmutable
}
