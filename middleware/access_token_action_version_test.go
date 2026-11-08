package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdtyActionPATCompatibilityRequiresPersistedOldVersion(t *testing.T) {
	for _, route := range []struct{ method, path, oldScope, newScope string }{
		{http.MethodPost, "/api/user/", "user:write", "adty_user:create"},
		{http.MethodDelete, "/api/user/:id", "user:write", "adty_user:delete"},
		{http.MethodGet, "/api/log/export", "log:read", "adty_log:export"},
		{http.MethodPost, "/api/models/", "model:write", "adty_model:create"},
		{http.MethodDelete, "/api/models/:id", "model:write", "adty_model:delete"},
		{http.MethodPost, "/api/prefill_group/", "group:write", "adty_model:create"},
		{http.MethodDelete, "/api/prefill_group/:id", "group:write", "adty_model:delete"},
		{http.MethodGet, "/api/user/", "user:read", "adty_user:read"},
		{http.MethodPut, "/api/user/", "user:write", "adty_user:write"},
		{http.MethodGet, "/api/models/", "model:read", "adty_model:read"},
		{http.MethodPut, "/api/models/", "model:write", "adty_model:write"},
		{http.MethodGet, "/api/prefill_group/", "group:read", "adty_model:read"},
		{http.MethodPut, "/api/prefill_group/", "group:write", "adty_model:write"},
		{http.MethodGet, "/api/log/", "log:read", "adty_log:read"},
		{http.MethodGet, "/api/user/topup", "billing:read", "adty_finance:read"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			setupDashboardAuthMiddlewareTest(t)
			restore := EnableAdtyAdminPermissions()
			t.Cleanup(restore)
			root := createMiddlewarePATUser(t, "action-version-root", "unused")
			require.NoError(t, model.DB.Model(root).Update("role", common.RoleRootUser).Error)
			engine := gin.New()
			engine.Handle(route.method, route.path, AdminAuth(), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"scope_version": c.GetInt("access_token_scope_version")})
			})
			for _, grant := range []struct {
				name, scope   string
				version, want int
			}{
				{"persisted legacy grant", route.oldScope, 0, http.StatusOK},
				{"new grant cannot inherit legacy alias", route.oldScope, 2, http.StatusForbidden},
				{"new independent grant", route.newScope, 2, http.StatusOK},
				{"future version fails closed", route.newScope, 99, http.StatusForbidden},
			} {
				t.Run(grant.name, func(t *testing.T) {
					raw, token := createMiddlewareScopedToken(t, root.Id, 0, grant.scope)
					require.NoError(t, model.DB.Model(token).Update("scope_version", grant.version).Error)
					request := httptest.NewRequest(route.method, strings.ReplaceAll(route.path, ":id", "17"), nil)
					request.Header.Set("Authorization", "Bearer "+raw)
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, request)
					assert.Equal(t, grant.want, response.Code, response.Body.String())
					if grant.want == http.StatusOK {
						var body struct {
							ScopeVersion int `json:"scope_version"`
						}
						require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
						assert.Equal(t, grant.version, body.ScopeVersion)
					}
				})
			}
		})
	}
}

func TestAdtyGroupListIntersectsTheSameLiveAndTokenPermission(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}, &model.AuthzActionCatalogState{}))
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	restore := EnableAdtyAdminPermissions()
	require.NoError(t, authz.Init(model.DB))
	t.Cleanup(func() {
		restore()
		require.NoError(t, authz.Init(model.DB))
		common.IsMasterNode = wasMaster
	})
	admin := createMiddlewarePATUser(t, "group-intersection-admin", "unused")
	require.NoError(t, model.DB.Model(admin).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"adty_user": {"read": true}, "channel": {"read": false}}))
	engine := gin.New()
	engine.GET("/api/group/", AdminAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, grant := range []struct {
		name, scope   string
		version, want int
	}{
		{"aligned current capability", "adty_user:read", 2, http.StatusNoContent},
		{"mismatched current capability", "channel:read", 2, http.StatusForbidden},
		{"old group dropdown capability", "group:read", 0, http.StatusNoContent},
		{"new token cannot request old group capability", "group:read", 2, http.StatusForbidden},
	} {
		t.Run(grant.name, func(t *testing.T) {
			raw, token := createMiddlewareScopedToken(t, admin.Id, 0, grant.scope)
			require.NoError(t, model.DB.Model(token).Update("scope_version", grant.version).Error)
			response := middlewareBearerRequest(engine, "/api/group/", raw)
			assert.Equal(t, grant.want, response.Code, response.Body.String())
		})
	}
}

func TestAdtyStaticActionScopeStillRequiresLivePermission(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}, &model.AuthzActionCatalogState{}))
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	restore := EnableAdtyAdminPermissions()
	require.NoError(t, authz.Init(model.DB))
	t.Cleanup(func() {
		restore()
		require.NoError(t, authz.Init(model.DB))
		common.IsMasterNode = wasMaster
	})
	admin := createMiddlewarePATUser(t, "live-action-admin", "unused")
	require.NoError(t, model.DB.Model(admin).Update("role", common.RoleAdminUser).Error)
	raw, _ := createMiddlewareScopedToken(t, admin.Id, 0, "adty_user:create")
	engine := gin.New()
	// This static route deliberately has no later RequirePermission middleware.
	engine.POST("/api/user/", AdminAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/user/", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response.Code
	}
	assert.Equal(t, http.StatusForbidden, request(), "a matching PAT cannot supply the user's missing action")
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"adty_user": {"create": true}}))
	assert.Equal(t, http.StatusNoContent, request(), "create does not require edit")
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"adty_user": {"create": false}}))
	assert.Equal(t, http.StatusForbidden, request(), "revocation applies to an already issued PAT")
}
