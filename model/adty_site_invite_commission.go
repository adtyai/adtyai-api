package model

import (
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AdtySiteCommissionPendingCoverage = "pending_coverage"
	AdtySiteCommissionCovered         = "covered"
	AdtySiteCommissionAvailable       = "available"
	AdtySiteCommissionClawedBack      = "clawed_back"

	// Official DEC-025 display states. A row can be split across several of
	// these at once. They are derived from amount fields and are not a
	// replacement for the recorded status column.
	AdtySiteCommissionStateOnHold              = "on_hold"
	AdtySiteCommissionStatePendingCoverage     = "pending_coverage"
	AdtySiteCommissionStateConverted           = "converted"
	AdtySiteCommissionStateWithdrawalRequested = "withdrawal_requested"
	AdtySiteCommissionStateWithdrawnReleased   = "withdrawn_released"
	AdtySiteCommissionStateNotIssued           = "not_issued"
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

// AdtySiteCommissionStateAmount is one official lifecycle state and its live amount.
type AdtySiteCommissionStateAmount struct {
	State     string `json:"state"`
	AmountUSD string `json:"amount_usd"`
}

// AdtySiteCommissionRefundPreview is the official commission view for a refund
// preview or refund page. Expected amounts follow the after-refund splitter:
// release of remaining hold, clawback of converted balance, recovery of
// withdrawn payout, and cancellation of pending-coverage remainder.
type AdtySiteCommissionRefundPreview struct {
	IssuedUSD                        string                          `json:"issued_usd"`
	States                           []AdtySiteCommissionStateAmount `json:"states"`
	AmountUSD                        string                          `json:"amount_usd"`
	EarningsHoldUSD                  string                          `json:"earnings_hold_usd,omitempty"`
	MarginHoldUSD                    string                          `json:"margin_hold_usd,omitempty"`
	FrozenUSD                        string                          `json:"frozen_usd,omitempty"`
	ConvertedUSD                     string                          `json:"converted_usd,omitempty"`
	ConvertedClawedUSD               string                          `json:"converted_clawed_usd,omitempty"`
	WithdrawnUSD                     string                          `json:"withdrawn_usd,omitempty"`
	ClawedUSD                        string                          `json:"clawed_usd,omitempty"`
	RecoveredUSD                     string                          `json:"recovered_usd,omitempty"`
	RecoveryUSD                      string                          `json:"recovery_usd,omitempty"`
	RecordedStatus                   string                          `json:"recorded_status,omitempty"`
	ExpectedReleaseUSD               string                          `json:"expected_release_usd"`
	ExpectedConvertedClawbackUSD     string                          `json:"expected_converted_clawback_usd"`
	ExpectedWithdrawnRecoveryUSD     string                          `json:"expected_withdrawn_recovery_usd"`
	ExpectedPendingCoverageCancelUSD string                          `json:"expected_pending_coverage_cancel_usd"`
	WithdrawnRecoveryLiable          string                          `json:"withdrawn_recovery_liable,omitempty"`
	ExpectedClawbackUSD              string                          `json:"expected_clawback_usd,omitempty"`
}

// AdtySiteInviteCommissionView is a commission row plus its official state breakdown.
type AdtySiteInviteCommissionView struct {
	AdtySiteInviteCommission
	OfficialStates []AdtySiteCommissionStateAmount `json:"official_states"`
}

// SplitDecimalByWeights assigns total across positive weights, giving any
// remainder to the last positive weight. Zero weights stay zero.
func SplitDecimalByWeights(total decimal.Decimal, weights []decimal.Decimal) []decimal.Decimal {
	out := make([]decimal.Decimal, len(weights))
	sum := decimal.Zero
	last := -1
	for i, weight := range weights {
		if weight.IsPositive() {
			sum = sum.Add(weight)
			last = i
		}
	}
	if !total.IsPositive() || !sum.IsPositive() {
		return out
	}
	if total.GreaterThan(sum) {
		total = sum
	}
	assigned := decimal.Zero
	for i, weight := range weights {
		if !weight.IsPositive() {
			continue
		}
		if i == last {
			out[i] = total.Sub(assigned)
			break
		}
		part := total.Mul(weight).Div(sum).Round(8)
		if assigned.Add(part).GreaterThan(total) {
			part = total.Sub(assigned)
		}
		out[i] = part
		assigned = assigned.Add(part)
	}
	return out
}

func siteInviteCommissionConvertedLive(row AdtySiteInviteCommission) decimal.Decimal {
	live := row.ConvertedUSD.Sub(row.ConvertedClawedUSD)
	if live.IsNegative() {
		return decimal.Zero
	}
	return live
}

func siteInviteCommissionHoldUSD(row AdtySiteInviteCommission) decimal.Decimal {
	hold := row.EarningsHoldUSD.Add(row.MarginHoldUSD)
	if hold.IsNegative() {
		return decimal.Zero
	}
	return hold
}

func siteInviteCommissionUncoveredUSD(row AdtySiteInviteCommission, withdrawnLive decimal.Decimal) decimal.Decimal {
	if withdrawnLive.IsNegative() {
		withdrawnLive = decimal.Zero
	}
	uncovered := row.AmountUSD.Sub(row.ClawedUSD).Sub(siteInviteCommissionHoldUSD(row)).Sub(siteInviteCommissionConvertedLive(row)).Sub(withdrawnLive)
	if uncovered.IsNegative() {
		return decimal.Zero
	}
	return uncovered
}

// SiteInviteCommissionRefundParts splits this refund's claw the same way
// adtySiteInviteCommissionAfterRefundTx does. It does not move money.
func SiteInviteCommissionRefundParts(row AdtySiteInviteCommission, withdrawnLive decimal.Decimal, refundQuota, faceQuota int64) (release, convertedClaw, withdrawnRecovery, pendingCancel, thisClaw decimal.Decimal) {
	thisClaw = SiteInviteCommissionThisClaw(row.AmountUSD, row.ClawedUSD, refundQuota, faceQuota)
	if withdrawnLive.IsNegative() {
		withdrawnLive = decimal.Zero
	}
	parts := SplitDecimalByWeights(thisClaw, []decimal.Decimal{
		siteInviteCommissionHoldUSD(row),
		siteInviteCommissionConvertedLive(row),
		withdrawnLive,
		siteInviteCommissionUncoveredUSD(row, withdrawnLive),
	})
	return parts[0], parts[1], parts[2], parts[3], thisClaw
}

// SiteInviteCommissionOfficialStates lists live amounts under official DEC-025
// names. It does not use covered, available, or clawed_back as product states.
func SiteInviteCommissionOfficialStates(row AdtySiteInviteCommission, withdrawnLive decimal.Decimal) []AdtySiteCommissionStateAmount {
	hold := siteInviteCommissionHoldUSD(row)
	pendingWithdrawal := row.FrozenUSD
	if pendingWithdrawal.IsNegative() {
		pendingWithdrawal = decimal.Zero
	}
	onHold := hold.Sub(pendingWithdrawal)
	if onHold.IsNegative() {
		onHold = decimal.Zero
	}
	states := make([]AdtySiteCommissionStateAmount, 0, 5)
	appendPositive := func(state string, amount decimal.Decimal) {
		if amount.IsPositive() {
			states = append(states, AdtySiteCommissionStateAmount{State: state, AmountUSD: amount.String()})
		}
	}
	appendPositive(AdtySiteCommissionStateOnHold, onHold)
	appendPositive(AdtySiteCommissionStatePendingCoverage, siteInviteCommissionUncoveredUSD(row, withdrawnLive))
	appendPositive(AdtySiteCommissionStateConverted, siteInviteCommissionConvertedLive(row))
	appendPositive(AdtySiteCommissionStateWithdrawalRequested, pendingWithdrawal)
	appendPositive(AdtySiteCommissionStateWithdrawnReleased, row.WithdrawnUSD)
	return states
}

// SiteInviteCommissionWithdrawnLive is the withdrawn amount that has not yet
// been recorded as an open or historical withdrawn recovery.
func SiteInviteCommissionWithdrawnLive(db *gorm.DB, commissionID int64, withdrawn decimal.Decimal) (decimal.Decimal, error) {
	if db == nil || !withdrawn.IsPositive() {
		return decimal.Zero, nil
	}
	var rows []AdtyCommissionRecovery
	if err := db.Where("commission_id = ? AND kind = ?", commissionID, "withdrawn").Find(&rows).Error; err != nil {
		return decimal.Zero, err
	}
	recorded := decimal.Zero
	for _, row := range rows {
		recorded = recorded.Add(row.AmountUSD)
	}
	left := withdrawn.Sub(recorded)
	if left.IsNegative() {
		return decimal.Zero, nil
	}
	return left, nil
}

// BuildSiteInviteCommissionRefundPreview fills official states and the four
// expected refund amounts. It does not move money.
func BuildSiteInviteCommissionRefundPreview(row AdtySiteInviteCommission, withdrawnLive decimal.Decimal, refundQuota, faceQuota int64) *AdtySiteCommissionRefundPreview {
	release, convertedClaw, withdrawnRecovery, pendingCancel, thisClaw := SiteInviteCommissionRefundParts(row, withdrawnLive, refundQuota, faceQuota)
	view := &AdtySiteCommissionRefundPreview{
		IssuedUSD:                        row.AmountUSD.String(),
		States:                           SiteInviteCommissionOfficialStates(row, withdrawnLive),
		AmountUSD:                        row.AmountUSD.String(),
		EarningsHoldUSD:                  row.EarningsHoldUSD.String(),
		MarginHoldUSD:                    row.MarginHoldUSD.String(),
		FrozenUSD:                        row.FrozenUSD.String(),
		ConvertedUSD:                     row.ConvertedUSD.String(),
		ConvertedClawedUSD:               row.ConvertedClawedUSD.String(),
		WithdrawnUSD:                     row.WithdrawnUSD.String(),
		ClawedUSD:                        row.ClawedUSD.String(),
		RecoveredUSD:                     row.RecoveredUSD.String(),
		RecoveryUSD:                      row.RecoveryUSD.String(),
		RecordedStatus:                   row.Status,
		ExpectedReleaseUSD:               release.String(),
		ExpectedConvertedClawbackUSD:     convertedClaw.String(),
		ExpectedWithdrawnRecoveryUSD:     withdrawnRecovery.String(),
		ExpectedPendingCoverageCancelUSD: pendingCancel.String(),
		ExpectedClawbackUSD:              thisClaw.String(),
	}
	if withdrawnRecovery.IsPositive() {
		view.WithdrawnRecoveryLiable = "inviter"
	}
	return view
}

// LoadSiteInviteCommissionRefundPreview reads the commission for one top-up
// and builds the official refund preview. It returns nil when none was issued.
func LoadSiteInviteCommissionRefundPreview(db *gorm.DB, tradeNo string, refundQuota, faceQuota int64) (*AdtySiteCommissionRefundPreview, error) {
	if db == nil || strings.TrimSpace(tradeNo) == "" {
		return nil, nil
	}
	var row AdtySiteInviteCommission
	if err := db.Where("trade_no = ?", tradeNo).Limit(1).Find(&row).Error; err != nil {
		return nil, err
	}
	if row.Id == 0 || !row.AmountUSD.IsPositive() {
		return nil, nil
	}
	withdrawnLive, err := SiteInviteCommissionWithdrawnLive(db, row.Id, row.WithdrawnUSD)
	if err != nil {
		return nil, err
	}
	return BuildSiteInviteCommissionRefundPreview(row, withdrawnLive, refundQuota, faceQuota), nil
}

// PresentSiteInviteCommissions attaches official state amounts to each row.
func PresentSiteInviteCommissions(db *gorm.DB, rows []AdtySiteInviteCommission) ([]AdtySiteInviteCommissionView, error) {
	views := make([]AdtySiteInviteCommissionView, 0, len(rows))
	if len(rows) == 0 {
		return views, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Id)
	}
	recorded := map[int64]decimal.Decimal{}
	if db != nil {
		var recoveries []AdtyCommissionRecovery
		if err := db.Where("commission_id IN ? AND kind = ?", ids, "withdrawn").Find(&recoveries).Error; err != nil {
			return nil, err
		}
		for _, recovery := range recoveries {
			recorded[recovery.CommissionId] = recorded[recovery.CommissionId].Add(recovery.AmountUSD)
		}
	}
	for _, row := range rows {
		withdrawnLive := row.WithdrawnUSD.Sub(recorded[row.Id])
		if withdrawnLive.IsNegative() || !row.WithdrawnUSD.IsPositive() {
			withdrawnLive = decimal.Zero
		}
		views = append(views, AdtySiteInviteCommissionView{
			AdtySiteInviteCommission: row,
			OfficialStates:           SiteInviteCommissionOfficialStates(row, withdrawnLive),
		})
	}
	return views, nil
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
