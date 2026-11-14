package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExprOutputKeepsExactDecimalPriceCoefficients(t *testing.T) {
	// 0.1234567890123456789 cannot be represented exactly in float64, so a
	// float VM would hand a rounded exprOutput to settlement.
	const coeff = "0.1234567890123456789"
	exprStr := `tier("base", p * ` + coeff + ` + c * 0.1)`
	params := billingexpr.TokenParams{P: 3, C: 3}

	_, trace, err := billingexpr.RunExpr(exprStr, params)
	require.NoError(t, err)

	want := decimal.RequireFromString(coeff).Mul(decimal.NewFromInt(3)).Add(decimal.RequireFromString("0.1").Mul(decimal.NewFromInt(3)))
	require.True(t, trace.CostDec.Equal(want), "got %s want %s", trace.CostDec, want)

	floatOutput := 3*0.1234567890123456789 + 3*0.1
	assert.False(t, decimal.NewFromFloat(floatOutput).Equal(want), "the float64 product must actually differ so this test would fail on the old VM")

	snap := &billingexpr.BillingSnapshot{
		ExprString:   exprStr,
		ExprHash:     billingexpr.ExprHashString(exprStr),
		GroupRatio:   1,
		QuotaPerUnit: 500_000,
	}
	result, err := billingexpr.ComputeTieredQuota(snap, params)
	require.NoError(t, err)
	expectedBefore := want.Div(decimal.NewFromInt(1_000_000)).Mul(common.QuotaPerUnitDecimal())
	assert.True(t, result.ActualQuotaBeforeGroupDec.Equal(expectedBefore), "got %s want %s", result.ActualQuotaBeforeGroupDec, expectedBefore)
}

func TestFixedPriceLiteralStaysExactBeforeSettlement(t *testing.T) {
	const exprStr = `tier("request", fixed(0.07))`
	_, trace, err := billingexpr.RunExpr(exprStr, billingexpr.TokenParams{})
	require.NoError(t, err)
	wantOutput := decimal.RequireFromString("0.07").Mul(decimal.NewFromInt(1_000_000))
	require.True(t, trace.CostDec.Equal(wantOutput), "got %s want %s", trace.CostDec, wantOutput)

	snap := &billingexpr.BillingSnapshot{
		ExprString:   exprStr,
		ExprHash:     billingexpr.ExprHashString(exprStr),
		GroupRatio:   1.1,
		QuotaPerUnit: 500_000,
	}
	result, err := billingexpr.ComputeTieredQuota(snap, billingexpr.TokenParams{})
	require.NoError(t, err)
	assert.True(t, result.ActualQuotaBeforeGroupDec.Equal(decimal.RequireFromString("35000")))
	assert.Equal(t, 38500, result.ActualQuotaAfterGroup)
}

func TestScientificPriceCoefficientUsesSourceLiteral(t *testing.T) {
	_, trace, err := billingexpr.RunExpr(`p * 1e-1`, billingexpr.TokenParams{P: 3})
	require.NoError(t, err)
	assert.True(t, trace.CostDec.Equal(decimal.RequireFromString("0.3")), "got %s", trace.CostDec)
}
