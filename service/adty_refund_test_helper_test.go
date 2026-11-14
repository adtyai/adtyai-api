package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

func completeManualRefundForTest(t *testing.T, tradeNo, reason string, operator int, refundID string, requestUSD decimal.Decimal) (*model.AdtyManualRefundResult, error) {
	t.Helper()
	if _, err := model.ApplyAdtyTopUpManualRefund(tradeNo, reason, operator, refundID, requestUSD); err != nil {
		return nil, err
	}
	if _, err := model.ApproveAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":approve", ""); err != nil {
		return nil, err
	}
	if _, err := model.ReclaimAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":reclaim"); err != nil {
		return nil, err
	}
	if _, err := model.PlanAdtyTopUpManualRefundBonus(tradeNo, refundID, operator, refundID+":bonus"); err != nil {
		return nil, err
	}
	if _, err := model.PlanAdtyTopUpManualRefundCommission(tradeNo, refundID, operator, refundID+":commission"); err != nil {
		return nil, err
	}
	if _, err := model.RecordAdtyTopUpManualRefundPayout(tradeNo, refundID, operator, refundID+":payout", model.AdtyManualRefundPayout{
		Status:   model.AdtyManualRefundPayoutSuccess,
		Currency: "USDT",
		Amount:   requestUSD,
		PaidAt:   1,
		Voucher:  "voucher-" + refundID,
	}); err != nil {
		return nil, err
	}
	return model.CompleteAdtyTopUpManualRefund(tradeNo, refundID, operator, refundID+":complete")
}

// refundAdtyTopUp completes the official manual refund for the previewed
// principal amount. A zero preview is not treated as a finished refund.
func refundAdtyTopUp(t *testing.T, tradeNo, reason string, operator int, key string) (*model.AdtyManualRefundResult, error) {
	t.Helper()
	expected := int64(0)
	if preview, err := model.PreviewAdtyTopUpRefund(tradeNo); err == nil {
		expected = preview.RefundQuota
	}
	requestUSD, err := model.RefundQuotaToUSD(expected)
	if err != nil {
		return nil, err
	}
	return completeManualRefundForTest(t, tradeNo, reason, operator, key, requestUSD)
}
