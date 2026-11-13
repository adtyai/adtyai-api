package common

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestQuotaPerUnitDecimalIgnoresFloatOverride(t *testing.T) {
	old := QuotaPerUnit
	QuotaPerUnit = 1
	t.Cleanup(func() { QuotaPerUnit = old })

	require.True(t, QuotaPerUnitDecimal().Equal(decimal.NewFromInt(500000)))
	quota, err := WalletQuotaFromDecimalStrict(decimal.RequireFromString("1.25").Mul(QuotaPerUnitDecimal()))
	require.NoError(t, err)
	require.Equal(t, 625000, quota)
}
