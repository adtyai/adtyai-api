package service

import (
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
)

// adtyAdminPermissionsEnabled 由 middleware.EnableAdtyAdminPermissions 打开。
// 上游测试不打开它，保持上游「管理员都能用」的行为。
var adtyAdminPermissionsEnabled atomic.Bool

// SetAdtyAdminPermissionsEnabled 打开或关闭二开的后台权限检查。
func SetAdtyAdminPermissionsEnabled(enabled bool) {
	adtyAdminPermissionsEnabled.Store(enabled)
}

// AdtyAdminPermissionsEnabled 说明二开的后台权限检查是否打开。
func AdtyAdminPermissionsEnabled() bool {
	return adtyAdminPermissionsEnabled.Load()
}

// AdtyAdminCan 用在只按角色放行、路由检查覆盖不到的地方：检查没打开时总是 true，
// 超级管理员总是 true，其余按 Casbin 权限判断。
//
// 拿不到用户角色时一律拒绝（fail closed），不回退到任何角色的基线：
// GetUserCache 的失败里包含 ErrUserAuthCachePending——那正是「收紧权限的更新
// 尚未落库」的栅栏，此刻按管理员基线放行等于绕过它。角色已知时用
// AdtyAdminRoleCan 并把可信 role 传进来，不要让本函数去猜一个最宽松的默认值。
func AdtyAdminCan(userID int, permission authz.Permission) bool {
	if !adtyAdminPermissionsEnabled.Load() {
		return true
	}
	user, err := model.GetUserCache(userID)
	if err != nil {
		common.SysError(common.LogText("adty admin permission check failed closed: user=%d err=%s", userID, err.Error()))
		return false
	}
	return authz.Can(userID, user.Role, permission)
}

// AdtyAdminRoleCan 和 AdtyAdminCan 一样，但角色已知，不再查用户。
func AdtyAdminRoleCan(userID, role int, permission authz.Permission) bool {
	if !adtyAdminPermissionsEnabled.Load() {
		return true
	}
	return authz.Can(userID, role, permission)
}
