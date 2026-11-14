package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestCheckinCreditDoesNotRunWhenSettingEnabled(t *testing.T) {
	db := adtyTestDB(t)
	useUserCacheMiniRedis(t)
	resetBatchUpdateTestState(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Checkin{}))
	saved := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 5000, MaxQuota: 5000}
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = saved })
	require.False(t, operation_setting.IsCheckinEnabled())

	user := createReserveTestUser(t, 1000)
	require.NoError(t, populateUserCache(user))
	record, err := UserCheckin(user.Id)
	require.ErrorIs(t, err, ErrCheckinRetired)
	require.Nil(t, record)

	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 1000, stored.Quota)
	require.Zero(t, stored.CheckinQuota)
	var count int64
	require.NoError(t, db.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&count).Error)
	require.Zero(t, count, "enabling the setting must not insert a new award")

	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, 1000, cached.Quota)
	reserved, err := TryReserveUserQuota(user.Id, 1000)
	require.NoError(t, err)
	require.True(t, reserved)
}

func TestCheckinDoesNotChangeWalletWhenSettingEnabled(t *testing.T) {
	db := adtyTestDB(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true
	require.NoError(t, db.AutoMigrate(&User{}, &Checkin{}))
	saved := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 5000, MaxQuota: 5000}
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = saved })

	for _, tc := range []struct {
		name    string
		raw     int
		pending int
	}{
		{name: "pending credit fills ceiling", raw: common.MaxWalletQuota - 5000, pending: 5000},
		{name: "pending spend leaves room", raw: common.MaxWalletQuota, pending: -5000},
		{name: "negative raw stays unchanged", raw: -80, pending: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := createReserveTestUser(t, tc.raw)
			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota_pending", tc.pending).Error)
			_, err := UserCheckin(user.Id)
			require.ErrorIs(t, err, ErrCheckinRetired)
			var stored User
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, tc.raw, stored.Quota)
			require.Equal(t, tc.pending, stored.QuotaPending)
			require.Zero(t, stored.CheckinQuota)
			var count int64
			require.NoError(t, db.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&count).Error)
			require.Zero(t, count, "a retired credit path cannot insert an award")
		})
	}
}
