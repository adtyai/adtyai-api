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
	VerificationScopeAdtyTopUpRefund         = "adty.topup.refund"
	VerificationScopeAdtyTopUpManualComplete = "adty.topup.manual_complete"
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

var adtyTopUpVerificationPermissions = map[string]authz.Permission{
	VerificationScopeAdtyTopUpRefund:         authz.AdtyFinanceRefund,
	VerificationScopeAdtyTopUpManualComplete: authz.AdtyFinanceManualCredit,
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

// adtyBindTopUpOperation validates and normalizes a refund or manual-complete
// context for BindVerificationOperation.
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

func init() {
	accessTokenVerificationScopes[VerificationScopeAdtyTopUpRefund] = AccessTokenScopeOf(authz.AdtyFinanceRefund)
	accessTokenVerificationScopes[VerificationScopeAdtyTopUpManualComplete] = AccessTokenScopeOf(authz.AdtyFinanceManualCredit)
}
