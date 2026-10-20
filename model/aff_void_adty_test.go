package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// 审查 N-P1-6：返佣已提现（aff_quota 不足）时作废被拒，aff_quota 不变负；余额足够时正常冲正。
func TestVoidAffCommissionRefusesWhenWithdrawn(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.RedisEnabled = false
	require.NoError(t, DB.AutoMigrate(&AffCommission{}, &AffWithdrawal{}))
	require.NoError(t, EnsureLedgerSchema(DB))
	u := createReserveTestUser(t, 0)
	c := AffCommission{InviterId: u.Id, InviteeId: u.Id + 1, OrderNo: "N16-1", Source: "topup", CommissionQuota: 5000000, Status: AffCommissionStatusAvailable}
	require.NoError(t, DB.Create(&c).Error)

	_, err := VoidAffCommission(c.Id, 1)
	require.ErrorIs(t, err, ErrAffCommissionAlreadyWithdrawn)
	var aff int
	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Select("aff_quota").Scan(&aff).Error)
	require.Equal(t, 0, aff)
	var st string
	require.NoError(t, DB.Model(&AffCommission{}).Where("id = ?", c.Id).Select("status").Scan(&st).Error)
	require.Equal(t, AffCommissionStatusAvailable, st)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Update("aff_quota", 6000000).Error)
	out, err := VoidAffCommission(c.Id, 1)
	require.NoError(t, err)
	require.Equal(t, AffCommissionStatusVoided, out.Status)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Select("aff_quota").Scan(&aff).Error)
	require.Equal(t, 1000000, aff)
}
