package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkTopupOrderNeedManualStoresCallbackAmount(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TopupOrder{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM topup_orders") })

	order := &TopupOrder{
		OrderNo:       "USDT-SHORT",
		UserId:        1,
		AmountUSD:     10,
		PaymentMethod: "usdt",
		Status:        TopupOrderStatusPending,
		PayUSDT:       10,
		RefundStatus:  RefundStatusNone,
	}
	require.NoError(t, DB.Create(order).Error)

	got, err := MarkTopupOrderNeedManual(order.OrderNo, "underpay", 8.5)
	require.NoError(t, err)
	require.Equal(t, TopupOrderStatusNeedManual, got.Status)
	require.Equal(t, "underpay", got.PendingReason)
	require.NotNil(t, got.UsdtCallbackAmount)
	require.InDelta(t, 8.5, *got.UsdtCallbackAmount, 1e-8)

	var stored TopupOrder
	require.NoError(t, DB.Where("order_no = ?", order.OrderNo).First(&stored).Error)
	require.NotNil(t, stored.UsdtCallbackAmount)
	require.InDelta(t, 8.5, *stored.UsdtCallbackAmount, 1e-8)

	again, err := MarkTopupOrderNeedManual(order.OrderNo, "timeout", 9)
	require.NoError(t, err)
	require.Equal(t, "underpay", again.PendingReason)
	require.NotNil(t, again.UsdtCallbackAmount)
	require.InDelta(t, 8.5, *again.UsdtCallbackAmount, 1e-8)
}

func TestMarkTopupOrderNeedManualFillsMissingCallbackAmount(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TopupOrder{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM topup_orders") })

	order := &TopupOrder{
		OrderNo:       "USDT-LATE",
		UserId:        1,
		AmountUSD:     10,
		PaymentMethod: "usdt",
		Status:        TopupOrderStatusPending,
		PayUSDT:       10,
		RefundStatus:  RefundStatusNone,
	}
	require.NoError(t, DB.Create(order).Error)

	first, err := MarkTopupOrderNeedManual(order.OrderNo, "timeout", -1)
	require.NoError(t, err)
	require.Equal(t, TopupOrderStatusNeedManual, first.Status)
	require.Nil(t, first.UsdtCallbackAmount)

	filled, err := MarkTopupOrderNeedManual(order.OrderNo, "timeout", 10)
	require.NoError(t, err)
	require.NotNil(t, filled.UsdtCallbackAmount)
	require.InDelta(t, 10, *filled.UsdtCallbackAmount, 1e-8)
}
