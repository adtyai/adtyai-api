package model

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// CumulativeGiftReclaim is this refund's gift recovery:
// gift * min(cumulativeRefundedPrincipal / originalPrincipal, 1) minus earlier recoveries.
// cumulativeRefundedPrincipal must be completed refund principal (S) only.
func CumulativeGiftReclaim(gift, principal, refundedBefore, refundNow decimal.Decimal) decimal.Decimal {
	if !gift.IsPositive() || !principal.IsPositive() || !refundNow.IsPositive() {
		return decimal.Zero
	}
	ratio := func(refunded decimal.Decimal) decimal.Decimal {
		if refunded.IsNegative() {
			return decimal.Zero
		}
		value := refunded.Div(principal)
		if value.GreaterThan(decimal.NewFromInt(1)) {
			return decimal.NewFromInt(1)
		}
		return value
	}
	return gift.Mul(ratio(refundedBefore.Add(refundNow)).Sub(ratio(refundedBefore))).Round(8)
}

// ManualRefundUSDQuota converts an admin-entered USD amount into principal quota.
// occupiedUSD is S+H+U already occupying the original order's paid principal L.
func ManualRefundUSDQuota(faceUSD, occupiedUSD, requestUSD decimal.Decimal) (int64, error) {
	if !requestUSD.IsPositive() {
		return 0, common.NewMessage("Refund amount must be positive")
	}
	remaining := faceUSD.Sub(occupiedUSD)
	if requestUSD.GreaterThan(remaining) {
		return 0, common.NewMessage("Refund amount exceeds the remaining principal")
	}
	quota, err := common.WalletQuotaFromDecimalStrict(requestUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil {
		return 0, err
	}
	if quota <= 0 {
		return 0, common.NewMessage("Refund amount must be positive")
	}
	return int64(quota), nil
}

// AdtyRefundPageFacts is the recorded refund-page check. Amounts come from the
// order and its fund lots. A blank lot field means that lot was not recorded.
type AdtyRefundPageFacts struct {
	TradeNo               string                           `json:"trade_no"`
	SiteId                int                              `json:"site_id"`
	UserId                int                              `json:"user_id"`
	FaceUSD               string                           `json:"face_usd"`
	PayCurrency           string                           `json:"pay_currency"`
	PaidAmount            string                           `json:"paid_amount"`
	PaidCurrency          string                           `json:"paid_currency"`
	OrderRate             string                           `json:"order_rate"`
	PrincipalConsumedUSD  string                           `json:"principal_consumed_usd,omitempty"`
	PrincipalRemainingUSD string                           `json:"principal_remaining_usd,omitempty"`
	PendingQuota          int64                            `json:"pending_quota"`
	RefundOccupiedQuota   int64                            `json:"refund_occupied_quota"`
	RefundCompletedQuota  int64                            `json:"refund_completed_quota"`
	RefundInProgressQuota int64                            `json:"refund_in_progress_quota"`
	RefundUnknownQuota    int64                            `json:"refund_unknown_quota"`
	RefundRemainingQuota  int64                            `json:"refund_remaining_quota"`
	RefundCompletedUSD    string                           `json:"refund_completed_usd,omitempty"`
	RefundInProgressUSD   string                           `json:"refund_in_progress_usd,omitempty"`
	RefundUnknownUSD      string                           `json:"refund_unknown_usd,omitempty"`
	RefundRemainingUSD    string                           `json:"refund_remaining_usd,omitempty"`
	ManualShortfallQuota  int64                            `json:"manual_shortfall_quota"`
	ManualShortfallUSD    string                           `json:"manual_shortfall_usd,omitempty"`
	BonusUSD              string                           `json:"bonus_usd"`
	BonusUsedQuota        int64                            `json:"bonus_used_quota"`
	BonusRemainingQuota   int64                            `json:"bonus_remaining_quota"`
	CumulativeRefundUSD   string                           `json:"cumulative_refund_usd"`
	ForeignReference      string                           `json:"foreign_reference,omitempty"`
	RefundProof           string                           `json:"refund_proof,omitempty"`
	Commission            *AdtySiteCommissionRefundPreview `json:"commission,omitempty"`
	ManualRefunds         []AdtyManualRefund               `json:"manual_refunds,omitempty"`
	Occupancy             *AdtyManualRefundOccupancy       `json:"occupancy,omitempty"`
}

// LoadAdtyRefundPageFacts reads the order, its fund lots, the customer's
// in-flight precharge, official occupancy, and the site-invite commission
// breakdown. requestUSD, when positive, adds the foreign reference and the
// expected commission split as if this incremental refund later completes.
// In-progress and unknown occupancy is not included in that ratio preview.
// It does not write.
func LoadAdtyRefundPageFacts(tradeNo string, requestUSD decimal.Decimal) (*AdtyRefundPageFacts, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, common.NewMessage("Invalid parameters")
	}
	var ext AdtyTopupOrder
	if err := DB.Where("trade_no = ?", tradeNo).First(&ext).Error; err != nil {
		return nil, err
	}
	var user User
	if err := DB.Select("id", "quota_pending").First(&user, ext.UserId).Error; err != nil {
		return nil, err
	}
	if err := adtyRequireManualRefundTable(DB); err != nil {
		return nil, err
	}
	var refunds []AdtyManualRefund
	if err := DB.Where("trade_no = ?", tradeNo).Order("id").Find(&refunds).Error; err != nil {
		return nil, err
	}
	held, unknown := adtySumManualRefundOccupancy(refunds)
	occupancy := adtyBuildManualRefundOccupancy(&ext, held, unknown)
	var shortfall int64
	for _, row := range refunds {
		shortfall += row.ShortfallQuota
	}
	facts := &AdtyRefundPageFacts{
		TradeNo: tradeNo, SiteId: ext.SiteId, UserId: ext.UserId,
		FaceUSD: ext.FaceUSD.String(), PayCurrency: ext.PayCurrency,
		PaidAmount: ext.PaidAmount.String(), PaidCurrency: ext.PaidCurrency,
		OrderRate: ext.PriceSnapshot.String(), PendingQuota: int64(user.QuotaPending),
		RefundOccupiedQuota:   occupancy.OccupiedQuota,
		RefundCompletedQuota:  occupancy.CompletedQuota,
		RefundInProgressQuota: occupancy.InProgressQuota,
		RefundUnknownQuota:    occupancy.UnknownQuota,
		RefundRemainingQuota:  occupancy.RemainingQuota,
		RefundCompletedUSD:    occupancy.CompletedUSD,
		RefundInProgressUSD:   occupancy.InProgressUSD,
		RefundUnknownUSD:      occupancy.UnknownUSD,
		RefundRemainingUSD:    occupancy.RemainingUSD,
		ManualShortfallQuota:  shortfall,
		ManualShortfallUSD:    adtyQuotaUSDString(shortfall),
		BonusUSD:              ext.BonusUSD.String(),
		CumulativeRefundUSD:   occupancy.CompletedUSD,
		RefundProof:           ext.RefundProof,
		ManualRefunds:         refunds,
		Occupancy:             occupancy,
	}
	var lots []AdtyFundBatch
	if err := DB.Where("user_id = ? AND source = ?", ext.UserId, tradeNo).Find(&lots).Error; err != nil {
		return nil, err
	}
	var principalOriginal, principalRemaining, bonusOriginal, bonusRemaining int64
	var sawPrincipal, sawBonus bool
	for _, lot := range lots {
		switch lot.Kind {
		case "principal":
			principalOriginal += lot.Original
			principalRemaining += lot.Remaining
			sawPrincipal = true
		case "bonus":
			bonusOriginal += lot.Original
			bonusRemaining += lot.Remaining
			sawBonus = true
		}
	}
	if sawPrincipal {
		facts.PrincipalConsumedUSD = decimal.NewFromInt(principalOriginal - principalRemaining).Div(common.QuotaPerUnitDecimal()).String()
		facts.PrincipalRemainingUSD = decimal.NewFromInt(principalRemaining).Div(common.QuotaPerUnitDecimal()).String()
	}
	if sawBonus {
		facts.BonusUsedQuota = bonusOriginal - bonusRemaining
		facts.BonusRemainingQuota = bonusRemaining
	}
	previewQuota := ext.RefundQuota
	if requestUSD.IsPositive() {
		requestQuota, err := ManualRefundUSDQuota(ext.FaceUSD, decimal.NewFromInt(occupancy.OccupiedQuota).Div(common.QuotaPerUnitDecimal()), requestUSD)
		if err != nil {
			return nil, err
		}
		if requestQuota > occupancy.RemainingQuota {
			return nil, common.NewMessage("Refund amount exceeds the remaining principal")
		}
		facts.ForeignReference = adtyRefundAmount(&ext, requestQuota, ext.FaceQuota()).String()
		previewQuota = ext.RefundQuota + requestQuota
	}
	commission, err := LoadSiteInviteCommissionRefundPreview(DB, tradeNo, previewQuota, ext.FaceQuota())
	if err != nil {
		return nil, err
	}
	facts.Commission = commission
	return facts, nil
}

func attachAdtyManualRefundFacts(result *AdtyManualRefundResult, tradeNo string, requestUSD decimal.Decimal) error {
	facts, err := LoadAdtyRefundPageFacts(tradeNo, requestUSD)
	if err != nil {
		return err
	}
	result.Facts = facts
	if facts != nil {
		result.Commission = facts.Commission
		if result.Occupancy == nil {
			result.Occupancy = facts.Occupancy
		}
	}
	return nil
}

func adtyManualRefundView(refund *AdtyManualRefund, occupancy *AdtyManualRefundOccupancy, ledger *AdtyBalanceLedger) *AdtyManualRefundResult {
	return &AdtyManualRefundResult{Refund: refund, Occupancy: occupancy, Ledger: ledger}
}

func adtyNormalizeRefundID(refundID string) (string, error) {
	refundID = strings.TrimSpace(refundID)
	if refundID == "" || len(refundID) > 64 {
		return "", common.NewMessage("Invalid idempotency key")
	}
	return refundID, nil
}

func adtyNormalizeStepIdemKey(refundID, step, idemKey string) (string, error) {
	idemKey = strings.TrimSpace(idemKey)
	if idemKey == "" {
		idemKey = refundID + ":" + step
	}
	if len(idemKey) > 64 {
		return "", common.NewMessage("Invalid idempotency key")
	}
	return idemKey, nil
}

func adtyOrderLotRemainingTx(tx *gorm.DB, userID int, tradeNo string) (principal, bonus int64, err error) {
	if err := adtyRequireFundBatchTable(tx); err != nil {
		return 0, 0, err
	}
	var lots []AdtyFundBatch
	if err := tx.Where("user_id = ? AND source = ?", userID, tradeNo).Find(&lots).Error; err != nil {
		return 0, 0, err
	}
	for _, lot := range lots {
		switch lot.Kind {
		case "principal":
			principal += lot.Remaining
		case "bonus":
			bonus += lot.Remaining
		}
	}
	return principal, bonus, nil
}

func adtySpendableQuota(before, checkinQuota int64) int64 {
	spendable := before - checkinQuota
	if spendable < 0 {
		return 0
	}
	return spendable
}

func adtyDebitUserQuotaTx(tx *gorm.DB, userID int, deducted int64) error {
	if deducted <= 0 {
		return nil
	}
	if err := tx.Model(&User{}).Where("id = ?", userID).Update("quota", gorm.Expr("quota - ?", deducted)).Error; err != nil {
		return err
	}
	return clampCheckinQuotaDB(tx, userID)
}

func adtyManualRefundLedger(userID int, deducted, before int64, typRef, reason string, operatorID int, key string) *AdtyBalanceLedger {
	ledgerKey := key
	return &AdtyBalanceLedger{
		UserId: userID, DeltaQuota: -deducted, BalanceBefore: before, BalanceAfter: before - deducted,
		Type: AdtyLedgerTypeRefund, RefType: "topup", RefId: typRef,
		Reason: reason, OperatorId: operatorID, IdempotencyKey: &ledgerKey,
	}
}

func adtyEnsureCreditedOrder(topUp *TopUp) error {
	if topUp.Status != common.TopUpStatusSuccess && topUp.Status != TopUpStatusRefunded {
		return common.NewMessage("Only credited orders can be refunded")
	}
	return nil
}

func adtyRequireApprovedManualRefund(row *AdtyManualRefund) error {
	if row.Status == AdtyManualRefundStatusTerminated {
		return common.NewMessage("This refund has been terminated")
	}
	if row.Status == AdtyManualRefundStatusCompleted {
		return common.NewMessage("This refund is already complete")
	}
	if row.ApprovedAt == 0 {
		return common.NewMessage("This refund has not been approved")
	}
	return nil
}

// ApplyAdtyTopUpManualRefund files a manual refund application. The authorized
// amount occupies H and is capped by S+H+U+r ≤ L. It does not debit the wallet,
// reclaim gift, handle commission, or record an offline payout.
func ApplyAdtyTopUpManualRefund(tradeNo, reason string, operatorID int, refundID string, requestUSD decimal.Decimal) (*AdtyManualRefundResult, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, common.NewMessage("A refund reason is required")
	}
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	var result *AdtyManualRefundResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		topUp, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		existing, err := adtyFindManualRefundByIDTx(tx, refundID)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.TradeNo != tradeNo {
				return ErrAdtyIdempotencyConflict
			}
			rows, err := lockAdtyManualRefundsTx(tx, tradeNo)
			if err != nil {
				return err
			}
			held, unknown := adtySumManualRefundOccupancy(rows)
			result = adtyManualRefundView(existing, adtyBuildManualRefundOccupancy(ext, held, unknown), nil)
			return nil
		}
		if err := adtyEnsureCreditedOrder(topUp); err != nil {
			return err
		}
		rows, err := lockAdtyManualRefundsTx(tx, tradeNo)
		if err != nil {
			return err
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		occupancy := adtyBuildManualRefundOccupancy(ext, held, unknown)
		requestQuota, err := ManualRefundUSDQuota(ext.FaceUSD, decimal.NewFromInt(occupancy.OccupiedQuota).Div(common.QuotaPerUnitDecimal()), requestUSD)
		if err != nil {
			return err
		}
		if requestQuota > occupancy.RemainingQuota {
			return common.NewMessage("Refund amount exceeds the remaining principal")
		}
		now := common.GetTimestamp()
		row := &AdtyManualRefund{
			RefundId:         refundID,
			TradeNo:          tradeNo,
			UserId:           topUp.UserId,
			SiteId:           ext.SiteId,
			Status:           AdtyManualRefundStatusSubmitted,
			RequestUSD:       requestUSD,
			RequestQuota:     requestQuota,
			Reason:           reason,
			ForeignReference: adtyRefundAmount(ext, requestQuota, ext.FaceQuota()),
			ApplyOperatorId:  operatorID,
			AppliedAt:        now,
			ApplyIdemKey:     refundID,
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		result = adtyManualRefundView(row, adtyBuildManualRefundOccupancy(ext, held+requestQuota, unknown), nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero); err != nil {
		return nil, err
	}
	return result, nil
}

// ApproveAdtyTopUpManualRefund authorizes a submitted refund. Occupancy stays in H/U.
func ApproveAdtyTopUpManualRefund(tradeNo, refundID string, operatorID int, idemKey, note string) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	idemKey, err = adtyNormalizeStepIdemKey(refundID, AdtyManualRefundStepApprove, idemKey)
	if err != nil {
		return nil, err
	}
	var result *AdtyManualRefundResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		row, rows, err := lockAdtyManualRefundByIDTx(tx, tradeNo, refundID)
		if err != nil {
			return err
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		occupancy := adtyBuildManualRefundOccupancy(ext, held, unknown)
		if row.ApprovedAt > 0 {
			result = adtyManualRefundView(row, occupancy, nil)
			return nil
		}
		if row.Status != AdtyManualRefundStatusSubmitted {
			return common.NewMessage("This refund cannot be approved")
		}
		now := common.GetTimestamp()
		if err := tx.Model(row).Updates(map[string]any{
			"status":              AdtyManualRefundStatusApproved,
			"approve_operator_id": operatorID,
			"approved_at":         now,
			"approve_idem_key":    idemKey,
			"approve_note":        strings.TrimSpace(note),
		}).Error; err != nil {
			return err
		}
		row.Status = AdtyManualRefundStatusApproved
		row.ApproveOperatorId, row.ApprovedAt, row.ApproveIdemKey = operatorID, now, idemKey
		row.ApproveNote = strings.TrimSpace(note)
		result = adtyManualRefundView(row, occupancy, nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero)
}

// ReclaimAdtyTopUpManualRefund records internal principal reclaim. Authorized r
// is not reduced when spendable or remaining principal is short; the gap is
// persisted as 人工处理差额. Other orders and accounts are not debited.
func ReclaimAdtyTopUpManualRefund(tradeNo, refundID string, operatorID int, idemKey string) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	idemKey, err = adtyNormalizeStepIdemKey(refundID, AdtyManualRefundStepReclaim, idemKey)
	if err != nil {
		return nil, err
	}
	var result *AdtyManualRefundResult
	var userID int
	var deducted int64
	replay := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		topUp, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		before, checkinQuota, err := lockUserQuotaForRefundTx(tx, topUp.UserId)
		if err != nil {
			return err
		}
		row, rows, err := lockAdtyManualRefundByIDTx(tx, tradeNo, refundID)
		if err != nil {
			return err
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		occupancy := adtyBuildManualRefundOccupancy(ext, held, unknown)
		if row.ReclaimedAt > 0 {
			existing, err := FindAdtyLedgerByIdemKeyTx(tx, row.ReclaimIdemKey)
			if err != nil {
				return err
			}
			result = adtyManualRefundView(row, occupancy, existing)
			replay = true
			return nil
		}
		if err := adtyRequireApprovedManualRefund(row); err != nil {
			return err
		}
		if existing, err := FindAdtyLedgerByIdemKeyTx(tx, idemKey); err != nil {
			return err
		} else if existing != nil {
			return ErrAdtyIdempotencyConflict
		}
		remainingPrincipal, _, err := adtyOrderLotRemainingTx(tx, topUp.UserId, tradeNo)
		if err != nil {
			return err
		}
		spendable := adtySpendableQuota(before, checkinQuota)
		reclaimable := remainingPrincipal
		if spendable < reclaimable {
			reclaimable = spendable
		}
		if row.RequestQuota < reclaimable {
			reclaimable = row.RequestQuota
		}
		if reclaimable < 0 {
			reclaimable = 0
		}
		consumed := row.RequestQuota - remainingPrincipal
		if remainingPrincipal > row.RequestQuota {
			consumed = 0
		}
		if consumed < 0 {
			consumed = 0
		}
		shortfall := row.RequestQuota - reclaimable
		if err := adtyDebitUserQuotaTx(tx, topUp.UserId, reclaimable); err != nil {
			return err
		}
		if reclaimable > 0 {
			if taken, err := ReclaimAdtyFundBatchesBySourceTx(tx, topUp.UserId, tradeNo, reclaimable, 0); err != nil {
				return err
			} else if taken != reclaimable {
				return common.NewMessage("Internal principal reclaim could not be applied to this order")
			}
		}
		ledger := adtyManualRefundLedger(topUp.UserId, reclaimable, before, tradeNo,
			row.Reason+" [manual_usd="+row.RequestUSD.String()+" shortfall="+adtyQuotaUSDString(shortfall)+" refund_id="+refundID+"]",
			operatorID, idemKey)
		if err := InsertAdtyLedgerTx(tx, ledger); err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(row).Updates(map[string]any{
			"reclaim_operator_id":         operatorID,
			"reclaimed_at":                now,
			"reclaim_idem_key":            idemKey,
			"reclaimable_principal_quota": remainingPrincipal,
			"deducted_principal_quota":    reclaimable,
			"consumed_principal_quota":    consumed,
			"shortfall_quota":             shortfall,
			"shortfall_usd":               decimal.NewFromInt(shortfall).Div(common.QuotaPerUnitDecimal()),
		}).Error; err != nil {
			return err
		}
		row.ReclaimOperatorId, row.ReclaimedAt, row.ReclaimIdemKey = operatorID, now, idemKey
		row.ReclaimablePrincipalQuota, row.DeductedPrincipalQuota = remainingPrincipal, reclaimable
		row.ConsumedPrincipalQuota, row.ShortfallQuota = consumed, shortfall
		row.ShortfallUSD = decimal.NewFromInt(shortfall).Div(common.QuotaPerUnitDecimal())
		userID, deducted = topUp.UserId, reclaimable
		result = adtyManualRefundView(row, occupancy, ledger)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !replay && deducted > 0 {
		if err := invalidateUserCache(userID); err != nil {
			common.SysError(common.LogText("refund cache update failed user_id=%d: %s", userID, err.Error()))
		}
	}
	return result, attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero)
}

func adtyPlannedBonusQuota(ext *AdtyTopupOrder, requestUSD decimal.Decimal) (int64, error) {
	alreadyUSD := decimal.NewFromInt(ext.RefundQuota).Div(common.QuotaPerUnitDecimal())
	reclaimUSD := CumulativeGiftReclaim(ext.BonusUSD, ext.FaceUSD, alreadyUSD, requestUSD)
	reclaimQuota, err := common.WalletQuotaFromDecimalStrict(reclaimUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil {
		return 0, err
	}
	return int64(reclaimQuota), nil
}

// PlanAdtyTopUpManualRefundBonus records the official cumulative-ratio gift
// reclaim expected when this refund later completes. It does not debit gift
// early and does not treat H/U as completed S.
func PlanAdtyTopUpManualRefundBonus(tradeNo, refundID string, operatorID int, idemKey string) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	idemKey, err = adtyNormalizeStepIdemKey(refundID, AdtyManualRefundStepBonus, idemKey)
	if err != nil {
		return nil, err
	}
	var result *AdtyManualRefundResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		row, rows, err := lockAdtyManualRefundByIDTx(tx, tradeNo, refundID)
		if err != nil {
			return err
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		occupancy := adtyBuildManualRefundOccupancy(ext, held, unknown)
		if row.BonusAt > 0 {
			result = adtyManualRefundView(row, occupancy, nil)
			return nil
		}
		if err := adtyRequireApprovedManualRefund(row); err != nil {
			return err
		}
		planned, err := adtyPlannedBonusQuota(ext, row.RequestUSD)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(row).Updates(map[string]any{
			"bonus_operator_id":   operatorID,
			"bonus_at":            now,
			"bonus_idem_key":      idemKey,
			"bonus_planned_quota": planned,
		}).Error; err != nil {
			return err
		}
		row.BonusOperatorId, row.BonusAt, row.BonusIdemKey, row.BonusPlannedQuota = operatorID, now, idemKey, planned
		result = adtyManualRefundView(row, occupancy, nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero)
}

// PlanAdtyTopUpManualRefundCommission records the official itemized commission
// preview for S plus this refund. Fund movement happens only at completion.
func PlanAdtyTopUpManualRefundCommission(tradeNo, refundID string, operatorID int, idemKey string) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	idemKey, err = adtyNormalizeStepIdemKey(refundID, AdtyManualRefundStepCommission, idemKey)
	if err != nil {
		return nil, err
	}
	var result *AdtyManualRefundResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		row, rows, err := lockAdtyManualRefundByIDTx(tx, tradeNo, refundID)
		if err != nil {
			return err
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		occupancy := adtyBuildManualRefundOccupancy(ext, held, unknown)
		if row.CommissionAt > 0 {
			result = adtyManualRefundView(row, occupancy, nil)
			return nil
		}
		if err := adtyRequireApprovedManualRefund(row); err != nil {
			return err
		}
		preview, err := LoadSiteInviteCommissionRefundPreview(tx, tradeNo, ext.RefundQuota+row.RequestQuota, ext.FaceQuota())
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(preview)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(row).Updates(map[string]any{
			"commission_operator_id": operatorID,
			"commission_at":          now,
			"commission_idem_key":    idemKey,
			"commission_snapshot":    string(snapshot),
		}).Error; err != nil {
			return err
		}
		row.CommissionOperatorId, row.CommissionAt, row.CommissionIdemKey = operatorID, now, idemKey
		row.CommissionSnapshot = string(snapshot)
		result = adtyManualRefundView(row, occupancy, nil)
		result.Commission = preview
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero)
}

// AdtyManualRefundPayout is the offline payment recorded by authorized staff.
type AdtyManualRefundPayout struct {
	Status   string
	Currency string
	Amount   decimal.Decimal
	PaidAt   int64
	Voucher  string
}

// RecordAdtyTopUpManualRefundPayout records the independent offline payout fact.
// A successful payout does not complete internal bookkeeping, and an internal
// shortfall does not change the authorized amount or mark payout done.
func RecordAdtyTopUpManualRefundPayout(tradeNo, refundID string, operatorID int, idemKey string, payout AdtyManualRefundPayout) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	idemKey, err = adtyNormalizeStepIdemKey(refundID, AdtyManualRefundStepPayout, idemKey)
	if err != nil {
		return nil, err
	}
	status := strings.TrimSpace(payout.Status)
	switch status {
	case AdtyManualRefundPayoutSuccess, AdtyManualRefundPayoutUnknown, AdtyManualRefundPayoutFailed:
	default:
		return nil, common.NewMessage("Invalid payout status")
	}
	currency := strings.TrimSpace(payout.Currency)
	voucher := strings.TrimSpace(payout.Voucher)
	if status == AdtyManualRefundPayoutSuccess {
		if currency == "" || !payout.Amount.IsPositive() || payout.PaidAt <= 0 || voucher == "" || len(voucher) > 255 {
			return nil, common.NewMessage("Invalid parameters")
		}
	} else if len(voucher) > 255 || len(currency) > 16 {
		return nil, common.NewMessage("Invalid parameters")
	}
	var result *AdtyManualRefundResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		row, rows, err := lockAdtyManualRefundByIDTx(tx, tradeNo, refundID)
		if err != nil {
			return err
		}
		if row.PayoutRecordedAt > 0 && row.PayoutStatus != AdtyManualRefundPayoutUnknown {
			held, unknown := adtySumManualRefundOccupancy(rows)
			result = adtyManualRefundView(row, adtyBuildManualRefundOccupancy(ext, held, unknown), nil)
			return nil
		}
		if err := adtyRequireApprovedManualRefund(row); err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(row).Updates(map[string]any{
			"payout_status":      status,
			"payout_currency":    currency,
			"payout_amount":      payout.Amount,
			"payout_at":          payout.PaidAt,
			"payout_recorded_at": now,
			"payout_voucher":     voucher,
			"payout_operator_id": operatorID,
			"payout_idem_key":    idemKey,
		}).Error; err != nil {
			return err
		}
		if voucher != "" {
			if err := tx.Model(&AdtyTopupOrder{}).Where("id = ?", ext.Id).Update("refund_proof", voucher).Error; err != nil {
				return err
			}
		}
		row.PayoutStatus, row.PayoutCurrency, row.PayoutAmount = status, currency, payout.Amount
		row.PayoutAt, row.PayoutRecordedAt, row.PayoutVoucher = payout.PaidAt, now, voucher
		row.PayoutOperatorId, row.PayoutIdemKey = operatorID, idemKey
		for i := range rows {
			if rows[i].RefundId == row.RefundId {
				rows[i] = *row
			}
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		result = adtyManualRefundView(row, adtyBuildManualRefundOccupancy(ext, held, unknown), nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero)
}

// CompleteAdtyTopUpManualRefund marks the refund overall complete only after
// apply, approve, internal reclaim, bonus plan, commission plan, and a
// successful offline payout have been persisted. Gift reclaim and commission
// fund movement use completed S, including this refund in the same transaction.
func CompleteAdtyTopUpManualRefund(tradeNo, refundID string, operatorID int, idemKey string) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	idemKey, err = adtyNormalizeStepIdemKey(refundID, AdtyManualRefundStepComplete, idemKey)
	if err != nil {
		return nil, err
	}
	var result *AdtyManualRefundResult
	var userID int
	var deducted int64
	var committedTopUp *TopUp
	var committedExt *AdtyTopupOrder
	replay := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		topUp, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		before, checkinQuota, err := lockUserQuotaForRefundTx(tx, topUp.UserId)
		if err != nil {
			return err
		}
		row, rows, err := lockAdtyManualRefundByIDTx(tx, tradeNo, refundID)
		if err != nil {
			return err
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		if row.Status == AdtyManualRefundStatusCompleted {
			existing, err := FindAdtyLedgerByIdemKeyTx(tx, row.CompleteIdemKey)
			if err != nil {
				return err
			}
			result = adtyManualRefundView(row, adtyBuildManualRefundOccupancy(ext, held, unknown), existing)
			replay = true
			return nil
		}
		if err := adtyRequireApprovedManualRefund(row); err != nil {
			return err
		}
		if row.ReclaimedAt == 0 {
			return common.NewMessage("Internal principal reclaim has not been recorded")
		}
		if row.BonusAt == 0 {
			return common.NewMessage("Gift reclaim has not been recorded")
		}
		if row.CommissionAt == 0 {
			return common.NewMessage("Commission handling has not been recorded")
		}
		if row.PayoutRecordedAt == 0 || row.PayoutStatus != AdtyManualRefundPayoutSuccess {
			return common.NewMessage("Offline payout has not been confirmed")
		}
		if existing, err := FindAdtyLedgerByIdemKeyTx(tx, idemKey); err != nil {
			return err
		} else if existing != nil {
			return ErrAdtyIdempotencyConflict
		}
		planned, err := adtyPlannedBonusQuota(ext, row.RequestUSD)
		if err != nil {
			return err
		}
		_, remainingBonus, err := adtyOrderLotRemainingTx(tx, topUp.UserId, tradeNo)
		if err != nil {
			return err
		}
		spendable := adtySpendableQuota(before, checkinQuota)
		bonusTaken := planned
		if remainingBonus < bonusTaken {
			bonusTaken = remainingBonus
		}
		if spendable < bonusTaken {
			bonusTaken = spendable
		}
		if bonusTaken < 0 {
			bonusTaken = 0
		}
		bonusShortfall := planned - bonusTaken
		if err := adtyDebitUserQuotaTx(tx, topUp.UserId, bonusTaken); err != nil {
			return err
		}
		if bonusTaken > 0 {
			if taken, err := ReclaimAdtyFundBatchesBySourceTx(tx, topUp.UserId, tradeNo, 0, bonusTaken); err != nil {
				return err
			} else if taken != bonusTaken {
				return common.NewMessage("Gift reclaim could not be applied to this order")
			}
		}
		ledger := adtyManualRefundLedger(topUp.UserId, bonusTaken, before, tradeNo,
			row.Reason+" [manual_bonus="+adtyQuotaUSDString(bonusTaken)+" refund_id="+refundID+"]",
			operatorID, idemKey)
		if err := InsertAdtyLedgerTx(tx, ledger); err != nil {
			return err
		}
		now := common.GetTimestamp()
		newRefundQuota := ext.RefundQuota + row.RequestQuota
		status := topUp.Status
		if newRefundQuota >= ext.FaceQuota() {
			status = TopUpStatusRefunded
		}
		if err := tx.Model(&TopUp{}).Where("id = ?", topUp.Id).Update("status", status).Error; err != nil {
			return err
		}
		if err := tx.Model(&AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(map[string]any{
			"refunded_at":   now,
			"refund_reason": row.Reason,
			"refund_quota":  newRefundQuota,
			"refund_amount": ext.RefundAmount.Add(row.RequestUSD),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(row).Updates(map[string]any{
			"status":                AdtyManualRefundStatusCompleted,
			"bonus_deducted_quota":  bonusTaken,
			"bonus_shortfall_quota": bonusShortfall,
			"complete_operator_id":  operatorID,
			"completed_at":          now,
			"complete_idem_key":     idemKey,
		}).Error; err != nil {
			return err
		}
		row.Status = AdtyManualRefundStatusCompleted
		row.BonusDeductedQuota, row.BonusShortfallQuota = bonusTaken, bonusShortfall
		row.CompleteOperatorId, row.CompletedAt, row.CompleteIdemKey = operatorID, now, idemKey
		topUp.Status = status
		ext.RefundedAt, ext.RefundReason = now, row.Reason
		ext.RefundQuota, ext.RefundAmount = newRefundQuota, ext.RefundAmount.Add(row.RequestUSD)
		ext.RefundDeductedQuota = row.DeductedPrincipalQuota + bonusTaken
		ext.RefundBonusQuota = bonusTaken
		if AdtyAfterTopUpRefundedTx != nil {
			if err := AdtyAfterTopUpRefundedTx(tx, topUp, ext); err != nil {
				return err
			}
		}
		userID, deducted = topUp.UserId, bonusTaken
		committedTopUp, committedExt = topUp, ext
		for i := range rows {
			if rows[i].RefundId == row.RefundId {
				rows[i] = *row
			}
		}
		held, unknown = adtySumManualRefundOccupancy(rows)
		result = adtyManualRefundView(row, adtyBuildManualRefundOccupancy(ext, held, unknown), ledger)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !replay && deducted > 0 {
		if err := invalidateUserCache(userID); err != nil {
			common.SysError(common.LogText("refund cache update failed user_id=%d: %s", userID, err.Error()))
		}
	}
	if !replay && AdtyAfterTopUpRefundCommitted != nil && committedTopUp != nil {
		AdtyAfterTopUpRefundCommitted(committedTopUp, committedExt)
	}
	return result, attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero)
}

// TerminateAdtyTopUpManualRefund formally ends a refund after payout failure
// (or before payout confirmation) and releases H/U. A prior internal reclaim is
// left in place; this does not invent a reversal credit.
func TerminateAdtyTopUpManualRefund(tradeNo, refundID string, operatorID int, idemKey, note string) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	idemKey, err = adtyNormalizeStepIdemKey(refundID, AdtyManualRefundStepTerminate, idemKey)
	if err != nil {
		return nil, err
	}
	var result *AdtyManualRefundResult
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		row, rows, err := lockAdtyManualRefundByIDTx(tx, tradeNo, refundID)
		if err != nil {
			return err
		}
		held, unknown := adtySumManualRefundOccupancy(rows)
		if row.Status == AdtyManualRefundStatusTerminated {
			result = adtyManualRefundView(row, adtyBuildManualRefundOccupancy(ext, held, unknown), nil)
			return nil
		}
		if row.Status == AdtyManualRefundStatusCompleted {
			return common.NewMessage("This refund is already complete")
		}
		if row.PayoutStatus == AdtyManualRefundPayoutSuccess {
			return common.NewMessage("A confirmed offline payout cannot be terminated")
		}
		if row.PayoutRecordedAt > 0 && row.PayoutStatus != AdtyManualRefundPayoutFailed && row.PayoutStatus != AdtyManualRefundPayoutUnknown {
			return common.NewMessage("This refund cannot be terminated")
		}
		now := common.GetTimestamp()
		if err := tx.Model(row).Updates(map[string]any{
			"status":                AdtyManualRefundStatusTerminated,
			"terminate_operator_id": operatorID,
			"terminated_at":         now,
			"terminate_idem_key":    idemKey,
			"terminate_note":        strings.TrimSpace(note),
		}).Error; err != nil {
			return err
		}
		row.Status = AdtyManualRefundStatusTerminated
		row.TerminateOperatorId, row.TerminatedAt, row.TerminateIdemKey = operatorID, now, idemKey
		row.TerminateNote = strings.TrimSpace(note)
		for i := range rows {
			if rows[i].RefundId == row.RefundId {
				rows[i] = *row
			}
		}
		held, unknown = adtySumManualRefundOccupancy(rows)
		result = adtyManualRefundView(row, adtyBuildManualRefundOccupancy(ext, held, unknown), nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, attachAdtyManualRefundFacts(result, tradeNo, decimal.Zero)
}

// ListAdtyTopUpManualRefunds returns occupancy and all machine rows for an order.
func ListAdtyTopUpManualRefunds(tradeNo string) (*AdtyManualRefundResult, error) {
	facts, err := LoadAdtyRefundPageFacts(tradeNo, decimal.Zero)
	if err != nil {
		return nil, err
	}
	return &AdtyManualRefundResult{Occupancy: facts.Occupancy, Facts: facts, Commission: facts.Commission}, nil
}

func GetAdtyTopUpManualRefund(tradeNo, refundID string) (*AdtyManualRefundResult, error) {
	refundID, err := adtyNormalizeRefundID(refundID)
	if err != nil {
		return nil, err
	}
	if err := adtyRequireManualRefundTable(DB); err != nil {
		return nil, err
	}
	var row AdtyManualRefund
	if err := DB.Where("trade_no = ? AND refund_id = ?", tradeNo, refundID).First(&row).Error; err != nil {
		return nil, err
	}
	facts, err := LoadAdtyRefundPageFacts(tradeNo, decimal.Zero)
	if err != nil {
		return nil, err
	}
	return &AdtyManualRefundResult{Refund: &row, Occupancy: facts.Occupancy, Facts: facts, Commission: facts.Commission}, nil
}

// RecordAdtyRefundProof stores a voucher on the original order only. It does
// not record an offline payout step or complete a manual refund.
func RecordAdtyRefundProof(tradeNo, proof string) error {
	proof = strings.TrimSpace(proof)
	if tradeNo == "" || proof == "" || len(proof) > 255 {
		return common.NewMessage("Invalid parameters")
	}
	return DB.Model(&AdtyTopupOrder{}).Where("trade_no = ?", tradeNo).Update("refund_proof", proof).Error
}
