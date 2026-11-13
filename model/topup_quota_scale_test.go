package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopUpQuotaFromUSDUnitsUsesOfficialScale(t *testing.T) {
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = old })

	quota, err := topUpQuotaFromUSDUnits(3)
	require.NoError(t, err)
	assert.Equal(t, 1_500_000, quota, "credit path must use QuotaPerUnitDecimal, not NewFromFloat(common.QuotaPerUnit)")

	quota, err = topUpQuotaFromUSDUnits(0)
	require.NoError(t, err)
	assert.Equal(t, 0, quota)

	_, err = topUpQuotaFromUSDUnits(int64(common.MaxWalletQuota/int(common.FixedQuotaPerUnit) + 1))
	require.Error(t, err)
}
