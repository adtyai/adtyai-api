package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestSaveGroupBillingRefreshesMemoryWhenPublishFails(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	previous := applyOptionMemory
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		applyOptionMemory = previous
		common.OptionMap = previousMap
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		require.NoError(t, DB.Where("key IN ?", []string{"GroupRatio", "UserUsableGroups"}).Delete(&Option{}).Error)
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP"}`))
	applyOptionMemory = func(string, string) error {
		return errors.New("memory publish failed")
	}

	err := SaveGroupBillingTable(
		map[string]float64{"vip": 1},
		map[string]string{"vip": "VIP"},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()["vip"])
	require.Equal(t, "VIP", setting.GetUserUsableGroupsCopy()["vip"])
}

func TestCreateGroupRefreshesMemoryWhenPublishFails(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}, &GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	previous := applyOptionMemory
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		applyOptionMemory = previous
		common.OptionMap = previousMap
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, DB.Where("code = ?", "refreshonly").Delete(&GroupMeta{}).Error)
		require.NoError(t, DB.Where("key = ?", "GroupRatio").Delete(&Option{}).Error)
	})
	calls := 0
	applyOptionMemory = func(string, string) error {
		calls++
		return errors.New("memory publish failed")
	}

	_, err := CreateGroupWithRatio(DB, GroupMetaCreateInput{Code: "refreshonly", DisplayName: "Refresh"})
	require.NotZero(t, calls)
	require.NoError(t, err)
	require.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()["refreshonly"])
}
