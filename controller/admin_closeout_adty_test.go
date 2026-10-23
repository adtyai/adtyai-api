package controller

import (
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHiddenGroupGrantDoesNotChangeIdentity(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}))
	require.NoError(t, model.EnsureUserGroupGrantSchema(database))

	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))

	root := model.User{Username: "grant-root", Password: "unused", Role: common.RoleRootUser, Group: "default", AffCode: "grroot"}
	alice := model.User{Username: "grant-alice", DisplayName: "Alice", Password: "unused", Role: common.RoleCommonUser, Group: "default", AffCode: "gralice"}
	require.NoError(t, database.Create(&root).Error)
	require.NoError(t, database.Create(&alice).Error)

	params := gin.Params{{Key: "code", Value: "vip"}}
	denied := adtyCall(AdminGrantGroup, http.MethodPost, "/api/admin/groups/vip/grants", `{"user_id":`+strconv.Itoa(alice.Id)+`}`, alice.Id, common.RoleAdminUser, params)
	require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())

	public := adtyCall(AdminGrantGroup, http.MethodPost, "/api/admin/groups/default/grants", `{"user_id":`+strconv.Itoa(alice.Id)+`}`, root.Id, common.RoleRootUser, gin.Params{{Key: "code", Value: "default"}})
	require.Equal(t, http.StatusBadRequest, public.Code, public.Body.String())
	require.Contains(t, public.Body.String(), "not_hidden")

	missing := adtyCall(AdminGrantGroup, http.MethodPost, "/api/admin/groups/vip/grants", `{"user_id":999999}`, root.Id, common.RoleRootUser, params)
	require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())

	granted := adtyCall(AdminGrantGroup, http.MethodPost, "/api/admin/groups/vip/grants", `{"user_id":`+strconv.Itoa(alice.Id)+`}`, root.Id, common.RoleRootUser, params)
	require.Equal(t, http.StatusOK, granted.Code, granted.Body.String())
	again := adtyCall(AdminGrantGroup, http.MethodPost, "/api/admin/groups/vip/grants", `{"user_id":`+strconv.Itoa(alice.Id)+`}`, root.Id, common.RoleRootUser, params)
	require.Equal(t, http.StatusConflict, again.Code, again.Body.String())

	var stored model.User
	require.NoError(t, database.First(&stored, alice.Id).Error)
	require.Equal(t, "default", stored.Group)

	allowed := service.GetUserAllowedGroups(alice.Id, stored.Group)
	_, ok := allowed["vip"]
	require.True(t, ok)
	_, publicOnly := service.GetUserAllowedGroups(0, stored.Group)["vip"]
	require.False(t, publicOnly)
	require.True(t, service.UserMayUseGroup(alice.Id, stored.Group, "vip"))
	require.False(t, service.UserMayUseGroup(alice.Id, stored.Group, "auto"))

	listed := adtyCall(AdminListGroupGrants, http.MethodGet, "/api/admin/groups/vip/grants", "", root.Id, common.RoleRootUser, params)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"user_id":`+strconv.Itoa(alice.Id))
	require.Contains(t, listed.Body.String(), "grant-alice")

	revoked := adtyCall(AdminRevokeGroupGrant, http.MethodDelete, "/api/admin/groups/vip/grants/"+strconv.Itoa(alice.Id), "", root.Id, common.RoleRootUser, gin.Params{{Key: "code", Value: "vip"}, {Key: "user_id", Value: strconv.Itoa(alice.Id)}})
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	missingGrant := adtyCall(AdminRevokeGroupGrant, http.MethodDelete, "/api/admin/groups/vip/grants/"+strconv.Itoa(alice.Id), "", root.Id, common.RoleRootUser, gin.Params{{Key: "code", Value: "vip"}, {Key: "user_id", Value: strconv.Itoa(alice.Id)}})
	require.Equal(t, http.StatusNotFound, missingGrant.Code, missingGrant.Body.String())

	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		var indexDef string
		require.NoError(t, database.Raw(`SELECT indexdef FROM pg_indexes WHERE tablename = 'user_group_grants' AND indexname = 'ux_user_group_grants_user_code'`).Scan(&indexDef).Error)
		require.Contains(t, indexDef, "UNIQUE")
		require.Contains(t, indexDef, "user_id")
		require.Contains(t, indexDef, "group_code")
	}
}

func TestAdminListAffInvitersSearchesDirectory(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.AffCommission{}))

	inviter := model.User{Username: "InviterA", DisplayName: "推广甲", Password: "unused", AffCode: "invA", AffQuota: 1000, AffHistoryQuota: 2500}
	invitee := model.User{Username: "invitee-a", Password: "unused", AffCode: "invEa"}
	commissionOnly := model.User{Username: "commission-only", Password: "unused", AffCode: "invC"}
	unrelated := model.User{Username: "plain-user", Password: "unused", AffCode: "invP"}
	require.NoError(t, database.Create(&inviter).Error)
	invitee.InviterId = inviter.Id
	require.NoError(t, database.Create(&invitee).Error)
	require.NoError(t, database.Create(&commissionOnly).Error)
	require.NoError(t, database.Create(&unrelated).Error)
	require.NoError(t, database.Create(&model.AffCommission{
		InviterId: commissionOnly.Id, InviteeId: unrelated.Id, OrderNo: "closeout-aff-1",
		Source: "topup", PaidAmountUSD: 10, RateSnapshot: 0.1, CommissionQuota: 100, Status: "available",
	}).Error)

	all := adtyCall(AdminListAffInviters, http.MethodGet, "/api/admin/aff/inviters", "", 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusOK, all.Code, all.Body.String())
	require.Contains(t, all.Body.String(), "InviterA")
	require.Contains(t, all.Body.String(), "commission-only")
	require.NotContains(t, all.Body.String(), "plain-user")
	require.Contains(t, all.Body.String(), `"invitee_count":1`)

	byName := adtyCall(AdminListAffInviters, http.MethodGet, "/api/admin/aff/inviters?keyword=invitera", "", 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusOK, byName.Code, byName.Body.String())
	require.Contains(t, byName.Body.String(), "InviterA")
	require.NotContains(t, byName.Body.String(), "commission-only")

	byID := adtyCall(AdminListAffInviters, http.MethodGet, "/api/admin/aff/inviters?keyword="+strconv.Itoa(commissionOnly.Id), "", 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusOK, byID.Code, byID.Body.String())
	require.Contains(t, byID.Body.String(), "commission-only")
	require.NotContains(t, byID.Body.String(), "InviterA")
}

func TestAdminListAffInvitersNumericKeywordMatchesNameAndCode(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.AffCommission{}))

	byCode := model.User{Username: "code-inviter", DisplayName: "代码88001", Password: "unused", AffCode: "88001"}
	byName := model.User{Username: "77002", Password: "unused", AffCode: "nameInv"}
	unrelated := model.User{Username: "other-inviter", Password: "unused", AffCode: "otherInv"}
	require.NoError(t, database.Create(&byCode).Error)
	require.NoError(t, database.Create(&byName).Error)
	require.NoError(t, database.Create(&unrelated).Error)
	require.NotEqual(t, 88001, byCode.Id)
	require.NotEqual(t, 77002, byName.Id)
	require.NoError(t, database.Create(&model.User{Username: "code-invitee", Password: "unused", AffCode: "88001x", InviterId: byCode.Id}).Error)
	require.NoError(t, database.Create(&model.User{Username: "name-invitee", Password: "unused", AffCode: "77002x", InviterId: byName.Id}).Error)
	require.NoError(t, database.Create(&model.User{Username: "other-invitee", Password: "unused", AffCode: "otherx", InviterId: unrelated.Id}).Error)

	byAffCode := adtyCall(AdminListAffInviters, http.MethodGet, "/api/admin/aff/inviters?keyword=88001", "", 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusOK, byAffCode.Code, byAffCode.Body.String())
	require.Contains(t, byAffCode.Body.String(), "code-inviter")
	require.NotContains(t, byAffCode.Body.String(), "other-inviter")
	require.NotContains(t, byAffCode.Body.String(), "77002")

	byUsername := adtyCall(AdminListAffInviters, http.MethodGet, "/api/admin/aff/inviters?keyword=77002", "", 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusOK, byUsername.Code, byUsername.Body.String())
	require.Contains(t, byUsername.Body.String(), "\"username\":\"77002\"")
	require.NotContains(t, byUsername.Body.String(), "code-inviter")
}

func TestAffOverviewInviterCountMatchesDirectory(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.AffCommission{}, &model.AffWithdrawal{}))

	first := model.User{Username: "live-inviter-a", Password: "unused", AffCode: "ovA"}
	second := model.User{Username: "live-inviter-b", Password: "unused", AffCode: "ovB"}
	deleted := model.User{Username: "deleted-inviter", Password: "unused", AffCode: "ovD"}
	require.NoError(t, database.Create(&first).Error)
	require.NoError(t, database.Create(&second).Error)
	require.NoError(t, database.Create(&deleted).Error)
	require.NoError(t, database.Create(&model.User{Username: "invitee-a", Password: "unused", AffCode: "ovIa", InviterId: first.Id}).Error)
	require.NoError(t, database.Create(&model.User{Username: "invitee-b", Password: "unused", AffCode: "ovIb", InviterId: second.Id}).Error)
	require.NoError(t, database.Delete(&deleted).Error)
	require.NoError(t, database.Create(&model.AffCommission{
		InviterId: deleted.Id, InviteeId: first.Id, OrderNo: "closeout-aff-deleted",
		Source: "topup", PaidAmountUSD: 10, RateSnapshot: 0.1, CommissionQuota: 100, Status: "available",
	}).Error)

	_, total, err := model.ListAffAdminInviters("", 20, 0)
	require.NoError(t, err)
	overview, err := model.GetAffOverview()
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, total, overview.Inviters)
}

func TestAffOverviewCountsMatchLiveInviterDirectory(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.AffCommission{}, &model.AffWithdrawal{}))

	live := model.User{Username: "live-card-inviter", Password: "unused", AffCode: "cardLive"}
	gone := model.User{Username: "deleted-card-inviter", Password: "unused", AffCode: "cardGone"}
	require.NoError(t, database.Create(&live).Error)
	require.NoError(t, database.Create(&gone).Error)
	require.NoError(t, database.Create(&model.User{Username: "live-card-invitee", Password: "unused", AffCode: "cardIe", InviterId: live.Id}).Error)
	require.NoError(t, database.Create(&model.User{Username: "deleted-card-invitee", Password: "unused", AffCode: "cardIeD", InviterId: gone.Id}).Error)
	require.NoError(t, database.Delete(&gone).Error)
	require.NoError(t, database.Create(&model.AffCommission{
		InviterId: live.Id, InviteeId: 1, OrderNo: "closeout-card-live",
		Source: "topup", PaidAmountUSD: 10, RateSnapshot: 0.1, CommissionQuota: 20, Status: "available",
	}).Error)
	require.NoError(t, database.Create(&model.AffCommission{
		InviterId: gone.Id, InviteeId: 1, OrderNo: "closeout-card-deleted",
		Source: "topup", PaidAmountUSD: 10, RateSnapshot: 0.1, CommissionQuota: 80, Status: "available",
	}).Error)
	require.NoError(t, database.Create(&model.AffCommission{
		InviterId: gone.Id, InviteeId: 1, OrderNo: "closeout-card-deleted-frozen",
		Source: "topup", PaidAmountUSD: 1, RateSnapshot: 0.1, CommissionQuota: 5, Status: "frozen",
	}).Error)

	overview, err := model.GetAffOverview()
	require.NoError(t, err)
	_, total, err := model.ListAffAdminInviters("", 20, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, int64(1), overview.InvitedUsers)
	require.Equal(t, int64(20), overview.CommissionAvailable)
	require.Equal(t, int64(0), overview.CommissionFrozen)
}

func TestSaveGroupBillingTableDoesNotPartiallyWrite(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}))

	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP"}`))

	extra := model.SaveGroupBillingTable(
		map[string]float64{"vip": 2, "channelonly": 0},
		map[string]string{"vip": "VIP"},
		nil,
	)
	require.Error(t, extra)
	require.Equal(t, map[string]float64{"vip": 2}, ratio_setting.GetGroupRatioCopy())
	require.Equal(t, map[string]string{"vip": "VIP"}, setting.GetUserUsableGroupsCopy())

	rolled := model.SaveGroupBillingTable(
		map[string]float64{"vip": 1},
		map[string]string{"vip": "Hidden"},
		map[string]string{"vip": "\u4f1a\u5458"},
	)
	require.Error(t, rolled)
	require.Equal(t, map[string]float64{"vip": 2}, ratio_setting.GetGroupRatioCopy())
	require.Equal(t, map[string]string{"vip": "VIP"}, setting.GetUserUsableGroupsCopy())
	var stored model.Option
	err := database.Where("key = ?", "GroupRatio").First(&stored).Error
	if err == nil {
		parsed := map[string]float64{}
		require.NoError(t, common.Unmarshal([]byte(stored.Value), &parsed))
		require.Equal(t, map[string]float64{"vip": 2}, parsed)
	}
	err = database.Where("key = ?", "UserUsableGroups").First(&stored).Error
	if err == nil {
		parsed := map[string]string{}
		require.NoError(t, common.Unmarshal([]byte(stored.Value), &parsed))
		require.Equal(t, "VIP", parsed["vip"])
		require.NotContains(t, parsed, "channelonly")
	}
}

func TestSaveGroupBillingTableCommitsTogether(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}, &model.GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":2,"auto":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP"}`))

	require.NoError(t, model.SaveGroupBillingTable(
		map[string]float64{"vip": 0, "auto": 1},
		map[string]string{},
		map[string]string{"vip": "\u4f1a\u5458"},
	))
	require.Equal(t, 0.0, ratio_setting.GetGroupRatioCopy()["vip"])
	require.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()["auto"])
	require.NotContains(t, setting.GetUserUsableGroupsCopy(), "vip")
	var meta model.GroupMeta
	require.NoError(t, database.Where("code = ?", "vip").First(&meta).Error)
	require.Equal(t, "\u4f1a\u5458", meta.DisplayName)
}

func TestAdminPostGroupWritesRatioWithMeta(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}, &model.GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))

	auto := adtyCall(AdminPostGroup, http.MethodPost, "/api/admin/groups", `{"code":"auto","display_name":"Auto"}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusBadRequest, auto.Code, auto.Body.String())
	require.Contains(t, auto.Body.String(), "invalid_code")
	require.NotContains(t, auto.Body.String(), "no such table")
	require.NotContains(t, ratio_setting.GetGroupRatioCopy(), "auto")

	created := adtyCall(AdminPostGroup, http.MethodPost, "/api/admin/groups", `{"code":"vip","display_name":"\u4f1a\u5458"}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	require.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()["vip"])
	require.NotContains(t, setting.GetUserUsableGroupsCopy(), "vip")
	var meta model.GroupMeta
	require.NoError(t, database.Where("code = ?", "vip").First(&meta).Error)
	require.Equal(t, "\u4f1a\u5458", meta.DisplayName)

	again := adtyCall(AdminPostGroup, http.MethodPost, "/api/admin/groups", `{"code":"vip"}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusBadRequest, again.Code, again.Body.String())
	require.Contains(t, again.Body.String(), "exists")
	require.NotContains(t, again.Body.String(), "no such table")
	require.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()["vip"])
}

func TestAdminPostGroupRepairsMissingRatio(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}, &model.GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, database.Create(&model.GroupMeta{Code: "stuck", DisplayName: "Stuck"}).Error)

	repaired := adtyCall(AdminPostGroup, http.MethodPost, "/api/admin/groups", `{"code":"stuck"}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, repaired.Code, repaired.Body.String())
	require.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()["stuck"])
}

func TestUpdateUserRejectsHiddenIdentity(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.UserSession{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))

	alice := model.User{Username: "alice-id", DisplayName: "Alice", Password: "unused-password", Role: common.RoleCommonUser, Group: "default", AffCode: "aliceid1"}
	require.NoError(t, database.Create(&alice).Error)
	body := func(group, name string) string {
		return `{"id":` + strconv.Itoa(alice.Id) + `,"username":"alice-id","display_name":"` + name + `","role":1,"group":"` + group + `"}`
	}

	hidden := adtyCall(UpdateUser, http.MethodPut, "/api/user/", body("vip", "Alice"), 1, common.RoleAdminUser, nil)
	require.NotEqual(t, http.StatusOK, hidden.Code, hidden.Body.String())
	require.NotContains(t, hidden.Body.String(), "no such table")
	var stored model.User
	require.NoError(t, database.First(&stored, alice.Id).Error)
	require.Equal(t, "default", stored.Group)

	autoGroup := adtyCall(UpdateUser, http.MethodPut, "/api/user/", body("auto", "Alice"), 1, common.RoleAdminUser, nil)
	require.NotEqual(t, http.StatusOK, autoGroup.Code, autoGroup.Body.String())
	require.NoError(t, database.First(&stored, alice.Id).Error)
	require.Equal(t, "default", stored.Group)

	require.NoError(t, database.Model(&stored).Update("group", "vip").Error)
	kept := adtyCall(UpdateUser, http.MethodPut, "/api/user/", body("vip", "Kept"), 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusOK, kept.Code, kept.Body.String())
	require.NoError(t, database.First(&stored, alice.Id).Error)
	require.Equal(t, "vip", stored.Group)
	require.Equal(t, "Kept", stored.DisplayName)

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	public := adtyCall(UpdateUser, http.MethodPut, "/api/user/", body("default", "Public"), 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusOK, public.Code, public.Body.String())
	require.NoError(t, database.First(&stored, alice.Id).Error)
	require.Equal(t, "default", stored.Group)
}

func TestSaveGroupBillingDropsStalePublicGroup(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}, &model.GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","channelonly":"Channel"}`))
	require.False(t, service.UserMayUseGroup(1, "default", "channelonly"))

	require.NoError(t, model.SaveGroupBillingTable(
		map[string]float64{"default": 1},
		map[string]string{"default": "Default", "channelonly": "Channel"},
		nil,
	))
	require.NotContains(t, setting.GetUserUsableGroupsCopy(), "channelonly")
	require.Equal(t, "Default", setting.GetUserUsableGroupsCopy()["default"])
}

func TestAdminCloseoutHidesDatabaseErrors(t *testing.T) {
	_ = adtyTestDB(t)
	inviters := adtyCall(AdminListAffInviters, http.MethodGet, "/api/admin/aff/inviters", "", 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusInternalServerError, inviters.Code, inviters.Body.String())
	require.NotContains(t, inviters.Body.String(), "no such table")
	require.Contains(t, inviters.Body.String(), `"code":"internal_error"`)

	grants := adtyCall(AdminListGroupGrants, http.MethodGet, "/api/admin/groups/vip/grants", "", 1, common.RoleAdminUser, gin.Params{{Key: "code", Value: "vip"}})
	require.Equal(t, http.StatusInternalServerError, grants.Code, grants.Body.String())
	require.NotContains(t, grants.Body.String(), "no such table")
	require.Contains(t, grants.Body.String(), `"code":"internal_error"`)

	groups := adtyCall(AdminGetGroups, http.MethodGet, "/api/admin/groups", "", 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusInternalServerError, groups.Code, groups.Body.String())
	require.NotContains(t, groups.Body.String(), "no such table")
	require.Contains(t, groups.Body.String(), `"code":"internal_error"`)

	put := adtyCall(AdminPutGroup, http.MethodPut, "/api/admin/groups/vip", `{"display_name":"VIP"}`, 1, common.RoleRootUser, gin.Params{{Key: "code", Value: "vip"}})
	require.Equal(t, http.StatusInternalServerError, put.Code, put.Body.String())
	require.NotContains(t, put.Body.String(), "no such table")
	require.Contains(t, put.Body.String(), `"code":"internal_error"`)

	post := adtyCall(AdminPostGroup, http.MethodPost, "/api/admin/groups", `{"code":"vip"}`, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusInternalServerError, post.Code, post.Body.String())
	require.NotContains(t, post.Body.String(), "no such table")
	require.Contains(t, post.Body.String(), `"code":"internal_error"`)
}

func readStoredGroupRatio(t *testing.T, database *gorm.DB) map[string]float64 {
	t.Helper()
	var stored model.Option
	require.NoError(t, database.Where(&model.Option{Key: "GroupRatio"}).First(&stored).Error)
	parsed := map[string]float64{}
	require.NoError(t, common.Unmarshal([]byte(stored.Value), &parsed))
	return parsed
}

func TestCreateGroupWithRatioKeepsDatabaseKeys(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}, &model.GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, database.Create(&model.Option{Key: "GroupRatio", Value: `{"default":1,"partner":2}`}).Error)

	_, err := model.CreateGroupWithRatio(database, model.GroupMetaCreateInput{Code: "svip", DisplayName: "SVIP"})
	require.NoError(t, err)

	stored := readStoredGroupRatio(t, database)
	require.Equal(t, 1.0, stored["default"])
	require.Equal(t, 2.0, stored["partner"])
	require.Equal(t, 1.0, stored["svip"])
	require.Equal(t, 2.0, ratio_setting.GetGroupRatioCopy()["partner"])
	require.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()["svip"])
}

func TestSaveGroupBillingTableDoesNotDropDatabaseKeys(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}, &model.GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP"}`))
	require.NoError(t, database.Create(&model.Option{Key: "GroupRatio", Value: `{"vip":2,"partner":3}`}).Error)

	err := model.SaveGroupBillingTable(map[string]float64{"vip": 0}, map[string]string{}, nil)
	require.ErrorIs(t, err, model.ErrGroupBillingKeySet)
	stored := readStoredGroupRatio(t, database)
	require.Equal(t, 2.0, stored["vip"])
	require.Equal(t, 3.0, stored["partner"])
	require.Equal(t, 3.0, ratio_setting.GetGroupRatioCopy()["partner"])
}

func TestCreateGroupWithRatioConcurrentKeepsBothCodes(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}, &model.GroupMeta{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, code := range []string{"alpha", "beta"} {
		wg.Add(1)
		go func(code string) {
			defer wg.Done()
			_, err := model.CreateGroupWithRatio(database, model.GroupMetaCreateInput{Code: code, DisplayName: code})
			errs <- err
		}(code)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	stored := readStoredGroupRatio(t, database)
	require.Equal(t, 1.0, stored["default"])
	require.Equal(t, 1.0, stored["alpha"])
	require.Equal(t, 1.0, stored["beta"])
}

func TestUpdateOptionCannotChangeBillingGroupSet(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Option{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	common.OptionMapRWMutex.Lock()
	previousRatio, hadRatio := common.OptionMap["GroupRatio"]
	previousUsable, hadUsable := common.OptionMap["UserUsableGroups"]
	common.OptionMap["GroupRatio"] = `{"default":1}`
	common.OptionMap["UserUsableGroups"] = `{"default":"Default"}`
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if hadRatio {
			common.OptionMap["GroupRatio"] = previousRatio
		} else {
			delete(common.OptionMap, "GroupRatio")
		}
		if hadUsable {
			common.OptionMap["UserUsableGroups"] = previousUsable
		} else {
			delete(common.OptionMap, "UserUsableGroups")
		}
	})

	ratio := adtyCall(UpdateOption, http.MethodPut, "/api/option/", `{"key":"GroupRatio","value":"{\"default\":1,\"hidden\":2}"}`, 1, common.RoleRootUser, nil)
	require.Contains(t, ratio.Body.String(), "groups API", ratio.Body.String())
	require.NotContains(t, ratio.Body.String(), "hidden")
	require.NotContains(t, ratio_setting.GetGroupRatioCopy(), "hidden")

	usable := adtyCall(UpdateOption, http.MethodPut, "/api/option/", `{"key":"UserUsableGroups","value":"{\"default\":\"Default\",\"vip\":\"VIP\"}"}`, 1, common.RoleRootUser, nil)
	require.Contains(t, usable.Body.String(), "groups API", usable.Body.String())
	require.NotContains(t, setting.GetUserUsableGroupsCopy(), "vip")

	require.ErrorIs(t, model.UpdateOption("GroupRatio", `{"default":1,"hidden":2}`), model.ErrBillingGroupOptionLocked)
	require.NotContains(t, ratio_setting.GetGroupRatioCopy(), "hidden")
	require.ErrorIs(t, model.UpdateOptionsBulk(map[string]string{
		"UserUsableGroups": `{"default":"Default","vip":"VIP"}`,
	}), model.ErrBillingGroupOptionLocked)
	require.NotContains(t, setting.GetUserUsableGroupsCopy(), "vip")
}

func TestUpdateUserHidesDatabaseError(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}))
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))

	alice := model.User{Username: "alice-db", DisplayName: "Alice", Password: "unused-password", Role: common.RoleCommonUser, Group: "default", AffCode: "alicedb1"}
	other := model.User{Username: "taken-name", DisplayName: "Other", Password: "unused-password", Role: common.RoleCommonUser, Group: "default", AffCode: "takendb1"}
	require.NoError(t, database.Create(&alice).Error)
	require.NoError(t, database.Create(&other).Error)
	body := `{"id":` + strconv.Itoa(alice.Id) + `,"username":"taken-name","display_name":"Alice","role":1,"group":"default"}`

	rec := adtyCall(UpdateUser, http.MethodPut, "/api/user/", body, 1, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"code":"internal_error"`)
	require.NotContains(t, rec.Body.String(), "UNIQUE")
	require.NotContains(t, rec.Body.String(), "constraint")
	require.NotContains(t, rec.Body.String(), "taken-name")
	var stored model.User
	require.NoError(t, database.First(&stored, alice.Id).Error)
	require.Equal(t, "alice-db", stored.Username)
}

func TestAffOverviewWithdrawalCountsExcludeDeletedInviters(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.AffCommission{}, &model.AffWithdrawal{}))

	live := model.User{Username: "live-withdraw", Password: "unused", AffCode: "wdLive"}
	gone := model.User{Username: "gone-withdraw", Password: "unused", AffCode: "wdGone"}
	require.NoError(t, database.Create(&live).Error)
	require.NoError(t, database.Create(&gone).Error)
	require.NoError(t, database.Delete(&gone).Error)
	require.NoError(t, database.Create(&model.AffWithdrawal{
		UserId: live.Id, AmountQuota: 40, NetQuota: 40, IdemKey: "wd-live-pending", Status: model.AffWithdrawStatusPending, PayMethod: model.AffPayMethodUSDT,
	}).Error)
	require.NoError(t, database.Create(&model.AffWithdrawal{
		UserId: gone.Id, AmountQuota: 90, NetQuota: 90, IdemKey: "wd-gone-pending", Status: model.AffWithdrawStatusPending, PayMethod: model.AffPayMethodUSDT,
	}).Error)
	require.NoError(t, database.Create(&model.AffWithdrawal{
		UserId: live.Id, AmountQuota: 15, NetQuota: 15, IdemKey: "wd-live-paid", Status: model.AffWithdrawStatusPaid, PayMethod: model.AffPayMethodAlipay,
	}).Error)
	require.NoError(t, database.Create(&model.AffWithdrawal{
		UserId: gone.Id, AmountQuota: 70, NetQuota: 70, IdemKey: "wd-gone-paid", Status: model.AffWithdrawStatusPaid, PayMethod: model.AffPayMethodAlipay,
	}).Error)

	overview, err := model.GetAffOverview()
	require.NoError(t, err)
	require.Equal(t, int64(1), overview.WithdrawPendingUSDT)
	require.Equal(t, int64(0), overview.WithdrawPendingAlipay)
	require.Equal(t, int64(0), overview.WithdrawPendingLegacy)
	require.Equal(t, int64(15), overview.WithdrawPaidQuota)
}

func TestAdminAffInviteeSummaryOmitsVoidedMoneyAndKeepsVoidedStatus(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.AffCommission{}))

	inviter := model.User{Username: "void-sum-inviter", Password: "unused", AffCode: "voidsum"}
	invitee := model.User{Username: "void-sum-invitee", Password: "unused", AffCode: "voidsume"}
	require.NoError(t, database.Create(&inviter).Error)
	invitee.InviterId = inviter.Id
	require.NoError(t, database.Create(&invitee).Error)
	require.NoError(t, database.Create(&model.AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: "void-sum-live",
		Source: "topup", PaidAmountUSD: 10, CommissionQuota: 100, Status: model.AffCommissionStatusAvailable,
	}).Error)
	require.NoError(t, database.Create(&model.AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: "void-sum-dead",
		Source: "topup", PaidAmountUSD: 40, CommissionQuota: 400, Status: model.AffCommissionStatusVoided,
	}).Error)

	rows, total, err := model.ListAffInvitees(inviter.Id, 20, 0, false)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.InDelta(t, 10, rows[0].PaidUSD, 0.0001)
	require.Equal(t, int64(100), rows[0].CommissionQuota)
	require.Contains(t, rows[0].Status, model.AffCommissionStatusAvailable)
	require.Contains(t, rows[0].Status, model.AffCommissionStatusVoided)
}
