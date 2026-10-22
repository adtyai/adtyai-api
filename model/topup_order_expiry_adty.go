package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// TopupOrderStatusExpired 审查 N-P2-8：超时未支付的订单。网关迟到的已验签回调仍可入账（见 CompleteTopupOrderBy）。
const TopupOrderStatusExpired = "expired"

// ExpirePendingTopupOrders 把创建超过 TopupOrderTimeoutMinutes 仍 pending 的非 USDT 订单置为 expired。
// USDT 窗口结束只改展示（usdt_expire_at），不改行状态，也不转人工。迟到的已验签状态 2 仍可入账。
func ExpirePendingTopupOrders(now time.Time) (int64, error) {
	timeout := GetTopupOrderTimeoutMinutes()
	if timeout <= 0 {
		return 0, nil
	}
	cutoff := now.Add(-time.Duration(timeout) * time.Minute).Unix()
	res := DB.Model(&TopupOrder{}).
		Where("status = ? AND create_time > 0 AND create_time < ? AND payment_provider <> ?", TopupOrderStatusPending, cutoff, PaymentProviderBepusdt).
		Update("status", TopupOrderStatusExpired)
	return res.RowsAffected, res.Error
}

// StartTopupOrderExpiryLoop 每分钟清理一次超时订单（仅主节点）。
func StartTopupOrderExpiryLoop() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			if n, err := ExpirePendingTopupOrders(time.Now()); err != nil {
				common.SysError(common.LogText("expire topup orders failed: %s", err.Error()))
			} else if n > 0 {
				common.SysLog(common.LogText("expired %d pending topup orders", n))
			}
		}
	}()
}
