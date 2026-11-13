package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Manual refund overall statuses. Occupancy uses the official S/H/U buckets:
// completed rows are counted in S (also stored on the order's RefundQuota);
// submitted/approved in-progress rows occupy H; unknown external payout occupies U;
// terminated rows release their occupancy.
const (
	AdtyManualRefundStatusSubmitted  = "submitted"
	AdtyManualRefundStatusApproved   = "approved"
	AdtyManualRefundStatusTerminated = "terminated"
	AdtyManualRefundStatusCompleted  = "completed"
)

const (
	AdtyManualRefundPayoutNone    = ""
	AdtyManualRefundPayoutUnknown = "unknown"
	AdtyManualRefundPayoutSuccess = "success"
	AdtyManualRefundPayoutFailed  = "failed"
)

const (
	AdtyManualRefundStepApply      = "apply"
	AdtyManualRefundStepApprove    = "approve"
	AdtyManualRefundStepReclaim    = "reclaim"
	AdtyManualRefundStepBonus      = "bonus"
	AdtyManualRefundStepCommission = "commission"
	AdtyManualRefundStepPayout     = "payout"
	AdtyManualRefundStepComplete   = "complete"
	AdtyManualRefundStepTerminate  = "terminate"
)

// ErrAdtyManualRefundTableMissing is returned when manual-refund occupancy
// cannot be read or written because the table is absent.
var ErrAdtyManualRefundTableMissing = errors.New("adty manual refund table is missing")

// AdtyManualRefund is one authorized manual top-up refund. Sub-steps are
// recorded independently; overall completion requires every required step.
type AdtyManualRefund struct {
	Id               int64           `json:"id"`
	RefundId         string          `json:"refund_id" gorm:"uniqueIndex;type:varchar(64);not null"`
	TradeNo          string          `json:"trade_no" gorm:"index;type:varchar(64);not null"`
	UserId           int             `json:"user_id" gorm:"index"`
	SiteId           int             `json:"site_id"`
	Status           string          `json:"status" gorm:"type:varchar(32);index;not null"`
	RequestUSD       decimal.Decimal `json:"request_usd" gorm:"type:decimal(20,8);not null"`
	RequestQuota     int64           `json:"request_quota" gorm:"not null"`
	Reason           string          `json:"reason" gorm:"type:text"`
	ForeignReference decimal.Decimal `json:"foreign_reference" gorm:"type:decimal(20,8);not null;default:0"`

	ApplyOperatorId int    `json:"apply_operator_id"`
	AppliedAt       int64  `json:"applied_at"`
	ApplyIdemKey    string `json:"apply_idem_key" gorm:"type:varchar(64)"`

	ApproveOperatorId int    `json:"approve_operator_id"`
	ApprovedAt        int64  `json:"approved_at"`
	ApproveIdemKey    string `json:"approve_idem_key" gorm:"type:varchar(64)"`
	ApproveNote       string `json:"approve_note" gorm:"type:text"`

	ReclaimOperatorId         int             `json:"reclaim_operator_id"`
	ReclaimedAt               int64           `json:"reclaimed_at"`
	ReclaimIdemKey            string          `json:"reclaim_idem_key" gorm:"type:varchar(64)"`
	ReclaimablePrincipalQuota int64           `json:"reclaimable_principal_quota"`
	DeductedPrincipalQuota    int64           `json:"deducted_principal_quota"`
	ConsumedPrincipalQuota    int64           `json:"consumed_principal_quota"`
	ShortfallQuota            int64           `json:"shortfall_quota"`
	ShortfallUSD              decimal.Decimal `json:"shortfall_usd" gorm:"type:decimal(20,8);not null;default:0"`

	BonusOperatorId     int    `json:"bonus_operator_id"`
	BonusAt             int64  `json:"bonus_at"`
	BonusIdemKey        string `json:"bonus_idem_key" gorm:"type:varchar(64)"`
	BonusPlannedQuota   int64  `json:"bonus_planned_quota"`
	BonusDeductedQuota  int64  `json:"bonus_deducted_quota"`
	BonusShortfallQuota int64  `json:"bonus_shortfall_quota"`

	CommissionOperatorId int    `json:"commission_operator_id"`
	CommissionAt         int64  `json:"commission_at"`
	CommissionIdemKey    string `json:"commission_idem_key" gorm:"type:varchar(64)"`
	CommissionSnapshot   string `json:"commission_snapshot" gorm:"type:text"`

	PayoutStatus     string          `json:"payout_status" gorm:"type:varchar(16)"`
	PayoutCurrency   string          `json:"payout_currency" gorm:"type:varchar(16)"`
	PayoutAmount     decimal.Decimal `json:"payout_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PayoutAt         int64           `json:"payout_at"`
	PayoutRecordedAt int64           `json:"payout_recorded_at"`
	PayoutVoucher    string          `json:"payout_voucher" gorm:"type:varchar(255)"`
	PayoutOperatorId int             `json:"payout_operator_id"`
	PayoutIdemKey    string          `json:"payout_idem_key" gorm:"type:varchar(64)"`

	CompleteOperatorId int    `json:"complete_operator_id"`
	CompletedAt        int64  `json:"completed_at"`
	CompleteIdemKey    string `json:"complete_idem_key" gorm:"type:varchar(64)"`

	TerminateOperatorId int    `json:"terminate_operator_id"`
	TerminatedAt        int64  `json:"terminated_at"`
	TerminateIdemKey    string `json:"terminate_idem_key" gorm:"type:varchar(64)"`
	TerminateNote       string `json:"terminate_note" gorm:"type:text"`

	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyManualRefund) TableName() string { return "adty_manual_refunds" }

// AdtyManualRefundOccupancy is the official cap breakdown for one original order.
// L is the paid USD principal in quota units. S is completed refund occupancy
// (historical one-shot RefundQuota plus completed machine rows, stored on the
// order). H and U come from in-progress machine rows and are not added to
// RefundQuota or refund ratios until the refund is completed.
type AdtyManualRefundOccupancy struct {
	FaceQuota       int64  `json:"face_quota"`
	CompletedQuota  int64  `json:"completed_quota"`
	InProgressQuota int64  `json:"in_progress_quota"`
	UnknownQuota    int64  `json:"unknown_quota"`
	OccupiedQuota   int64  `json:"occupied_quota"`
	RemainingQuota  int64  `json:"remaining_quota"`
	FaceUSD         string `json:"face_usd"`
	CompletedUSD    string `json:"completed_usd"`
	InProgressUSD   string `json:"in_progress_usd"`
	UnknownUSD      string `json:"unknown_usd"`
	OccupiedUSD     string `json:"occupied_usd"`
	RemainingUSD    string `json:"remaining_usd"`
}

// AdtyManualRefundResult is one state-machine step result.
type AdtyManualRefundResult struct {
	Refund     *AdtyManualRefund                `json:"refund"`
	Occupancy  *AdtyManualRefundOccupancy       `json:"occupancy"`
	Ledger     *AdtyBalanceLedger               `json:"ledger,omitempty"`
	Facts      *AdtyRefundPageFacts             `json:"facts,omitempty"`
	Commission *AdtySiteCommissionRefundPreview `json:"commission,omitempty"`
}

func adtyRequireManualRefundTable(tx *gorm.DB) error {
	if tx == nil || !tx.Migrator().HasTable(&AdtyManualRefund{}) {
		return ErrAdtyManualRefundTableMissing
	}
	return nil
}

func adtyManualRefundOccupancyBucket(row AdtyManualRefund) string {
	switch row.Status {
	case AdtyManualRefundStatusCompleted, AdtyManualRefundStatusTerminated:
		return ""
	default:
		if row.PayoutStatus == AdtyManualRefundPayoutUnknown {
			return "U"
		}
		return "H"
	}
}

func adtyQuotaUSDString(quota int64) string {
	return decimal.NewFromInt(quota).Div(common.QuotaPerUnitDecimal()).String()
}

func adtyBuildManualRefundOccupancy(ext *AdtyTopupOrder, held, unknown int64) *AdtyManualRefundOccupancy {
	face := ext.FaceQuota()
	completed := ext.RefundQuota
	if completed < 0 {
		completed = 0
	}
	if held < 0 {
		held = 0
	}
	if unknown < 0 {
		unknown = 0
	}
	occupied := completed + held + unknown
	remaining := face - occupied
	if remaining < 0 {
		remaining = 0
	}
	return &AdtyManualRefundOccupancy{
		FaceQuota:       face,
		CompletedQuota:  completed,
		InProgressQuota: held,
		UnknownQuota:    unknown,
		OccupiedQuota:   occupied,
		RemainingQuota:  remaining,
		FaceUSD:         ext.FaceUSD.String(),
		CompletedUSD:    adtyQuotaUSDString(completed),
		InProgressUSD:   adtyQuotaUSDString(held),
		UnknownUSD:      adtyQuotaUSDString(unknown),
		OccupiedUSD:     adtyQuotaUSDString(occupied),
		RemainingUSD:    adtyQuotaUSDString(remaining),
	}
}

func adtySumManualRefundOccupancy(rows []AdtyManualRefund) (held, unknown int64) {
	for _, row := range rows {
		switch adtyManualRefundOccupancyBucket(row) {
		case "H":
			held += row.RequestQuota
		case "U":
			unknown += row.RequestQuota
		}
	}
	return held, unknown
}

func lockAdtyManualRefundsTx(tx *gorm.DB, tradeNo string) ([]AdtyManualRefund, error) {
	if err := adtyRequireManualRefundTable(tx); err != nil {
		return nil, err
	}
	var rows []AdtyManualRefund
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func lockAdtyManualRefundByIDTx(tx *gorm.DB, tradeNo, refundID string) (*AdtyManualRefund, []AdtyManualRefund, error) {
	rows, err := lockAdtyManualRefundsTx(tx, tradeNo)
	if err != nil {
		return nil, nil, err
	}
	refundID = strings.TrimSpace(refundID)
	for i := range rows {
		if rows[i].RefundId == refundID {
			return &rows[i], rows, nil
		}
	}
	return nil, rows, common.NewMessage("Refund not found")
}

func adtyManualRefundCapRemaining(face, completed, held, unknown int64) int64 {
	if face <= 0 {
		return 0
	}
	used := completed + held + unknown
	if used < 0 || used >= face {
		return 0
	}
	return face - used
}

func adtyFindManualRefundByIDTx(tx *gorm.DB, refundID string) (*AdtyManualRefund, error) {
	if err := adtyRequireManualRefundTable(tx); err != nil {
		return nil, err
	}
	var rows []AdtyManualRefund
	if err := tx.Where("refund_id = ?", refundID).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}
