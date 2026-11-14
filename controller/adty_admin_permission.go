package controller

import (
	"errors"
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdtyPermissionDeniedCode 标出是二开的后台权限拒绝了请求，和上游的安全验证等 403 区分开。
const AdtyPermissionDeniedCode = "ADTY_PERMISSION_DENIED"

var errAdtyAdminActionDenied = errors.New("admin action permission denied")

// adtyAuthorizeAdminActions checks every action of a compound operation. Token
// scopes constrain the live user grant; neither can substitute for the other.
func adtyAuthorizeAdminActions(c *gin.Context, permissions ...authz.Permission) error {
	if !service.AdtyAdminPermissionsEnabled() {
		return nil
	}
	for _, permission := range permissions {
		if !service.AdtyAdminRoleCan(c.GetInt("id"), c.GetInt("role"), permission) {
			return errAdtyAdminActionDenied
		}
	}
	legacy, isToken := c.Get("access_token_legacy")
	if !isToken {
		return nil
	}
	if legacy == true {
		return errAdtyAdminActionDenied
	}
	scopes, _ := c.Get("access_token_scopes")
	granted, _ := scopes.([]string)
	for _, permission := range permissions {
		if !service.AccessTokenScopeAllowsRoute(granted, c.GetInt("access_token_scope_version"), c.GetInt("role"), c.Request.Method, c.FullPath(), service.AccessTokenScopeOf(permission)) {
			return errAdtyAdminActionDenied
		}
	}
	return nil
}

func denyAdtyAdminAction(c *gin.Context) {
	common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"), gin.H{"code": AdtyPermissionDeniedCode})
}

// adtyAccessTokenMayManage applies token scopes that the single manage route
// cannot express. A browser session has no access-token context and is unchanged.
func adtyAccessTokenMayManage(c *gin.Context, action string) bool {
	legacy, isToken := c.Get("access_token_legacy")
	if !isToken {
		return true
	}
	if legacy == true {
		return false
	}
	scopes, _ := c.Get("access_token_scopes")
	granted, _ := scopes.([]string)
	switch action {
	case "add_quota":
		return slices.Contains(granted, service.AccessTokenScopeOf(authz.AdtyUserQuota))
	case "promote", "demote":
		return false
	case "delete":
		return slices.Contains(granted, service.AccessTokenScopeOf(authz.AdtyUserDelete))
	default:
		return slices.Contains(granted, service.AccessTokenScopeOf(authz.AdtyUserWrite))
	}
}

// adtyCheckManageAction 按 ManageUser 的动作细分权限：启用、禁用要用户·编辑，删除要用户·删除，
// 调余额要用户·调余额；提升、降级沿用上游规则（只有超级管理员，由 canManageTargetRole 等检查）。
// 被拒时已写好 403。
func adtyCheckManageAction(c *gin.Context, req ManageRequest) bool {
	if !adtyAccessTokenMayManage(c, req.Action) {
		denyAdtyAdminAction(c)
		return false
	}
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
	if err := adtyAuthorizeAdminActions(c, authz.AdtyUserGroup); err != nil {
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

// appendAdtyPermissionAuditTx freezes the actual transaction's policy diff;
// post-commit reload/cache failures cannot erase a committed permission change.
func appendAdtyPermissionAuditTx(c *gin.Context, tx *gorm.DB, userID, role int, before authz.PermissionsMap, patch map[string]map[string]bool) (model.AuditLog, error) {
	after, err := authz.CapabilitiesInTx(tx, userID, role)
	if err != nil {
		return model.AuditLog{}, err
	}
	var added, removed []string
	for resource, actions := range after {
		for action, allowed := range actions {
			was := before[resource][action]
			key := resource + ":" + action
			if allowed && !was {
				added = append(added, key)
			}
			if !allowed && was {
				removed = append(removed, key)
			}
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	event := prepareManageAuditChange(c, userID, "adty.admin_permissions.update", map[string]any{
		"id":      userID,
		"added":   added,
		"removed": removed,
		"patch":   patch,
	}, before, after, "", "")
	return event, model.AppendAuditIntentTx(tx, event)
}
