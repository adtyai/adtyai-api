package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestCheckinCreditIsAvailableBeforeReturn(t *testing.T) {
	db := adtyTestDB(t)
	useUserCacheMiniRedis(t)
	resetBatchUpdateTestState(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Checkin{}))
	saved := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 5000, MaxQuota: 5000}
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = saved })

	user := createReserveTestUser(t, 1000)
	require.NoError(t, populateUserCache(user))
	record, err := UserCheckin(user.Id)
	require.NoError(t, err)
	require.Equal(t, 5000, record.QuotaAwarded)
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, 6000, cached.Quota)
	reserved, err := TryReserveUserQuota(user.Id, 6000)
	require.NoError(t, err)
	require.True(t, reserved, "the complete committed credit must be immediately spendable")
}

func TestCheckinUsesLogicalBalanceAndRollsBackAtWalletLimit(t *testing.T) {
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
		wantErr bool
	}{
		{name: "pending credit fills ceiling", raw: common.MaxWalletQuota - 5000, pending: 5000, wantErr: true},
		{name: "pending spend leaves room", raw: common.MaxWalletQuota, pending: -5000},
		{name: "negative raw does not create negative gift", raw: -80, pending: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := createReserveTestUser(t, tc.raw)
			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota_pending", tc.pending).Error)
			_, err := UserCheckin(user.Id)
			var stored User
			require.NoError(t, db.First(&stored, user.Id).Error)
			if tc.wantErr {
				require.Error(t, err)
				require.Equal(t, tc.raw, stored.Quota)
				require.Zero(t, stored.CheckinQuota)
				var count int64
				require.NoError(t, db.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&count).Error)
				require.Zero(t, count, "a rejected credit cannot consume today's check-in")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.raw+5000, stored.Quota)
			require.Equal(t, tc.pending, stored.QuotaPending)
			require.Equal(t, 5000, stored.CheckinQuota)
		})
	}
}
