package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestCustomHostnameMonthlyFeeStartsAfterOneHundred(t *testing.T) {
	assert.True(t, adtyHostnameMonthlyFee(0).IsZero())
	assert.True(t, adtyHostnameMonthlyFee(100).IsZero())
	assert.True(t, adtyHostnameMonthlyFee(101).Equal(decimal.RequireFromString("0.10")))
	assert.True(t, adtyHostnameMonthlyFee(103).Equal(decimal.RequireFromString("0.30")))
}
