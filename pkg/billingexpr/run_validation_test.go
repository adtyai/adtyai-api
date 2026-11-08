package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
)

func TestRunExprRejectsInvalidCostInRuntimeBranch(t *testing.T) {
	for _, tc := range []struct {
		name string
		expr string
	}{
		{"negative", `c == 777 ? -1000000.0 : p`},
		{"not a number", `c == 777 ? (c-c)/(c-c) : p`},
		{"positive infinity", `c == 777 ? c/(c-c) : p`},
		{"negative infinity", `c == 777 ? -c/(c-c) : p`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The reservation branch is valid; validation must also run for
			// the actual usage, which may select a different branch.
			cost, _, err := billingexpr.RunExpr(tc.expr, billingexpr.TokenParams{P: 100})
			require.NoError(t, err)
			require.Equal(t, 100.0, cost)

			cost, _, err = billingexpr.RunExprByHash(tc.expr, billingexpr.ExprHashString(tc.expr), billingexpr.TokenParams{P: 100, C: 777})
			require.Error(t, err)
			require.Zero(t, cost, "invalid costs must not reach quota conversion")
		})
	}
}

func TestRunTaskUsageExprRejectsNegativeRuntimeCost(t *testing.T) {
	const expression = `u("seconds") == 777 ? -1.0 : 1.0`
	cost, _, err := billingexpr.RunExprByHashWithRequest(expression, billingexpr.ExprHashString(expression), billingexpr.TokenParams{}, billingexpr.RequestInput{
		Usage: map[string]any{"seconds": 777.0},
	})
	require.Error(t, err)
	require.Zero(t, cost)
}
