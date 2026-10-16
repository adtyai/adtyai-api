package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 审查 N-P2-8：超时 pending 订单置 expired；USDT 不动；迟到的验签支付仍可入账。
func TestExpirePendingTopupOrders(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TopupOrder{}))
	require.NoError(t, EnsureLedgerSchema(DB))
	t.Cleanup(func() { DB.Exec("DELETE FROM topup_orders") })
	u := createReserveTestUser(t, 0)
	now := time.Now()
	old := now.Add(-2 * time.Hour).Unix()
	rows := []TopupOrder{
		{OrderNo: "EXP-OLD", UserId: u.Id, AmountUSD: 10, CreditAmountUSD: 10, PaymentProvider: "epay", Status: TopupOrderStatusPending, RefundStatus: "none", CreateTime: old},
		{OrderNo: "EXP-NEW", UserId: u.Id, AmountUSD: 10, PaymentProvider: "epay", Status: TopupOrderStatusPending, RefundStatus: "none", CreateTime: now.Unix()},
		{OrderNo: "EXP-USDT", UserId: u.Id, AmountUSD: 10, PaymentProvider: PaymentProviderBepusdt, Status: TopupOrderStatusPending, RefundStatus: "none", CreateTime: old},
	}
	for i := range rows {
		require.NoError(t, DB.Create(&rows[i]).Error)
	}
	n, err := ExpirePendingTopupOrders(now)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	got := map[string]string{}
	var all []TopupOrder
	require.NoError(t, DB.Find(&all).Error)
	for _, o := range all {
		got[o.OrderNo] = o.Status
	}
	require.Equal(t, map[string]string{"EXP-OLD": TopupOrderStatusExpired, "EXP-NEW": TopupOrderStatusPending, "EXP-USDT": TopupOrderStatusPending}, got)

	done, err := CompleteTopupOrder("EXP-OLD")
	require.NoError(t, err)
	require.Equal(t, TopupOrderStatusPaid, done.Status)
}
