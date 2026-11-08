package model

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var paymentReversalAmountPattern = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,8})?$`)

type PaymentOrphanReviewRequest struct {
	Status           string `json:"status"`
	TradeNo          string `json:"trade_no"`
	ExternalAmount   string `json:"external_amount"`
	ExternalCurrency string `json:"external_currency"`
	Proof            string `json:"proof"`
	Note             string `json:"note"`
	DismissalReason  string `json:"dismissal_reason"`
}

// PaymentOrphanReview is an append-only financial evidence snapshot. Amounts
// are cumulative for one payment, never increments to sum across notifications.
// A pending review records facts without authorizing a partial wallet clawback.
type PaymentOrphanReview struct {
	ID               int64           `json:"id"`
	OrphanID         int64           `json:"orphan_id" gorm:"index"`
	OperatorID       int             `json:"operator_id"`
	CreatedAt        int64           `json:"created_at" gorm:"autoCreateTime"`
	Status           string          `json:"status" gorm:"type:varchar(32)"`
	Provider         string          `json:"provider" gorm:"type:varchar(32)"`
	EventID          string          `json:"event_id" gorm:"type:varchar(128)"`
	EventType        string          `json:"event_type" gorm:"type:varchar(128)"`
	SessionID        string          `json:"session_id" gorm:"type:varchar(128)"`
	PayloadHash      string          `json:"payload_hash" gorm:"type:varchar(64)"`
	TradeNo          string          `json:"trade_no" gorm:"type:varchar(255);index"`
	ExternalAmount   decimal.Decimal `json:"external_amount" gorm:"type:decimal(20,8);not null;default:0"`
	ExternalCurrency string          `json:"external_currency" gorm:"type:varchar(16)"`
	Proof            string          `json:"proof" gorm:"type:varchar(512)"`
	Note             string          `json:"note" gorm:"type:text"`
	DismissalReason  string          `json:"dismissal_reason" gorm:"type:varchar(32)"`
	RefundLedgerID   int64           `json:"refund_ledger_id"`
	DeductedQuota    int64           `json:"deducted_quota"`
	RefundQuota      int64           `json:"refund_quota"`
	RefundAmount     decimal.Decimal `json:"refund_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PaidAmount       decimal.Decimal `json:"paid_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PaidCurrency     string          `json:"paid_currency" gorm:"type:varchar(16)"`
	ExposureAmount   decimal.Decimal `json:"exposure_amount" gorm:"type:decimal(20,8);not null;default:0"`
	Request          string          `json:"-" gorm:"type:text"`
}

func (PaymentOrphanReview) TableName() string { return "adty_payment_orphan_reviews" }

// ResolvePaymentOrphanReview never changes a wallet. A refunded disposition
// requires the existing authorized refund flow and its same-order ledger.
func ResolvePaymentOrphanReview(id int64, operatorID int, request PaymentOrphanReviewRequest) (*PaymentOrphanReview, error) {
	request.Status = strings.TrimSpace(request.Status)
	request.TradeNo = strings.TrimSpace(request.TradeNo)
	request.ExternalCurrency = strings.ToUpper(strings.TrimSpace(request.ExternalCurrency))
	request.ExternalAmount = strings.TrimSpace(request.ExternalAmount)
	request.Proof, request.Note = strings.TrimSpace(request.Proof), strings.TrimSpace(request.Note)
	request.DismissalReason = strings.TrimSpace(request.DismissalReason)
	if operatorID <= 0 || request.TradeNo == "" || len(request.TradeNo) > 255 || request.Proof == "" || len(request.Proof) > 512 || request.Note == "" || len(request.Note) > 2000 {
		return nil, common.NewMessage("Order number, proof and note are required")
	}
	amount := decimal.Zero
	if request.Status == PaymentOrphanStatusDismissed {
		if request.DismissalReason != "not_this_order" && request.DismissalReason != "reversal_canceled" {
			return nil, common.NewMessage("Confirm why this event does not block payment")
		}
		request.ExternalAmount, request.ExternalCurrency = "0", ""
	} else {
		var err error
		if !paymentReversalAmountPattern.MatchString(request.ExternalAmount) {
			return nil, common.NewMessage("Invalid external reversal amount or currency")
		}
		amount, err = decimal.NewFromString(request.ExternalAmount)
		if err != nil || amount.IsNegative() || amount.GreaterThan(decimal.New(1, 12).Sub(decimal.New(1, -8))) || !amount.Equal(amount.Truncate(8)) || len(request.ExternalCurrency) < 3 || len(request.ExternalCurrency) > 16 || request.DismissalReason != "" {
			return nil, common.NewMessage("Invalid external reversal amount or currency")
		}
		for _, letter := range request.ExternalCurrency {
			if letter < 'A' || letter > 'Z' {
				return nil, common.NewMessage("Invalid external reversal amount or currency")
			}
		}
		request.ExternalAmount = amount.String()
	}
	switch request.Status {
	case PaymentOrphanStatusPendingReview, PaymentOrphanStatusDismissed:
	case PaymentOrphanStatusRefunded, PaymentOrphanStatusNeverCredited:
		if !amount.IsPositive() {
			return nil, common.NewMessage("A confirmed external reversal amount is required")
		}
	default:
		return nil, ErrPaymentOrphanNotCredit
	}
	canonical, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	var result *PaymentOrphanReview
	err = DB.Transaction(func(tx *gorm.DB) error {
		// The same lock order as callbacks, queries and refunds prevents a
		// never-credited conclusion racing a successful payment.
		orphan, topUp, ext, err := LockPaymentOrphanOrderTx(tx, id)
		if err != nil {
			return err
		}
		if request.TradeNo != orphan.ReferenceID {
			return common.NewMessage("The review must reference the original order")
		}
		if orphan.ReviewID > 0 {
			var previous PaymentOrphanReview
			if err := tx.First(&previous, orphan.ReviewID).Error; err != nil {
				return err
			}
			if previous.Request == string(canonical) {
				result = &previous
				return nil
			}
		}
		if orphan.Status != PaymentOrphanStatusPendingReview {
			return ErrAdtyIdempotencyConflict
		}
		if topUp != nil && (topUp.PaymentProvider != orphan.Provider || (ext != nil && (ext.Provider != orphan.Provider || ext.UserId != topUp.UserId))) {
			return ErrPaymentMethodMismatch
		}
		review := &PaymentOrphanReview{
			OrphanID: id, OperatorID: operatorID, Status: request.Status, TradeNo: request.TradeNo,
			ExternalAmount: amount, ExternalCurrency: request.ExternalCurrency, Proof: request.Proof, Note: request.Note,
			DismissalReason: request.DismissalReason, Request: string(canonical),
			Provider: orphan.Provider, EventID: orphan.EventID, EventType: orphan.EventType, SessionID: orphan.SessionID,
		}
		payloadHash := sha256.Sum256([]byte(orphan.Payload))
		review.PayloadHash = hex.EncodeToString(payloadHash[:])
		if ext != nil {
			review.PaidAmount, review.PaidCurrency = ext.PaidAmount, ext.PaidCurrency
		}
		switch request.Status {
		case PaymentOrphanStatusRefunded:
			if topUp == nil || ext == nil || topUp.Status != TopUpStatusRefunded || !ext.PaidAmount.IsPositive() || !strings.EqualFold(ext.PaidCurrency, request.ExternalCurrency) || !amount.Equal(ext.PaidAmount) {
				return common.NewMessage("A full external reversal and the original wallet refund are required")
			}
			var ledgers []AdtyBalanceLedger
			if err := tx.Where("type = ? AND ref_type = ? AND ref_id = ? AND user_id = ?", AdtyLedgerTypeRefund, "topup", topUp.TradeNo, topUp.UserId).Limit(2).Find(&ledgers).Error; err != nil {
				return err
			}
			if len(ledgers) != 1 || ledgers[0].DeltaQuota > 0 || ledgers[0].DeltaQuota < -int64(common.MaxWalletQuota) || -ledgers[0].DeltaQuota > ext.CreditQuota || ext.RefundQuota < 0 || ext.RefundQuota > ext.FaceQuota() || ext.RefundQuota > -ledgers[0].DeltaQuota || !ext.RefundAmount.Equal(adtyRefundAmount(ext, ext.RefundQuota, ext.FaceQuota())) {
				return common.NewMessage("The original wallet refund ledger is required")
			}
			review.RefundLedgerID, review.DeductedQuota = ledgers[0].Id, -ledgers[0].DeltaQuota
			review.RefundQuota, review.RefundAmount = ext.RefundQuota, ext.RefundAmount
			review.ExposureAmount = decimal.Max(amount.Sub(ext.RefundAmount), decimal.Zero)
		case PaymentOrphanStatusNeverCredited:
			if topUp == nil || ext == nil || topUp.CompleteTime != 0 || ext.PaidSource != "" || ext.PaidAmount.IsPositive() || topUp.Status == common.TopUpStatusSuccess || topUp.Status == TopUpStatusRefunded {
				return common.NewMessage("The original order must never have been credited")
			}
			switch topUp.Status {
			case common.TopUpStatusPending, common.TopUpStatusFailed, common.TopUpStatusExpired, TopUpStatusCanceled:
			default:
				return common.NewMessage("The original order must never have been credited")
			}
			var credits int64
			if err := tx.Model(&AdtyBalanceLedger{}).Where("ref_type = ? AND ref_id = ? AND type = ?", "topup", topUp.TradeNo, AdtyLedgerTypeTopUp).Count(&credits).Error; err != nil {
				return err
			}
			if credits != 0 {
				return common.NewMessage("The original order must never have been credited")
			}
			if !strings.EqualFold(ext.PayCurrency, request.ExternalCurrency) {
				return common.NewMessage("The external reversal currency must match the original order")
			}
		}
		if err := tx.Create(review).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		resolvedAt := now
		if request.Status == PaymentOrphanStatusPendingReview {
			resolvedAt = 0
		}
		if err := tx.Model(&PaymentOrphanEvent{}).Where("id = ?", id).Updates(map[string]any{
			"status": request.Status, "resolved_by": operatorID, "resolved_at": resolvedAt,
			"resolution": request.Status, "resolution_note": request.Note, "review_id": review.ID, "update_time": now,
		}).Error; err != nil {
			return err
		}
		result = review
		return nil
	})
	return result, err
}
