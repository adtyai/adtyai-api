package router

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// enableAdtyAdminGuard 打开后台权限检查并在主节点模式下重新 Init，结束时恢复上游状态。
func enableAdtyAdminGuard(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}, &model.AuthzActionCatalogState{}))
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	restore := middleware.EnableAdtyAdminPermissions()
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		restore()
		require.NoError(t, authz.Init(db))
		common.IsMasterNode = wasMaster
	})
}

type adtyGuardFixture struct {
	db     *gorm.DB
	engine *gin.Engine
	admin  model.User
	token  string
	root   string
}

func newAdtyGuardFixture(t *testing.T) *adtyGuardFixture {
	t.Helper()
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.Channel{}, &model.Ability{},
		&model.AuditLog{}, &model.AuditIntent{}, &model.Log{}, &model.Option{}, &model.TopUp{}, &model.UserAccessToken{}))
	require.NoError(t, model.MigrateAdtyTables())
	require.NoError(t, model.SeedAdtyGroups())
	enableAdtyAdminGuard(t, db)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	token := adtySessionToken(t, db, "guard-admin", common.RoleAdminUser)
	var admin model.User
	require.NoError(t, db.Where("username = ?", "guard-admin").First(&admin).Error)
	root := adtySessionToken(t, db, "guard-root", common.RoleRootUser)
	return &adtyGuardFixture{db: db, engine: engine, admin: admin, token: token, root: root}
}

func createAdtyScopedToken(t *testing.T, userID int, scopes ...string) string {
	t.Helper()
	suffix, err := common.GenerateRandomCharsKey(43)
	require.NoError(t, err)
	raw := model.AccessTokenPrefix + suffix
	accessToken := &model.UserAccessToken{Name: "guard pat", TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw)}
	require.NoError(t, accessToken.SetScopes(scopes))
	require.NoError(t, model.CreateUserAccessToken(userID, accessToken, service.AccessTokenMaxPerUser))
	return raw
}

func (f *adtyGuardFixture) grant(t *testing.T, permissions ...authz.Permission) {
	t.Helper()
	grants := authz.PermissionsMap{}
	for _, resource := range authz.Catalog() {
		actions := map[string]bool{}
		for _, action := range resource.Actions {
			actions[action.Action] = slices.Contains(permissions, authz.Permission{Resource: resource.Resource, Action: action.Action})
		}
		grants[resource.Resource] = actions
	}
	require.NoError(t, authz.SetUserPermissions(f.admin.Id, grants))
}

func TestAdtyResourcesHaveNoDefaultRoles(t *testing.T) {
	restore := middleware.EnableAdtyAdminPermissions()
	t.Cleanup(restore)
	for _, resource := range authz.Catalog() {
		for _, action := range resource.Actions {
			assert.Empty(t, authz.PermissionsForRole(authz.BuiltInRoleAdmin), resource.Resource+":"+action.Action)
		}
	}
	var adty []string
	for _, resource := range authz.Catalog() {
		if strings.HasPrefix(resource.Resource, "adty_") {
			adty = append(adty, resource.Resource)
		}
	}
	assert.ElementsMatch(t, []string{"adty_user", "adty_log", "adty_model", "adty_finance", "adty_referral", "adty_payment", "adty_site"}, adty)
}

func TestAdminHasNoPermissionByDefault(t *testing.T) {
	f := newAdtyGuardFixture(t)
	for _, path := range []string{"/api/channel/", "/api/user/", "/api/log/", "/api/models/"} {
		resp := adtyJSON(t, f.engine, http.MethodGet, path, "", f.token)
		assert.Equal(t, http.StatusForbidden, resp.Code, path)
	}
	assert.False(t, authz.Can(f.admin.Id, common.RoleAdminUser, authz.ChannelRead))
}

func TestGrantAndRevokeTakeEffectImmediately(t *testing.T) {
	f := newAdtyGuardFixture(t)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/user/", "", f.token).Code)
	f.grant(t, authz.AdtyUserRead)
	assert.Equal(t, http.StatusOK, adtyJSON(t, f.engine, http.MethodGet, "/api/user/", "", f.token).Code)
	f.grant(t)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/user/", "", f.token).Code)
}

func TestRootBypassesGuard(t *testing.T) {
	f := newAdtyGuardFixture(t)
	for _, path := range []string{"/api/channel/", "/api/user/", "/api/log/", "/api/option/"} {
		resp := adtyJSON(t, f.engine, http.MethodGet, path, "", f.root)
		assert.NotEqual(t, http.StatusForbidden, resp.Code, path)
	}
}

func TestRootSystemScopeCannotBeGrantedToAdmin(t *testing.T) {
	f := newAdtyGuardFixture(t)
	all := make([]authz.Permission, 0)
	all = append(all, authz.AllPermissions()...)
	f.grant(t, all...)
	resp := adtyJSON(t, f.engine, http.MethodGet, "/api/option/", "", f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.Equal(t, http.StatusOK, adtyJSON(t, f.engine, http.MethodGet, "/api/user/", "", f.token).Code)
}

func TestRetiredDeploymentEndpointsReturnNotFound(t *testing.T) {
	f := newAdtyGuardFixture(t)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"model_deployment.ionet.enabled": "true",
		"model_deployment.ionet.api_key": "historical-deployment-key",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	var rootUser model.User
	require.NoError(t, f.db.Where("username = ?", "guard-root").First(&rootUser).Error)
	legacyPAT := createAdtyScopedToken(t, rootUser.Id, "deployment:read", "deployment:write")
	for _, token := range []string{"", f.token, f.root, legacyPAT} {
		for _, endpoint := range []struct{ method, path string }{
			{http.MethodGet, "/api/deployments/settings"},
			{http.MethodGet, "/api/deployments/"},
			{http.MethodPost, "/api/deployments/"},
			{http.MethodPut, "/api/deployments/old"},
			{http.MethodDelete, "/api/deployments/old"},
			{http.MethodGet, "/api/deployments/old/containers"},
		} {
			resp := adtyJSON(t, f.engine, endpoint.method, endpoint.path, `{}`, token)
			assert.Equal(t, http.StatusNotFound, resp.Code, endpoint.path)
		}
	}
	for _, key := range []string{"model_deployment.ionet.enabled", "model_deployment.ionet.api_key"} {
		require.NoError(t, f.db.Create(&model.Option{Key: key, Value: "historical"}).Error)
		resp := adtyJSON(t, f.engine, http.MethodPut, "/api/option/", fmt.Sprintf(`{"key":%q,"value":"true"}`, key), f.root)
		assert.Contains(t, resp.Body.String(), `"success":false`)
		var stored model.Option
		require.NoError(t, f.db.Where("key = ?", key).First(&stored).Error)
		assert.Equal(t, "historical", stored.Value)
	}
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, "/api/option/", "", f.root).Body.String(), "model_deployment.")
}

func TestUserWriteAccessTokenCannotAdjustQuotaOrPromote(t *testing.T) {
	f := newAdtyGuardFixture(t)
	require.NoError(t, model.EnsureLegacyAccessTokenRetireAt(time.Now().Unix()))
	f.grant(t, authz.AdtyUserWrite, authz.AdtyUserQuota)
	target := model.User{Username: "pat-target", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&target).Error)
	raw := createAdtyScopedToken(t, f.admin.Id, "user:write")

	adjust := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":500,"reason":"pat","idempotency_key":"pat-quota-1"}`, target.Id), raw)
	assert.Equal(t, http.StatusForbidden, adjust.Code, adjust.Body.String())
	promote := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"promote"}`, target.Id), raw)
	assert.Equal(t, http.StatusForbidden, promote.Code, promote.Body.String())

	var stored model.User
	require.NoError(t, f.db.First(&stored, target.Id).Error)
	assert.EqualValues(t, 0, stored.Quota)
	assert.Equal(t, common.RoleCommonUser, stored.Role)

	quotaToken := createAdtyScopedToken(t, f.admin.Id, service.AccessTokenScopeOf(authz.AdtyUserQuota))
	allowed := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":500,"reason":"pat","idempotency_key":"pat-quota-2"}`, target.Id), quotaToken)
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
}

func TestFinancePresetCanAdjustQuota(t *testing.T) {
	f := newAdtyGuardFixture(t)
	f.grant(t, authz.AdtyUserRead, authz.AdtyUserQuota)
	target := model.User{Username: "guard-target", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&target).Error)

	adjust := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":500,"reason":"test","idempotency_key":"finance-1"}`, target.Id), f.token)
	require.Equal(t, http.StatusOK, adjust.Code, adjust.Body.String())
	assert.Contains(t, adjust.Body.String(), `"success":true`)
	var stored model.User
	require.NoError(t, f.db.First(&stored, target.Id).Error)
	assert.EqualValues(t, 500, stored.Quota)

	disable := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"disable"}`, target.Id), f.token)
	assert.Equal(t, http.StatusForbidden, disable.Code)
	require.NoError(t, f.db.First(&stored, target.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, stored.Status)
}

func TestManageUserActionPermissions(t *testing.T) {
	f := newAdtyGuardFixture(t)
	target := model.User{Username: "guard-manage", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&target).Error)
	denied := func(method, path, body string) bool {
		resp := adtyJSON(t, f.engine, method, path, body, f.token)
		return resp.Code == http.StatusForbidden && strings.Contains(resp.Body.String(), `"ADTY_PERMISSION_DENIED"`)
	}
	manage := func(action string) bool {
		return denied(http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":%q,"mode":"add","value":1}`, target.Id, action))
	}

	assert.True(t, manage("enable"), "no permission at all")

	f.grant(t, authz.AdtyUserWrite)
	assert.False(t, manage("enable"))
	assert.False(t, manage("disable"))
	assert.True(t, manage("add_quota"))
	assert.True(t, manage("delete"))
	assert.True(t, denied(http.MethodDelete, fmt.Sprintf("/api/user/%d", target.Id), ""))

	f.grant(t, authz.AdtyUserDelete)
	assert.False(t, manage("delete"))
	assert.True(t, manage("enable"))
	assert.False(t, denied(http.MethodDelete, fmt.Sprintf("/api/user/%d", target.Id), ""))
}

func TestUpdateUserGroupNeedsGroupPermission(t *testing.T) {
	f := newAdtyGuardFixture(t)
	f.grant(t, authz.AdtyUserRead, authz.AdtyUserWrite)
	target := model.User{Username: "guard-group", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&target).Error)
	update := func(group string) string {
		return fmt.Sprintf(`{"id":%d,"username":"guard-group","display_name":"x","group":%q}`, target.Id, group)
	}

	same := adtyJSON(t, f.engine, http.MethodPut, "/api/user/", update("default"), f.token)
	assert.Contains(t, same.Body.String(), `"success":true`, same.Body.String())
	moved := adtyJSON(t, f.engine, http.MethodPut, "/api/user/", update("vip"), f.token)
	assert.Equal(t, http.StatusForbidden, moved.Code)
	emptied := adtyJSON(t, f.engine, http.MethodPut, "/api/user/", update(""), f.token)
	assert.Equal(t, http.StatusForbidden, emptied.Code, "a missing group still overwrites the column")

	f.grant(t, authz.AdtyUserRead, authz.AdtyUserWrite, authz.AdtyUserGroup)
	unknown := adtyJSON(t, f.engine, http.MethodPut, "/api/user/", update("gnope"), f.token)
	assert.Contains(t, unknown.Body.String(), `"success":false`)
	moved = adtyJSON(t, f.engine, http.MethodPut, "/api/user/", update("vip"), f.token)
	assert.Contains(t, moved.Body.String(), `"success":true`, moved.Body.String())
	var stored model.User
	require.NoError(t, f.db.First(&stored, target.Id).Error)
	assert.Equal(t, "vip", stored.Group)
}

func TestRemoveFromChannelsNeedsChannelWrite(t *testing.T) {
	f := newAdtyGuardFixture(t)
	f.grant(t, authz.AdtyModelRead, authz.AdtyModelWrite, authz.ChannelWrite)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodDelete, "/api/models/1", "", f.token).Code, "model write cannot replace model delete")
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, "/api/models/delete", `{"ids":[1],"remove_from_channels":false}`, f.token).Code, "batch deletion requires model delete")
	f.grant(t, authz.AdtyModelRead, authz.AdtyModelDelete)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodDelete, "/api/models/1?remove_from_channels=true", "", f.token).Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, "/api/models/delete", `{"ids":[1],"remove_from_channels":true}`, f.token).Code)
	plain := httptest.NewRequest(http.MethodPost, "/api/models/delete", strings.NewReader(`{"model_ids":[1],"remove_from_channels":true}`))
	plain.Header.Set("Content-Type", "text/plain")
	plain.Header.Set("Authorization", "Bearer "+f.token)
	plainResp := httptest.NewRecorder()
	f.engine.ServeHTTP(plainResp, plain)
	assert.Equal(t, http.StatusForbidden, plainResp.Code, "a non-JSON content type cannot skip the channel write check")
	assert.NotEqual(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodDelete, "/api/models/1", "", f.token).Code)
	assert.NotEqual(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPost, "/api/models/delete", `{"ids":[1],"remove_from_channels":false}`, f.token).Code)

	f.grant(t, authz.AdtyModelRead, authz.AdtyModelDelete, authz.ChannelWrite)
	assert.NotEqual(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodDelete, "/api/models/1?remove_from_channels=true", "", f.token).Code)
}

func TestAdminCannotEditPermissions(t *testing.T) {
	f := newAdtyGuardFixture(t)
	f.grant(t, authz.AdtyUserRead, authz.AdtyUserWrite)
	target := model.User{Username: "guard-peer", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, f.db.Create(&target).Error)
	body := fmt.Sprintf(`{"id":%d,"username":"guard-peer","group":"default","admin_permissions":{"adty_user":{"read":true}}}`, target.Id)
	resp := adtyJSON(t, f.engine, http.MethodPut, "/api/user/", body, f.token)
	assert.Contains(t, resp.Body.String(), `"success":false`)
	assert.False(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyUserRead))
}

func TestEveryUpstreamAdminScopeClassified(t *testing.T) {
	wasEnabled := service.AdtyAdminPermissionsEnabled()
	service.SetAdtyAdminPermissionsEnabled(false)
	catalog := service.AccessTokenCatalog(1, common.RoleRootUser)
	service.SetAdtyAdminPermissionsEnabled(wasEnabled)
	restore := middleware.EnableAdtyAdminPermissions()
	t.Cleanup(restore)
	casbin := map[string]bool{}
	for _, resource := range authz.Catalog() {
		casbin[resource.Resource] = true
	}
	var checkedScopes []string
	for _, group := range catalog {
		if group.Group != service.AccessTokenGroupAdmin {
			continue
		}
		for _, resource := range group.Resources {
			if casbin[resource.Resource] {
				continue
			}
			for _, action := range resource.Actions {
				scope := resource.Resource + ":" + action.Action
				checkedScopes = append(checkedScopes, scope)
				classified := middleware.AdtyAdminScopeClassified(scope)
				rootOnly := slices.Contains(middleware.AdtyRootOnlyAdminScopes, scope)
				assert.True(t, classified || rootOnly, "classify %s in middleware/adty_admin_guard.go", scope)
			}
		}
	}
	assert.Subset(t, checkedScopes, []string{
		"user:read", "user:write", "billing:read", "billing:write",
		"model:read", "model:write", "log:read", "group:read", "group:write",
	}, "the retained upstream admin scopes must not disappear when the ADTY grant catalog hides legacy scopes")
}

func TestAdminRouteInventory(t *testing.T) {
	wasEnabled := service.AdtyAdminPermissionsEnabled()
	service.SetAdtyAdminPermissionsEnabled(false)
	t.Cleanup(func() { service.SetAdtyAdminPermissionsEnabled(wasEnabled) })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	var routes []string
	for _, key := range middleware.AccessTokenRouteRuleKeys() {
		rule, _ := middleware.AccessTokenRouteRule(key)
		category, _, _ := strings.Cut(rule.Scope(), ":")
		switch category {
		case "user", "billing", "log", "model", "group":
			routes = append(routes, key+" "+rule.Scope())
		}
	}
	raw, err := os.ReadFile("../middleware/testdata/adty_admin_routes.txt")
	require.NoError(t, err)
	var listed []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			listed = append(listed, line)
		}
	}
	assert.Equal(t, listed, routes, "upstream added or removed admin routes; classify them in middleware/adty_admin_guard.go and update middleware/testdata/adty_admin_routes.txt")

	restore := middleware.EnableAdtyAdminPermissions()
	t.Cleanup(restore)
	currentScopes := map[string]bool{}
	for _, resource := range authz.Catalog() {
		for _, action := range resource.Actions {
			currentScopes[resource.Resource+":"+action.Action] = true
		}
	}
	for _, route := range listed {
		fields := strings.Fields(route)
		require.Len(t, fields, 3)
		key := fields[0] + " " + fields[1]
		rule, exists := middleware.AccessTokenRouteRule(key)
		require.True(t, exists, key)
		if key == "POST /api/user/topup/complete" {
			closed := httptest.NewRecorder()
			engine.ServeHTTP(closed, httptest.NewRequest(http.MethodPost, "/api/user/topup/complete", nil))
			assert.Equal(t, http.StatusForbidden, closed.Code, "the retired upstream completion route must remain closed")
			continue
		}
		if key == "GET /api/group/" {
			assert.Equal(t, "scope", rule.Kind(), key)
			assert.Equal(t, "group:read", rule.Scope(), "the group entry delegates to the same live/PAT permission intersection")
			continue
		}
		if key == "POST /api/models/sync_upstream" || key == "POST /api/vendors/operations" {
			assert.Equal(t, "any", rule.Kind(), key+" must defer compound actions to the live guard and handler")
			continue
		}
		assert.Equal(t, "scope", rule.Kind(), key)
		assert.True(t, currentScopes[rule.Scope()] || slices.Contains(middleware.AdtyRootOnlyAdminScopes, rule.Scope()), key+" must use a current action or an explicit Root-only scope")
	}
}

func TestMainEnablesAdtyAdminPermissions(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../main.go", nil, 0)
	require.NoError(t, err)
	order := []string{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok {
				name := pkg.Name + "." + sel.Sel.Name
				if name == "middleware.EnableAdtyAdminPermissions" || name == "authz.Init" {
					order = append(order, name)
				}
			}
		}
		return true
	})
	require.NotEmpty(t, order)
	assert.Equal(t, "middleware.EnableAdtyAdminPermissions", order[0], "main.go must enable admin permissions before authz.Init")
	assert.Contains(t, order, "authz.Init")
}
