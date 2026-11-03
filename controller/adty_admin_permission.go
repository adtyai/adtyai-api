package controller

import (
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// AdtyPermissionDeniedCode 标出是二开的后台权限拒绝了请求，和上游的安全验证等 403 区分开。
const AdtyPermissionDeniedCode = "ADTY_PERMISSION_DENIED"

func denyAdtyAdminAction(c *gin.Context) {
	common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"), gin.H{"code": AdtyPermissionDeniedCode})
}

// adtyCheckManageAction 按 ManageUser 的动作细分权限：启用、禁用要用户·编辑，删除要用户·删除，
// 调余额要用户·调余额；提升、降级沿用上游规则（只有超级管理员，由 canManageTargetRole 等检查）。
// 被拒时已写好 403。
func adtyCheckManageAction(c *gin.Context, req ManageRequest) bool {
	userID, role := c.GetInt("id"), c.GetInt("role")
	var permission authz.Permission
	switch req.Action {
	case "enable", "disable":
		permission = authz.AdtyUserWrite
	case "delete":
		permission = authz.AdtyUserDelete
	case "add_quota":
		permission = authz.AdtyUserQuota
	default:
		return true
	}
	if service.AdtyAdminRoleCan(userID, role, permission) {
		return true
	}
	denyAdtyAdminAction(c)
	return false
}

// adtyCheckUserUpdate 在 UpdateUser 里检查分组：EditWithTx 会无条件写分组，
// 所以请求里的分组和原值不同（没带或为空也算）时，要用户·改分组，并且目标分组在分组表里。
// 被拒时已写好响应。
func adtyCheckUserUpdate(c *gin.Context, origin *model.User, updated *model.User) bool {
	if updated.Group == origin.Group {
		return true
	}
	if !service.AdtyAdminRoleCan(c.GetInt("id"), c.GetInt("role"), authz.AdtyUserGroup) {
		denyAdtyAdminAction(c)
		return false
	}
	if !service.AdtyAdminPermissionsEnabled() {
		return true
	}
	active, err := model.ActiveAdtyGroupIDs()
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	if _, ok := active[updated.Group]; !ok {
		common.ApiErrorT(c, "Group not found")
		return false
	}
	return true
}

// adtyPermissionChange 记下改权限前后的能力，提交成功后写一条差异审计。
type adtyPermissionChange struct {
	userID int
	role   int
	before authz.PermissionsMap
}

func startAdtyPermissionChange(userID, role int) *adtyPermissionChange {
	if role < common.RoleAdminUser {
		return nil
	}
	return &adtyPermissionChange{userID: userID, role: role, before: authz.Capabilities(userID, role)}
}

// recordAdtyPermissionChange 在 ReloadPolicy 之后调用，比较前后能力，有变化才写审计。
func recordAdtyPermissionChange(c *gin.Context, change *adtyPermissionChange) {
	if change == nil {
		return
	}
	after := authz.Capabilities(change.userID, change.role)
	var added, removed []string
	for resource, actions := range after {
		for action, allowed := range actions {
			was := change.before[resource][action]
			key := resource + ":" + action
			if allowed && !was {
				added = append(added, key)
			}
			if !allowed && was {
				removed = append(removed, key)
			}
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	slices.Sort(added)
	slices.Sort(removed)
	recordManageAuditFor(c, change.userID, "adty.admin_permissions.update", map[string]any{
		"id":      change.userID,
		"added":   added,
		"removed": removed,
	})
}
