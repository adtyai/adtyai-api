package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestCumulativeGiftReclaimUsesRefundedPrincipalRatio(t *testing.T) {
	gift := decimal.RequireFromString("10")
	principal := decimal.RequireFromString("100")
	first := CumulativeGiftReclaim(gift, principal, decimal.Zero, decimal.RequireFromString("40"))
	require.True(t, first.Equal(decimal.RequireFromString("4")))
	second := CumulativeGiftReclaim(gift, principal, decimal.RequireFromString("40"), decimal.RequireFromString("60"))
	require.True(t, second.Equal(decimal.RequireFromString("6")))
}

func TestManualRefundUSDRejectsAmountAboveRemainingPrincipal(t *testing.T) {
	_, err := ManualRefundUSDQuota(decimal.RequireFromString("100"), decimal.RequireFromString("30"), decimal.RequireFromString("80"))
	require.Error(t, err)
}

func TestManualRefundUSDConvertsAtFixedScale(t *testing.T) {
	quota, err := ManualRefundUSDQuota(decimal.RequireFromString("100"), decimal.Zero, decimal.RequireFromString("1.5"))
	require.NoError(t, err)
	require.EqualValues(t, 750000, quota)
}
