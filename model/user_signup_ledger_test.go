package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSignupPaidQuotaWritesLedger(t *testing.T) {
	truncateTables(t)
	prev := common.QuotaForNewUser
	common.QuotaForNewUser = 1200
	t.Cleanup(func() { common.QuotaForNewUser = prev })

	user := &User{
		Username:    "signup-" + common.GetRandomString(8),
		Password:    "password-123456",
		DisplayName: "signup",
		Email:       common.GetRandomString(8) + "@example.com",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	require.NoError(t, user.Insert(0))

	var paid int
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select("quota").Scan(&paid).Error)
	require.Equal(t, 1200, paid)

	var count int64
	require.NoError(t, DB.Model(&LedgerEntry{}).
		Where("user_id = ? AND kind = ? AND account = ?", user.Id, "signup_bonus", "paid").
		Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestSignupMissingInviterRollsBackUser(t *testing.T) {
	truncateTables(t)
	prev := common.QuotaForInviter
	common.QuotaForInviter = 800
	t.Cleanup(func() { common.QuotaForInviter = prev })

	user := &User{
		Username:    "missing-inviter-" + common.GetRandomString(8),
		Password:    "password-123456",
		DisplayName: "missing",
		Email:       common.GetRandomString(8) + "@example.com",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	err := user.Insert(424242)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	var count int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", user.Username).Count(&count).Error)
	require.Zero(t, count)
}
