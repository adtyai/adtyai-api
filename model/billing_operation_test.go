package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBillingEarningFactorsUsable(t *testing.T) {
	t.Parallel()
	assert.True(t, BillingEarningFactorsUsable(1, 1))
	assert.True(t, BillingEarningFactorsUsable(0.8, 0.5), "markup k may be below 1")
	assert.True(t, BillingEarningFactorsUsable(1.25, 1.5), "cost factor d may be above 1")
	assert.True(t, BillingEarningFactorsUsable(0.8, 1.5))
	assert.False(t, BillingEarningFactorsUsable(0, 1.5))
	assert.False(t, BillingEarningFactorsUsable(1.25, 0))
	assert.False(t, BillingEarningFactorsUsable(-0.5, 1))
	assert.False(t, BillingEarningFactorsUsable(1, -0.5))
	assert.False(t, BillingEarningFactorsUsable(math.NaN(), 1))
	assert.False(t, BillingEarningFactorsUsable(1, math.NaN()))
	assert.False(t, BillingEarningFactorsUsable(math.Inf(1), 1))
	assert.False(t, BillingEarningFactorsUsable(1, math.Inf(1)))
}
