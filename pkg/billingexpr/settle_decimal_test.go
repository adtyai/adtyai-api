package billingexpr

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTieredSettleUsesOfficialScaleAndExactGroupRatio(t *testing.T) {
	snap := &BillingSnapshot{
		ExprString:   `tier("request", fixed(0.07))`,
		ExprHash:     ExprHashString(`tier("request", fixed(0.07))`),
		GroupRatio:   1.1,
		QuotaPerUnit: 500_000,
	}
	result, err := ComputeTieredQuota(snap, TokenParams{})
	require.NoError(t, err)

	expected := decimal.RequireFromString("0.07").Mul(common.QuotaPerUnitDecimal()).Mul(decimal.RequireFromString("1.1"))
	require.Equal(t, "38500", expected.String())
	assert.Equal(t, 38500, result.ActualQuotaAfterGroup)
	assert.True(t, result.ActualQuotaBeforeGroupDec.Equal(decimal.RequireFromString("35000")))
	assert.Nil(t, result.Clamp)

	// A mutated float scale on the process-wide variable must not change official snapshots.
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = old })
	again, err := ComputeTieredQuota(snap, TokenParams{})
	require.NoError(t, err)
	assert.Equal(t, 38500, again.ActualQuotaAfterGroup)
}

func TestTieredSettleOverflowStillFailsClosed(t *testing.T) {
	snap := &BillingSnapshot{
		ExprString:   `tier("huge", fixed(1e20))`,
		ExprHash:     ExprHashString(`tier("huge", fixed(1e20))`),
		GroupRatio:   1,
		QuotaPerUnit: 500_000,
	}
	result, err := ComputeTieredQuota(snap, TokenParams{})
	require.NoError(t, err)
	assert.Equal(t, int(common.MaxQuota), result.ActualQuotaAfterGroup)
	assert.NotNil(t, result.Clamp)
	assert.Equal(t, common.QuotaClampOverflow, result.Clamp.Kind)
}
