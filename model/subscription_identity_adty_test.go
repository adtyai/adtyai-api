package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func usePublicAndHiddenGroups(t *testing.T) {
	t.Helper()
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"channelonly":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
}

func TestCreateSubscriptionKeepsIdentityWhenUpgradeGroupIsHidden(t *testing.T) {
	truncateTables(t)
	usePublicAndHiddenGroups(t)

	user := User{
		Username: "sub-hidden-upgrade",
		Password: "unused-password-hash",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)
	plan := &SubscriptionPlan{
		Title:         "Hidden upgrade",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   100,
		UpgradeGroup:  "channelonly",
		Enabled:       true,
	}
	require.NoError(t, DB.Create(plan).Error)

	subscription, err := CreateUserSubscriptionFromPlanTx(DB, user.Id, plan, "test")
	require.NoError(t, err)
	require.NotZero(t, subscription.Id)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, "default", stored.Group)
}

func TestExpireDueSubscriptionsKeepsIdentityWhenTargetIsHidden(t *testing.T) {
	truncateTables(t)
	usePublicAndHiddenGroups(t)

	user := User{
		Username: "sub-hidden-expire",
		Password: "unused-password-hash",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "vip",
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:         user.Id,
		PlanId:         1,
		Status:         "active",
		EndTime:        1,
		UpgradeGroup:   "vip",
		PrevUserGroup:  "default",
		DowngradeGroup: "channelonly",
	}).Error)

	expired, err := ExpireDueSubscriptions(10)
	require.NoError(t, err)
	require.Equal(t, 1, expired)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, "vip", stored.Group)
	var subscription UserSubscription
	require.NoError(t, DB.First(&subscription, "user_id = ?", user.Id).Error)
	require.Equal(t, "expired", subscription.Status)
}

func TestExpireDueSubscriptionsStillRestoresPublicIdentity(t *testing.T) {
	truncateTables(t)
	usePublicAndHiddenGroups(t)

	user := User{
		Username: "sub-public-expire",
		Password: "unused-password-hash",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "vip",
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:        user.Id,
		PlanId:        1,
		Status:        "active",
		EndTime:       1,
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}).Error)

	expired, err := ExpireDueSubscriptions(10)
	require.NoError(t, err)
	require.Equal(t, 1, expired)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Equal(t, "default", stored.Group)
}
