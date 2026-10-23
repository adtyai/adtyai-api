package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestInsertDoesNotAssignHiddenDefaultIdentity(t *testing.T) {
	truncateTables(t)
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP"}`))

	user := &User{
		Username:    "hidden-default-" + common.GetRandomString(6),
		Password:    "password-123456",
		DisplayName: "hidden",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	err := user.Insert(0)
	require.ErrorIs(t, err, ErrNoPublicIdentityGroup)

	var count int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", user.Username).Count(&count).Error)
	require.Zero(t, count)
}

func TestInsertAssignsPublicDefaultIdentity(t *testing.T) {
	truncateTables(t)
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))

	user := &User{
		Username:    "public-default-" + common.GetRandomString(6),
		Password:    "password-123456",
		DisplayName: "public",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	require.NoError(t, user.Insert(0))

	var stored User
	require.NoError(t, DB.Select("id", "group").Where("username = ?", user.Username).First(&stored).Error)
	require.Equal(t, "default", stored.Group)
}
