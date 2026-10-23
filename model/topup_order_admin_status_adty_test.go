package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListAdminTopupOrdersUsesDisplayExpiry(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TopupOrder{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM topup_orders") })
	user := createReserveTestUser(t, 0)
	const now int64 = 1_700_000_000
	rows := []TopupOrder{
		{OrderNo: "USDT-OPEN", UserId: user.Id, AmountUSD: 10, PaymentMethod: "usdt", PaymentProvider: "bepusdt", Status: TopupOrderStatusPending, UsdtExpireAt: now + 60, RefundStatus: "none"},
		{OrderNo: "USDT-LATE", UserId: user.Id, AmountUSD: 10, PaymentMethod: "USDT", PaymentProvider: "bepusdt", Status: TopupOrderStatusPending, UsdtExpireAt: now, RefundStatus: "none"},
		{OrderNo: "EPAY-EXP", UserId: user.Id, AmountUSD: 10, PaymentMethod: "alipay", PaymentProvider: "epay", Status: TopupOrderStatusExpired, RefundStatus: "none"},
		{OrderNo: "EPAY-PEND", UserId: user.Id, AmountUSD: 10, PaymentMethod: "wxpay", PaymentProvider: "epay", Status: TopupOrderStatusPending, RefundStatus: "none"},
		{OrderNo: "USDT-REVIEW", UserId: user.Id, AmountUSD: 10, PaymentMethod: "usdt", PaymentProvider: "bepusdt", Status: TopupOrderStatusNeedManual, UsdtExpireAt: now - 10, RefundStatus: "none"},
	}
	for i := range rows {
		require.NoError(t, DB.Create(&rows[i]).Error)
	}

	expired, expiredTotal, err := ListAdminTopupOrders(AdminTopupOrderListFilter{Status: "expired", Now: now, Limit: 50})
	require.NoError(t, err)
	require.EqualValues(t, 2, expiredTotal)
	require.ElementsMatch(t, []string{"USDT-LATE", "EPAY-EXP"}, orderNos(expired))

	pending, pendingTotal, err := ListAdminTopupOrders(AdminTopupOrderListFilter{Status: "pending", Now: now, Limit: 50})
	require.NoError(t, err)
	require.EqualValues(t, 2, pendingTotal)
	require.ElementsMatch(t, []string{"USDT-OPEN", "EPAY-PEND"}, orderNos(pending))

	review, _, err := ListAdminTopupOrders(AdminTopupOrderListFilter{Status: TopupOrderStatusNeedManual, Now: now, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, []string{"USDT-REVIEW"}, orderNos(review))
}

func orderNos(rows []TopupOrder) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.OrderNo)
	}
	return out
}
