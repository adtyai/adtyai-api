package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestMarginCycleDrawLimit(t *testing.T) {
	tier := decimal.RequireFromString("10000")
	limit := AdtyMarginCycleLimit(tier)
	require.True(t, limit.Equal(decimal.RequireFromString("9000")))

	got := AdtyMarginNewDraw(AdtyMarginDrawInput{
		TierUSD:     tier,
		Balance:     decimal.RequireFromString("10000"),
		Used:        decimal.RequireFromString("8000"),
		Frozen:      decimal.RequireFromString("500"),
		Reserved:    decimal.RequireFromString("200"),
	})
	// 90% cap remaining = 9000-8000-500-200 = 300
	// cash room = 10000-500-200-1000 = 8300
	require.True(t, got.Equal(decimal.RequireFromString("300")))
}

func TestMarginCycleResetsOnlyAfterRealTopUp(t *testing.T) {
	tier := decimal.RequireFromString("10000")
	blocked := AdtyMarginNewDraw(AdtyMarginDrawInput{
		TierUSD: tier,
		Balance: decimal.RequireFromString("10000"),
		Used:    decimal.RequireFromString("9000"),
	})
	require.True(t, blocked.IsZero())

	opened := AdtyMarginNewDraw(AdtyMarginDrawInput{
		TierUSD:     tier,
		Balance:     decimal.RequireFromString("11000"),
		Used:        decimal.Zero,
		CycleOpened: true,
	})
	require.True(t, opened.Equal(decimal.RequireFromString("9000")))
}
