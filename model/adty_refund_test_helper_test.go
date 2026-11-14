package model

import (
	"testing"

	"github.com/shopspring/decimal"
)

// completeManualRefundForTest walks the official apply / approve / reclaim /
// bonus / commission / payout / complete machine. It does not use the retired
// one-shot RefundAdtyTopUp completion path.
func completeManualRefundForTest(t *testing.T, tradeNo, reason string, operator int, refundID string, requestUSD decimal.Decimal) (*AdtyManualRefundResult, error) {
	t.Helper()
	if _, err := ApplyAdtyTopUpManualRefund(tradeNo, reason, operator, refundID, requestUSD); err != nil {
		return nil, err
	}
	if _, err := ApproveAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":approve", ""); err != nil {
		return nil, err
	}
	if _, err := ReclaimAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":reclaim"); err != nil {
		return nil, err
	}
	if _, err := PlanAdtyTopUpManualRefundBonus(tradeNo, refundID, operator, refundID+":bonus"); err != nil {
		return nil, err
	}
	if _, err := PlanAdtyTopUpManualRefundCommission(tradeNo, refundID, operator, refundID+":commission"); err != nil {
		return nil, err
	}
	if _, err := RecordAdtyTopUpManualRefundPayout(tradeNo, refundID, operator, refundID+":payout", AdtyManualRefundPayout{
		Status:   AdtyManualRefundPayoutSuccess,
		Currency: "USDT",
		Amount:   requestUSD,
		PaidAt:   1,
		Voucher:  "voucher-" + refundID,
	}); err != nil {
		return nil, err
	}
	return CompleteAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":complete")
}

// refundAdtyTopUp completes the official manual refund for the previewed
// principal amount. A zero preview is not treated as a finished refund.
func refundAdtyTopUp(t *testing.T, tradeNo, reason string, operator int, key string) (*AdtyManualRefundResult, error) {
	t.Helper()
	expected := int64(0)
	if preview, err := PreviewAdtyTopUpRefund(tradeNo); err == nil {
		expected = preview.RefundQuota
	}
	requestUSD, err := RefundQuotaToUSD(expected)
	if err != nil {
		return nil, err
	}
	return completeManualRefundForTest(t, tradeNo, reason, operator, key, requestUSD)
}
