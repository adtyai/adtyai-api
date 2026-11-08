package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// EnableAdtyAdminPermissions 登记二开的权限项、清空内置管理员的默认权限，并打开后台路由检查。
// 只有 main 在 authz.Init 之前调用（任务 2.6）。返回的函数恢复原状，给测试用。
func EnableAdtyAdminPermissions() (restore func()) {
	restoreBaseline := authz.AdtyClearAdminBaseline()
	authz.AdtyRegisterResources()
	service.SetAdtyAdminPermissionsEnabled(true)
	return func() {
		service.SetAdtyAdminPermissionsEnabled(false)
		restoreBaseline()
	}
}

// AdtyAdminCan 判断管理员有没有这个权限，用在只按角色放行、路由检查覆盖不到的地方。
// 检查没打开时（上游测试）总是 true；超级管理员总是 true。
func AdtyAdminCan(userID int, permission authz.Permission) bool {
	return service.AdtyAdminCan(userID, permission)
}

// adtyScopePermissions 把上游的访问令牌分类对应到二开的权限，有其中一项就放行。
var adtyScopePermissions = map[string][]authz.Permission{
	"user:read":    {authz.AdtyUserRead},
	"user:write":   {authz.AdtyUserWrite},
	"billing:read": {authz.AdtyFinanceRead},
	// 后台「手动完成」只对上游支付商的订单有效，我们的订单走充值对账（任务 3.2）。
	"billing:write": {authz.AdtyFinanceManualCredit},
	"log:read":      {authz.AdtyLogRead},
	"model:read":    {authz.AdtyModelRead},
	"model:write":   {authz.AdtyModelWrite},
	// 只是分组下拉的数据，几个后台页面都要用。
	"group:read": {authz.AdtyUserRead, authz.AdtyLogRead, authz.AdtyModelRead, authz.ChannelRead},
	// 预填分组的增删改界面在模型页。
	"group:write": {authz.AdtyModelWrite},
}

// AdtyRootOnlyAdminScopes 是只给超级管理员的上游后台分类，合并上游时新出现的分类要归到这里或上表。
var AdtyRootOnlyAdminScopes = []string{"deployment:read", "deployment:write"}

// adtyAdminRouteAllowed 在 authHelper 里、身份写进上下文之后调用，只处理管理员级别的接口。
// 被拒时已经写好 403 响应。
func adtyAdminRouteAllowed(c *gin.Context, minRole int) bool {
	if !service.AdtyAdminPermissionsEnabled() || minRole != common.RoleAdminUser {
		return true
	}
	role := c.GetInt("role")
	if role >= common.RoleRootUser {
		return true
	}
	userID := c.GetInt("id")
	key := c.Request.Method + " " + c.FullPath()
	if adtyAdminRouteCan(c, userID, role, key) {
		return true
	}
	common.SysLog(common.LogText("admin route denied by permissions: user %d %s", userID, key))
	abortWithWebMessage(c, http.StatusForbidden, "ADTY_PERMISSION_DENIED", common.NewMessage("Unauthorized, insufficient privileges"))
	return false
}

func adtyAdminRouteCan(c *gin.Context, userID, role int, key string) bool {
	can := func(permissions ...authz.Permission) bool {
		for _, permission := range permissions {
			if !authz.Can(userID, role, permission) {
				continue
			}
			legacy, isToken := c.Get("access_token_legacy")
			if !isToken || legacy == true {
				return true
			}
			scopes, _ := c.Get("access_token_scopes")
			granted, _ := scopes.([]string)
			if service.AccessTokenScopeAllowsRoute(granted, c.GetInt("access_token_scope_version"), role, c.Request.Method, c.FullPath(), service.AccessTokenScopeOf(permission)) {
				return true
			}
		}
		return false
	}
	switch key {
	case "GET /api/group/":
		return can(authz.AdtyUserRead, authz.AdtyLogRead, authz.AdtyModelRead, authz.ChannelRead)
	case "POST /api/models/sync_upstream":
		return can(authz.AdtyModelCreate, authz.AdtyModelWrite)
	case "POST /api/vendors/operations":
		return can(authz.AdtyModelWrite, authz.AdtyModelDelete)
	case "POST /api/user/manage":
		// 动作由 controller 里的 AdtyCheckManageAction 细分。
		return can(authz.AdtyUserWrite, authz.AdtyUserQuota, authz.AdtyUserDelete)
	case "DELETE /api/user/:id":
		return can(authz.AdtyUserDelete)
	case "DELETE /api/models/:id":
		if adtyRemoveFromChannels(c.Query("remove_from_channels")) {
			return can(authz.AdtyModelDelete) && can(authz.ChannelWrite)
		}
		return can(authz.AdtyModelDelete)
	case "POST /api/models/delete":
		if adtyBatchRemovesFromChannels(c) {
			return can(authz.AdtyModelDelete) && can(authz.ChannelWrite)
		}
		return can(authz.AdtyModelDelete)
	}
	rule, ok := AccessTokenRouteRule(key)
	if !ok || rule.Kind() != "scope" {
		return false
	}
	scope := rule.Scope()
	// Static routes can also name Casbin actions without a later guard.
	for _, permission := range authz.AllPermissions() {
		if service.AccessTokenScopeOf(permission) == scope {
			return can(permission)
		}
	}
	permissions, ok := adtyScopePermissions[scope]
	if !ok {
		return false
	}
	return can(permissions...)
}

func adtyRemoveFromChannels(raw string) bool {
	remove, err := strconv.ParseBool(raw)
	return err == nil && remove
}

// adtyBatchRemovesFromChannels 读批量删除请求体里的 remove_from_channels，请求体留给后面的处理函数。
// 读不了时按会改渠道处理，权限要求更高。
func adtyBatchRemovesFromChannels(c *gin.Context) bool {
	contentType := c.GetHeader("Content-Type")
	// 非 JSON 时 UnmarshalBodyReusable 会跳过正文，字段保持 false，后面的处理函数仍可能按 JSON 删渠道。
	if !strings.HasPrefix(contentType, "application/json") {
		return true
	}
	var request struct {
		RemoveFromChannels bool `json:"remove_from_channels"`
	}
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return true
	}
	return request.RemoveFromChannels
}

// AdtyAdminScopeClassified 说明这个上游后台分类已经对应到二开权限。
func AdtyAdminScopeClassified(scope string) bool {
	_, ok := adtyScopePermissions[scope]
	return ok
}
