package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// CumulativeGiftReclaim is this refund's gift recovery:
// gift * min(cumulativeRefundedPrincipal / originalPrincipal, 1) minus earlier recoveries.
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
// The amount cannot exceed the order principal still available to refund.
func ManualRefundUSDQuota(faceUSD, alreadyRefundedUSD, requestUSD decimal.Decimal) (int64, error) {
	if !requestUSD.IsPositive() {
		return 0, common.NewMessage("Refund amount must be positive")
	}
	remaining := faceUSD.Sub(alreadyRefundedUSD)
	if requestUSD.GreaterThan(remaining) {
		return 0, common.NewMessage("Refund amount exceeds the remaining principal")
	}
	quota, err := common.WalletQuotaFromDecimalStrict(requestUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil {
		return 0, err
	}
	return int64(quota), nil
}

// AdtyRefundPageFacts is the recorded refund-page check. Amounts come from the
// order and its fund lots. A blank lot field means that lot was not recorded.
type AdtyRefundPageFacts struct {
	TradeNo               string `json:"trade_no"`
	SiteId                int    `json:"site_id"`
	UserId                int    `json:"user_id"`
	FaceUSD               string `json:"face_usd"`
	PayCurrency           string `json:"pay_currency"`
	PaidAmount            string `json:"paid_amount"`
	PaidCurrency          string `json:"paid_currency"`
	OrderRate             string `json:"order_rate"`
	PrincipalConsumedUSD  string `json:"principal_consumed_usd,omitempty"`
	PrincipalRemainingUSD string `json:"principal_remaining_usd,omitempty"`
	PendingQuota          int64  `json:"pending_quota"`
	RefundOccupiedQuota   int64  `json:"refund_occupied_quota"`
	BonusUSD              string `json:"bonus_usd"`
	BonusUsedQuota        int64  `json:"bonus_used_quota"`
	BonusRemainingQuota   int64  `json:"bonus_remaining_quota"`
	CumulativeRefundUSD   string `json:"cumulative_refund_usd"`
	ForeignReference      string `json:"foreign_reference,omitempty"`
	RefundProof           string `json:"refund_proof,omitempty"`
}

// LoadAdtyRefundPageFacts reads the order, its fund lots, and the customer's
// in-flight precharge. requestUSD, when positive, adds the foreign reference
// at the stored paid amount. It does not write.
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
	facts := &AdtyRefundPageFacts{
		TradeNo: tradeNo, SiteId: ext.SiteId, UserId: ext.UserId,
		FaceUSD: ext.FaceUSD.String(), PayCurrency: ext.PayCurrency,
		PaidAmount: ext.PaidAmount.String(), PaidCurrency: ext.PaidCurrency,
		OrderRate: ext.PriceSnapshot.String(), PendingQuota: int64(user.QuotaPending),
		RefundOccupiedQuota: ext.RefundQuota, BonusUSD: ext.BonusUSD.String(),
		CumulativeRefundUSD: decimal.NewFromInt(ext.RefundQuota).Div(common.QuotaPerUnitDecimal()).String(),
		RefundProof:         ext.RefundProof,
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
	if requestUSD.IsPositive() {
		alreadyUSD := decimal.NewFromInt(ext.RefundQuota).Div(common.QuotaPerUnitDecimal())
		requestQuota, err := ManualRefundUSDQuota(ext.FaceUSD, alreadyUSD, requestUSD)
		if err != nil {
			return nil, err
		}
		facts.ForeignReference = adtyRefundAmount(&ext, requestQuota, ext.FaceQuota()).String()
	}
	return facts, nil
}

// RecordAdtyRefundProof stores the offline payment voucher for a refund.
func RecordAdtyRefundProof(tradeNo, proof string) error {
	proof = strings.TrimSpace(proof)
	if tradeNo == "" || proof == "" || len(proof) > 255 {
		return common.NewMessage("Invalid parameters")
	}
	return DB.Model(&AdtyTopupOrder{}).Where("trade_no = ?", tradeNo).Update("refund_proof", proof).Error
}

// RefundAdtyTopUpManualUSD records an admin-entered USD refund and reclaims gift by the cumulative ratio.
func RefundAdtyTopUpManualUSD(tradeNo, reason string, operatorID int, idemKey string, requestUSD decimal.Decimal) (*AdtyTopUpRefundResult, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, common.NewMessage("A refund reason is required")
	}
	idemKey = strings.TrimSpace(idemKey)
	if idemKey == "" || len(idemKey) > 64 {
		return nil, common.NewMessage("Invalid idempotency key")
	}
	var result *AdtyTopUpRefundResult
	var userID int
	replay := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp, ext, err := lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		before, checkinQuota, err := lockUserQuotaForRefundTx(tx, topUp.UserId)
		if err != nil {
			return err
		}
		existing, err := FindAdtyLedgerByIdemKeyTx(tx, idemKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Type != AdtyLedgerTypeRefund || existing.RefId != tradeNo {
				return ErrAdtyIdempotencyConflict
			}
			result = &AdtyTopUpRefundResult{
				Ledger: existing, DeductedQuota: -existing.DeltaQuota, BonusQuota: ext.BonusUSD.Mul(common.QuotaPerUnitDecimal()).IntPart(),
				RefundQuota: ext.RefundQuota, RefundAmount: ext.RefundAmount, Currency: ext.refundCurrency(),
			}
			replay = true
			return nil
		}
		if topUp.Status != common.TopUpStatusSuccess && topUp.Status != TopUpStatusRefunded {
			return common.NewMessage("Only credited orders can be refunded")
		}
		alreadyUSD := decimal.NewFromInt(ext.RefundQuota).Div(common.QuotaPerUnitDecimal())
		requestQuota, err := ManualRefundUSDQuota(ext.FaceUSD, alreadyUSD, requestUSD)
		if err != nil {
			return err
		}
		reclaimUSD := CumulativeGiftReclaim(ext.BonusUSD, ext.FaceUSD, alreadyUSD, requestUSD)
		reclaimQuota, err := common.WalletQuotaFromDecimalStrict(reclaimUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
		if err != nil {
			return err
		}
		spendable := before - checkinQuota
		if spendable < 0 {
			spendable = 0
		}
		need := requestQuota + int64(reclaimQuota)
		deducted := need
		if spendable < deducted {
			deducted = spendable
		}
		userID = topUp.UserId
		if deducted > 0 {
			if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota - ?", deducted)).Error; err != nil {
				return err
			}
			if err := clampCheckinQuotaDB(tx, topUp.UserId); err != nil {
				return err
			}
		}
		now := common.GetTimestamp()
		newRefundQuota := ext.RefundQuota + requestQuota
		status := topUp.Status
		faceQuota, err := common.WalletQuotaFromDecimalStrict(ext.FaceUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
		if err != nil {
			return err
		}
		if newRefundQuota >= int64(faceQuota) {
			status = TopUpStatusRefunded
		}
		if err := tx.Model(&TopUp{}).Where("id = ?", topUp.Id).Update("status", status).Error; err != nil {
			return err
		}
		if err := tx.Model(&AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(map[string]any{
			"refunded_at":   now,
			"refund_reason": reason,
			"refund_quota":  newRefundQuota,
			"refund_amount": ext.RefundAmount.Add(requestUSD),
		}).Error; err != nil {
			return err
		}
		key := idemKey
		ledger := &AdtyBalanceLedger{
			UserId: topUp.UserId, DeltaQuota: -deducted, BalanceBefore: before, BalanceAfter: before - deducted,
			Type: AdtyLedgerTypeRefund, RefType: "topup", RefId: tradeNo,
			Reason:         reason + " [manual_usd=" + requestUSD.String() + "]",
			OperatorId:     operatorID,
			IdempotencyKey: &key,
		}
		if err := InsertAdtyLedgerTx(tx, ledger); err != nil {
			return err
		}
		result = &AdtyTopUpRefundResult{
			Ledger: ledger, DeductedQuota: deducted, BonusQuota: int64(reclaimQuota),
			RefundQuota: requestQuota, RefundAmount: requestUSD, Currency: ext.refundCurrency(),
		}
		topUp.Status = status
		ext.RefundQuota, ext.RefundAmount = newRefundQuota, ext.RefundAmount.Add(requestUSD)
		ext.RefundDeductedQuota, ext.RefundBonusQuota = deducted, int64(reclaimQuota)
		if AdtyAfterTopUpRefundedTx != nil {
			return AdtyAfterTopUpRefundedTx(tx, topUp, ext)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !replay && result.DeductedQuota > 0 {
		if err := invalidateUserCache(userID); err != nil {
			common.SysError(common.LogText("refund cache update failed user_id=%d: %s", userID, err.Error()))
		}
	}
	return result, nil
}
