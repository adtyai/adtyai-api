package model

import "testing"

// refundAdtyTopUp 用预览额度退款。预览失败时传 0：状态不对的订单会先被状态检查拒绝。
func refundAdtyTopUp(t *testing.T, tradeNo, reason string, operator int, key string) (*AdtyTopUpRefundResult, error) {
	t.Helper()
	expected := int64(0)
	if preview, err := PreviewAdtyTopUpRefund(tradeNo); err == nil {
		expected = preview.RefundQuota
	}
	return RefundAdtyTopUp(tradeNo, reason, operator, key, expected)
}
