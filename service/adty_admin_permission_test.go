package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// loadAdminBaselineForTest 把 authz 加载成「管理员基线放行 ChannelRead」的状态。
// 没有 enforcer 时 authz.Can 对所有角色都返回 false，那样「回退到管理员」
// 就不可观测，本条用例也就验不出方向。
func loadAdminBaselineForTest(t *testing.T) {
	t.Helper()
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = wasMaster })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}, &model.AuthzActionCatalogState{}))
	require.NoError(t, authz.Init(db))
}

// 拿不到角色必须拒绝：GetUserCache 的错误里包含 ErrUserAuthCachePending，
// 那是「收紧权限的更新尚未落库」的栅栏，回退到管理员基线等于绕过它。
func TestAdtyAdminCanDeniesWhenTheRoleCannotBeRead(t *testing.T) {
	previous := AdtyAdminPermissionsEnabled()
	SetAdtyAdminPermissionsEnabled(true)
	t.Cleanup(func() { SetAdtyAdminPermissionsEnabled(previous) })
	loadAdminBaselineForTest(t)

	require.True(t, authz.Can(0, common.RoleAdminUser, authz.ChannelRead),
		"用例前提：管理员基线放行该权限，否则「回退到管理员」观察不到")

	const unknownUserID = 919191
	_, err := model.GetUserCache(unknownUserID)
	require.Error(t, err, "用例前提：这个用户的角色读不出来")

	assert.False(t, AdtyAdminCan(unknownUserID, authz.ChannelRead),
		"拿不到角色必须拒绝，不能按管理员基线放行（ISS-R4-0080）")
}

// 能读到角色时照旧按该角色判定——fail closed 只改「不知道角色」那一条分支。
func TestAdtyAdminCanUsesTheCachedRole(t *testing.T) {
	previous := AdtyAdminPermissionsEnabled()
	SetAdtyAdminPermissionsEnabled(true)
	t.Cleanup(func() { SetAdtyAdminPermissionsEnabled(previous) })
	loadAdminBaselineForTest(t)

	user := &model.User{
		Id:       919192,
		Username: "w51_permission_admin",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	t.Cleanup(func() { model.DB.Exec("DELETE FROM users WHERE id = ?", user.Id) })

	assert.True(t, AdtyAdminCan(user.Id, authz.ChannelRead), "管理员基线放行 ChannelRead")
	assert.False(t, AdtyAdminCan(user.Id, authz.ChannelSensitiveWrite), "管理员基线不含敏感写")
}
