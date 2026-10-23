package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestCreateAffWithdrawalDoesNotReturnAnotherUsersRow(t *testing.T) {
	l2l3l4Cleanup(t)
	owner := l2l3l4User(t, 0, int(10*common.QuotaPerUnit), 0)
	other := l2l3l4User(t, 0, int(10*common.QuotaPerUnit), 0)
	const idem = "shared-withdraw-idem"
	const ownerAddress = "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb"
	const otherAddress = "0x0000000000000000000000000000000000000001"

	row, err := CreateAffWithdrawal(owner.Id, usdtWithdrawInput(1000, idem, ownerAddress, "trc20"))
	require.NoError(t, err)
	require.Equal(t, owner.Id, row.UserId)
	require.Equal(t, ownerAddress, row.PayAccount)

	leaked, err := CreateAffWithdrawal(other.Id, usdtWithdrawInput(1000, idem, otherAddress, "bep20"))
	require.ErrorIs(t, err, ErrAffWithdrawIdempotent)
	require.Nil(t, leaked)
	require.NotContains(t, err.Error(), ownerAddress)
	require.NotContains(t, err.Error(), otherAddress)

	var ownerAfter, otherAfter User
	require.NoError(t, DB.First(&ownerAfter, owner.Id).Error)
	require.NoError(t, DB.First(&otherAfter, other.Id).Error)
	require.Equal(t, owner.AffQuota-1000, ownerAfter.AffQuota)
	require.Equal(t, other.AffQuota, otherAfter.AffQuota)

	mismatch, err := CreateAffWithdrawal(owner.Id, usdtWithdrawInput(500, idem, otherAddress, "bep20"))
	require.ErrorIs(t, err, ErrAffWithdrawIdempotentMismatch)
	require.Nil(t, mismatch)
	require.NotContains(t, err.Error(), ownerAddress)
	require.NotContains(t, err.Error(), otherAddress)
	require.NoError(t, DB.First(&ownerAfter, owner.Id).Error)
	require.Equal(t, owner.AffQuota-1000, ownerAfter.AffQuota)
}
