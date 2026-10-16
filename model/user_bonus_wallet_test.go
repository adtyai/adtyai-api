package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDualWalletReserveBonusFirst(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.RedisEnabled = false

	user := createReserveTestUser(t, 10) // paid=10
	require.NoError(t, IncreaseUserBonusQuota(user.Id, 5, "test_credit", "seed bonus", "test", "seed1"))

	bonus, err := GetUserBonusBalance(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(5), bonus)

	total, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 15, total)

	// 消耗 7 → 赠送 0、实充 8
	b, p, ok, err := TryReserveUserWallet(user.Id, 7)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 5, b)
	assert.Equal(t, 2, p)

	bonus, err = GetUserBonusBalance(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(0), bonus)
	assert.Equal(t, 8, getUserQuotaFromDB(t, user.Id))

	total, err = GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 8, total)
}

func TestDualWalletBonusOnlyDoesNot403(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.RedisEnabled = false

	user := createReserveTestUser(t, 0) // paid=0
	require.NoError(t, IncreaseUserBonusQuota(user.Id, 100, "test_credit", "bonus only", "test", "seed2"))

	total, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 100, total)

	ok, err := TryReserveUserQuota(user.Id, 40)
	require.NoError(t, err)
	assert.True(t, ok, "bonus-only user with enough balance must reserve successfully")

	bonus, err := GetUserBonusBalance(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(60), bonus)
	assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))

	ok, err = TryReserveUserQuota(user.Id, 61)
	require.NoError(t, err)
	assert.False(t, ok, "when total insufficient, reserve must fail (maps to 403 upstream)")
}

func TestDualWalletRestoreSplit(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.RedisEnabled = false

	user := createReserveTestUser(t, 10)
	require.NoError(t, IncreaseUserBonusQuota(user.Id, 5, "test_credit", "seed", "test", "seed3"))

	b, p, ok, err := TryReserveUserWallet(user.Id, 7)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 5, b)
	require.Equal(t, 2, p)

	require.NoError(t, ReturnReservedUserWallet(user.Id, b, p))
	bonus, err := GetUserBonusBalance(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(5), bonus)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
}

func TestDecreaseUserWalletDoesNotGoNegative(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.RedisEnabled = false

	user := createReserveTestUser(t, 1) // paid=1
	require.NoError(t, EnsureUserBonusWallet(DB, user.Id))

	b, p, err := DecreaseUserWallet(user.Id, 5)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInsufficientWalletBalance))
	assert.Equal(t, 0, b)
	assert.Equal(t, 0, p)
	assert.Equal(t, 1, getUserQuotaFromDB(t, user.Id), "paid must stay 1, not negative")
	bonus, err := GetUserBonusBalance(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(0), bonus)
}

// 审查 P0-1：服务交付后的结算补扣不可免单——赠送扣到 0，实充扣成负数（对齐官方）。
func TestDecreaseUserWalletAllowDebtChargesIntoArrears(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.RedisEnabled = false

	user := createReserveTestUser(t, 100) // paid=100
	require.NoError(t, EnsureUserBonusWallet(DB, user.Id))
	require.NoError(t, IncreaseUserBonusQuota(user.Id, 300, "topup_bonus", "seed", "test", "p0-1"))

	b, p, err := DecreaseUserWalletAllowDebt(user.Id, 675)
	require.NoError(t, err)
	assert.Equal(t, 300, b)
	assert.Equal(t, 375, p)
	assert.Equal(t, -275, getUserQuotaFromDB(t, user.Id), "paid goes negative (arrears)")
	bonus, err := GetUserBonusBalance(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(0), bonus)

	// 欠费后下一次预留必须失败
	_, _, ok, err := TryReserveUserWallet(user.Id, 1)
	require.NoError(t, err)
	assert.False(t, ok)
}
