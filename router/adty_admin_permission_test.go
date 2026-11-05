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
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}))
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
		&model.AuditLog{}, &model.Log{}, &model.Option{}, &model.TopUp{}, &model.UserAccessToken{}))
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
	assert.ElementsMatch(t, []string{"adty_user", "adty_log", "adty_model", "adty_finance", "adty_referral", "adty_payment", "adty_marketing", "adty_site"}, adty)
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
	for _, path := range []string{"/api/channel/", "/api/user/", "/api/log/", "/api/deployments/settings"} {
		resp := adtyJSON(t, f.engine, http.MethodGet, path, "", f.root)
		assert.NotEqual(t, http.StatusForbidden, resp.Code, path)
	}
}

func TestUnclassifiedAdminScopeIsRootOnly(t *testing.T) {
	f := newAdtyGuardFixture(t)
	all := make([]authz.Permission, 0)
	all = append(all, authz.AllPermissions()...)
	f.grant(t, all...)
	resp := adtyJSON(t, f.engine, http.MethodGet, "/api/deployments/settings", "", f.token)
	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.Equal(t, http.StatusOK, adtyJSON(t, f.engine, http.MethodGet, "/api/user/", "", f.token).Code)
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
	f.grant(t, authz.AdtyModelRead, authz.AdtyModelWrite)
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

	f.grant(t, authz.AdtyModelRead, authz.AdtyModelWrite, authz.ChannelWrite)
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
	restore := middleware.EnableAdtyAdminPermissions()
	t.Cleanup(restore)
	casbin := map[string]bool{}
	for _, resource := range authz.Catalog() {
		casbin[resource.Resource] = true
	}
	for _, group := range service.AccessTokenCatalog(1, common.RoleRootUser) {
		if group.Group != service.AccessTokenGroupAdmin {
			continue
		}
		for _, resource := range group.Resources {
			if casbin[resource.Resource] {
				continue
			}
			for _, action := range resource.Actions {
				scope := resource.Resource + ":" + action.Action
				classified := middleware.AdtyAdminScopeClassified(scope)
				rootOnly := slices.Contains(middleware.AdtyRootOnlyAdminScopes, scope)
				assert.True(t, classified || rootOnly, "classify %s in middleware/adty_admin_guard.go", scope)
			}
		}
	}
}

func TestAdminRouteInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetApiRouter(gin.New())
	var routes []string
	for _, key := range middleware.AccessTokenRouteRuleKeys() {
		rule, _ := middleware.AccessTokenRouteRule(key)
		category, _, _ := strings.Cut(rule.Scope(), ":")
		switch category {
		case "user", "billing", "log", "model", "group", "deployment":
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
