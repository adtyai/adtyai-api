package service

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/shopspring/decimal"
)

// Verification scopes for admin top-up fund writes. These always require
// step-up; there is no amount threshold.
const (
	VerificationScopeAdtyTopUpRefund           = "adty.topup.refund"
	VerificationScopeAdtyTopUpManualComplete   = "adty.topup.manual_complete"
	VerificationScopeAdtyTopUpManualRefundStep = "adty.topup.manual_refund_step"
	VerificationScopeAdtyPaymentOrphanCredit   = "adty.payment_orphan.credit"
	VerificationScopeAdtyPaymentOrphanResolve  = "adty.payment_orphan.resolve"
	VerificationScopeAdtyTopUpQuery            = "adty.topup.query"
)

const (
	adtyTopUpTradeNoMaxLen = 255
	adtyTopUpTextMaxLen    = 1024
)

// AdtyTopUpRefundContext binds a refund proof to one order and the exact
// refund request (amount or quota, plus reason). A proof issued for another
// trade_no, amount, quota, or reason is rejected.
type AdtyTopUpRefundContext struct {
	TradeNo     string `json:"trade_no"`
	Reason      string `json:"reason"`
	RefundQuota *int64 `json:"refund_quota,omitempty"`
	RefundUSD   string `json:"refund_usd,omitempty"`
}

// AdtyTopUpManualCompleteContext binds a manual-credit proof to one order and
// the payment-voucher evidence plus note. The JSON "proof" field is voucher
// evidence, not the X-Security-Proof header.
type AdtyTopUpManualCompleteContext struct {
	TradeNo string `json:"trade_no"`
	Proof   string `json:"proof"`
	Note    string `json:"note"`
}

// AdtyTopUpManualRefundStepContext binds a step-up proof to one manual refund
// step. Payout proofs also bind the recorded currency, amount, voucher, time,
// and status.
type AdtyTopUpManualRefundStepContext struct {
	TradeNo        string `json:"trade_no"`
	RefundId       string `json:"refund_id"`
	Step           string `json:"step"`
	Note           string `json:"note,omitempty"`
	PayoutStatus   string `json:"payout_status,omitempty"`
	PayoutCurrency string `json:"payout_currency,omitempty"`
	PayoutAmount   string `json:"payout_amount,omitempty"`
	PayoutVoucher  string `json:"payout_voucher,omitempty"`
	PayoutAt       int64  `json:"payout_at,omitempty"`
}

// AdtyPaymentOrphanCreditContext binds a credit proof to one orphan and note.
type AdtyPaymentOrphanCreditContext struct {
	ID   int64  `json:"id"`
	Note string `json:"note,omitempty"`
}

// AdtyPaymentOrphanResolveContext binds a resolve proof to one orphan review.
type AdtyPaymentOrphanResolveContext struct {
	ID               int64  `json:"id"`
	Status           string `json:"status"`
	TradeNo          string `json:"trade_no,omitempty"`
	ExternalAmount   string `json:"external_amount,omitempty"`
	ExternalCurrency string `json:"external_currency,omitempty"`
	Proof            string `json:"proof,omitempty"`
	Note             string `json:"note,omitempty"`
	DismissalReason  string `json:"dismissal_reason,omitempty"`
}

// AdtyTopUpQueryContext binds a provider-query proof to one trade_no. The
// query may credit a verified payment, so it is always step-up gated.
type AdtyTopUpQueryContext struct {
	TradeNo string `json:"trade_no"`
}

var adtyTopUpVerificationPermissions = map[string]authz.Permission{
	VerificationScopeAdtyTopUpRefund:           authz.AdtyFinanceRefund,
	VerificationScopeAdtyTopUpManualComplete:   authz.AdtyFinanceManualCredit,
	VerificationScopeAdtyTopUpManualRefundStep: authz.AdtyFinanceRefund,
	VerificationScopeAdtyPaymentOrphanCredit:   authz.AdtyFinanceManualCredit,
	VerificationScopeAdtyPaymentOrphanResolve:  authz.AdtyFinanceManualCredit,
	VerificationScopeAdtyTopUpQuery:            authz.AdtyFinanceManualCredit,
}

func adtyIsTopUpVerificationScope(scope string) bool {
	_, ok := adtyTopUpVerificationPermissions[scope]
	return ok
}

func adtyTopUpVerificationForbidden(identity AuthIdentity, role int, scope string) bool {
	permission, ok := adtyTopUpVerificationPermissions[scope]
	if !ok {
		return false
	}
	return role < common.RoleAdminUser || !AdtyAdminRoleCan(identity.UserID, role, permission)
}

// adtyBindTopUpOperation validates and normalizes a finance write context
// for BindVerificationOperation.
func adtyBindTopUpOperation(scope string, fields map[string]json.RawMessage, raw json.RawMessage) (any, bool) {
	switch scope {
	case VerificationScopeAdtyTopUpRefund:
		var context AdtyTopUpRefundContext
		if common.Unmarshal(raw, &context) != nil || fields["trade_no"] == nil {
			return nil, false
		}
		for key := range fields {
			switch key {
			case "trade_no", "reason", "refund_quota", "refund_usd":
			default:
				return nil, false
			}
		}
		context.TradeNo = strings.TrimSpace(context.TradeNo)
		context.Reason = strings.TrimSpace(context.Reason)
		if context.TradeNo == "" || len(context.TradeNo) > adtyTopUpTradeNoMaxLen || len(context.Reason) > adtyTopUpTextMaxLen {
			return nil, false
		}
		hasQuota := fields["refund_quota"] != nil
		hasUSD := fields["refund_usd"] != nil
		if hasQuota == hasUSD {
			return nil, false
		}
		if hasUSD {
			amount, err := decimal.NewFromString(strings.TrimSpace(context.RefundUSD))
			if err != nil || !amount.IsPositive() {
				return nil, false
			}
			context.RefundUSD = amount.String()
			context.RefundQuota = nil
		} else if context.RefundQuota == nil {
			return nil, false
		} else {
			context.RefundUSD = ""
		}
		return context, true
	case VerificationScopeAdtyTopUpManualComplete:
		var context AdtyTopUpManualCompleteContext
		if len(fields) != 3 || fields["trade_no"] == nil || fields["proof"] == nil || fields["note"] == nil || common.Unmarshal(raw, &context) != nil {
			return nil, false
		}
		context.TradeNo = strings.TrimSpace(context.TradeNo)
		context.Proof = strings.TrimSpace(context.Proof)
		context.Note = strings.TrimSpace(context.Note)
		if context.TradeNo == "" || len(context.TradeNo) > adtyTopUpTradeNoMaxLen ||
			context.Proof == "" || len(context.Proof) > adtyTopUpTextMaxLen ||
			context.Note == "" || len(context.Note) > adtyTopUpTextMaxLen {
			return nil, false
		}
		return context, true
	case VerificationScopeAdtyTopUpManualRefundStep:
		var context AdtyTopUpManualRefundStepContext
		if common.Unmarshal(raw, &context) != nil || fields["trade_no"] == nil || fields["refund_id"] == nil || fields["step"] == nil {
			return nil, false
		}
		allowed := map[string]struct{}{
			"trade_no": {}, "refund_id": {}, "step": {}, "note": {},
			"payout_status": {}, "payout_currency": {}, "payout_amount": {},
			"payout_voucher": {}, "payout_at": {},
		}
		for key := range fields {
			if _, ok := allowed[key]; !ok {
				return nil, false
			}
		}
		context.TradeNo = strings.TrimSpace(context.TradeNo)
		context.RefundId = strings.TrimSpace(context.RefundId)
		context.Step = strings.TrimSpace(context.Step)
		context.Note = strings.TrimSpace(context.Note)
		context.PayoutStatus = strings.TrimSpace(context.PayoutStatus)
		context.PayoutCurrency = strings.TrimSpace(context.PayoutCurrency)
		context.PayoutVoucher = strings.TrimSpace(context.PayoutVoucher)
		if context.TradeNo == "" || len(context.TradeNo) > adtyTopUpTradeNoMaxLen ||
			context.RefundId == "" || len(context.RefundId) > 64 ||
			len(context.Note) > adtyTopUpTextMaxLen ||
			len(context.PayoutCurrency) > 16 || len(context.PayoutVoucher) > 255 {
			return nil, false
		}
		switch context.Step {
		case "approve", "reclaim", "bonus", "commission", "payout", "complete", "terminate":
		default:
			return nil, false
		}
		if context.PayoutAmount != "" {
			amount, err := decimal.NewFromString(strings.TrimSpace(context.PayoutAmount))
			if err != nil {
				return nil, false
			}
			context.PayoutAmount = amount.String()
		}
		return context, true
	case VerificationScopeAdtyPaymentOrphanCredit:
		var context AdtyPaymentOrphanCreditContext
		if common.Unmarshal(raw, &context) != nil || fields["id"] == nil {
			return nil, false
		}
		for key := range fields {
			switch key {
			case "id", "note":
			default:
				return nil, false
			}
		}
		context.Note = strings.TrimSpace(context.Note)
		if context.ID <= 0 || len(context.Note) > adtyTopUpTextMaxLen {
			return nil, false
		}
		return context, true
	case VerificationScopeAdtyPaymentOrphanResolve:
		var context AdtyPaymentOrphanResolveContext
		if common.Unmarshal(raw, &context) != nil || fields["id"] == nil || fields["status"] == nil {
			return nil, false
		}
		allowed := map[string]struct{}{
			"id": {}, "status": {}, "trade_no": {}, "external_amount": {},
			"external_currency": {}, "proof": {}, "note": {}, "dismissal_reason": {},
		}
		for key := range fields {
			if _, ok := allowed[key]; !ok {
				return nil, false
			}
		}
		context.Status = strings.TrimSpace(context.Status)
		context.TradeNo = strings.TrimSpace(context.TradeNo)
		context.ExternalAmount = strings.TrimSpace(context.ExternalAmount)
		context.ExternalCurrency = strings.ToUpper(strings.TrimSpace(context.ExternalCurrency))
		context.Proof = strings.TrimSpace(context.Proof)
		context.Note = strings.TrimSpace(context.Note)
		context.DismissalReason = strings.TrimSpace(context.DismissalReason)
		if context.ID <= 0 || context.Status == "" ||
			len(context.TradeNo) > adtyTopUpTradeNoMaxLen ||
			len(context.Proof) > 512 || len(context.Note) > 2000 ||
			len(context.DismissalReason) > adtyTopUpTextMaxLen ||
			len(context.ExternalCurrency) > 16 {
			return nil, false
		}
		if context.ExternalAmount != "" {
			amount, err := decimal.NewFromString(context.ExternalAmount)
			if err != nil {
				return nil, false
			}
			context.ExternalAmount = amount.String()
		}
		return context, true
	case VerificationScopeAdtyTopUpQuery:
		var context AdtyTopUpQueryContext
		if len(fields) != 1 || fields["trade_no"] == nil || common.Unmarshal(raw, &context) != nil {
			return nil, false
		}
		context.TradeNo = strings.TrimSpace(context.TradeNo)
		if context.TradeNo == "" || len(context.TradeNo) > adtyTopUpTradeNoMaxLen {
			return nil, false
		}
		return context, true
	default:
		return nil, false
	}
}

// AdtyTopUpRefundOperation builds the step-up operation for an admin refund.
func AdtyTopUpRefundOperation(tradeNo, reason, refundUSD string, refundQuota *int64) VerificationOperation {
	payload, _ := common.Marshal(AdtyTopUpRefundContext{
		TradeNo: tradeNo, Reason: reason, RefundUSD: refundUSD, RefundQuota: refundQuota,
	})
	return VerificationOperation{Scope: VerificationScopeAdtyTopUpRefund, Context: payload}
}

// AdtyTopUpManualCompleteOperation builds the step-up operation for a manual
// top-up credit. proof is the payment-voucher evidence string.
func AdtyTopUpManualCompleteOperation(tradeNo, proof, note string) VerificationOperation {
	payload, _ := common.Marshal(AdtyTopUpManualCompleteContext{TradeNo: tradeNo, Proof: proof, Note: note})
	return VerificationOperation{Scope: VerificationScopeAdtyTopUpManualComplete, Context: payload}
}

// AdtyManualRefundStepPayout is the payout evidence bound into a step-up proof.
type AdtyManualRefundStepPayout struct {
	Status   string
	Currency string
	Amount   string
	Voucher  string
	PaidAt   int64
}

// AdtyTopUpManualRefundStepOperation builds the step-up operation for one
// manual refund state-machine step.
func AdtyTopUpManualRefundStepOperation(tradeNo, refundID, step, note string, payout AdtyManualRefundStepPayout) VerificationOperation {
	payload, _ := common.Marshal(AdtyTopUpManualRefundStepContext{
		TradeNo:        tradeNo,
		RefundId:       refundID,
		Step:           step,
		Note:           note,
		PayoutStatus:   payout.Status,
		PayoutCurrency: payout.Currency,
		PayoutAmount:   strings.TrimSpace(payout.Amount),
		PayoutVoucher:  payout.Voucher,
		PayoutAt:       payout.PaidAt,
	})
	return VerificationOperation{Scope: VerificationScopeAdtyTopUpManualRefundStep, Context: payload}
}

// AdtyPaymentOrphanCreditOperation builds the step-up operation for crediting
// a payment orphan.
func AdtyPaymentOrphanCreditOperation(id int64, note string) VerificationOperation {
	payload, _ := common.Marshal(AdtyPaymentOrphanCreditContext{ID: id, Note: note})
	return VerificationOperation{Scope: VerificationScopeAdtyPaymentOrphanCredit, Context: payload}
}

// AdtyPaymentOrphanResolveOperation builds the step-up operation for one
// payment-orphan review request.
func AdtyPaymentOrphanResolveOperation(id int64, status, tradeNo, externalAmount, externalCurrency, proof, note, dismissalReason string) VerificationOperation {
	payload, _ := common.Marshal(AdtyPaymentOrphanResolveContext{
		ID: id, Status: status, TradeNo: tradeNo, ExternalAmount: externalAmount,
		ExternalCurrency: externalCurrency, Proof: proof, Note: note, DismissalReason: dismissalReason,
	})
	return VerificationOperation{Scope: VerificationScopeAdtyPaymentOrphanResolve, Context: payload}
}

// AdtyTopUpQueryOperation builds the step-up operation for a provider query
// that may credit a verified payment.
func AdtyTopUpQueryOperation(tradeNo string) VerificationOperation {
	payload, _ := common.Marshal(AdtyTopUpQueryContext{TradeNo: tradeNo})
	return VerificationOperation{Scope: VerificationScopeAdtyTopUpQuery, Context: payload}
}

func init() {
	accessTokenVerificationScopes[VerificationScopeAdtyTopUpRefund] = AccessTokenScopeOf(authz.AdtyFinanceRefund)
	accessTokenVerificationScopes[VerificationScopeAdtyTopUpManualComplete] = AccessTokenScopeOf(authz.AdtyFinanceManualCredit)
	accessTokenVerificationScopes[VerificationScopeAdtyTopUpManualRefundStep] = AccessTokenScopeOf(authz.AdtyFinanceRefund)
	accessTokenVerificationScopes[VerificationScopeAdtyPaymentOrphanCredit] = AccessTokenScopeOf(authz.AdtyFinanceManualCredit)
	accessTokenVerificationScopes[VerificationScopeAdtyPaymentOrphanResolve] = AccessTokenScopeOf(authz.AdtyFinanceManualCredit)
	accessTokenVerificationScopes[VerificationScopeAdtyTopUpQuery] = AccessTokenScopeOf(authz.AdtyFinanceManualCredit)
}
