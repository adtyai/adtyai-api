package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdtyCompoundActionPATIntersectionsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("set " + dialect.env)
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			setupMetadataActionPermissions(t, db)
			require.NoError(t, authz.SetUserPermissions(905, authz.PermissionsMap{
				"adty_model": {"create": true, "write": true, "delete": true},
				"adty_user":  {"write": true, "group": true}, "channel": {"write": true},
			}))
			for _, tc := range []struct {
				name, method, path string
				version            int
				scopes             []string
				permissions        []authz.Permission
				allow              bool
			}{
				{"legacy sync create", "POST", "/api/models/sync_upstream", 0, []string{"model:write"}, []authz.Permission{authz.AdtyModelCreate}, true},
				{"new coarse sync denied", "POST", "/api/models/sync_upstream", 2, []string{"model:write"}, []authz.Permission{authz.AdtyModelCreate}, false},
				{"create only sync", "POST", "/api/models/sync_upstream", 2, []string{"adty_model:create"}, []authz.Permission{authz.AdtyModelCreate}, true},
				{"sync update needs own scope", "POST", "/api/models/sync_upstream", 2, []string{"adty_model:create"}, []authz.Permission{authz.AdtyModelCreate, authz.AdtyModelWrite}, false},
				{"legacy merge", "POST", "/api/vendors/operations", 0, []string{"model:write"}, []authz.Permission{authz.AdtyModelWrite, authz.AdtyModelDelete}, true},
				{"merge missing delete", "POST", "/api/vendors/operations", 2, []string{"adty_model:write"}, []authz.Permission{authz.AdtyModelWrite, authz.AdtyModelDelete}, false},
				{"merge missing write", "POST", "/api/vendors/operations", 2, []string{"adty_model:delete"}, []authz.Permission{authz.AdtyModelWrite, authz.AdtyModelDelete}, false},
				{"merge both", "POST", "/api/vendors/operations", 2, []string{"adty_model:write", "adty_model:delete"}, []authz.Permission{authz.AdtyModelWrite, authz.AdtyModelDelete}, true},
				{"legacy group edit", "PUT", "/api/user/", 0, []string{"user:write"}, []authz.Permission{authz.AdtyUserWrite, authz.AdtyUserGroup}, true},
				{"group edit missing group", "PUT", "/api/user/", 2, []string{"adty_user:write"}, []authz.Permission{authz.AdtyUserWrite, authz.AdtyUserGroup}, false},
				{"group edit both", "PUT", "/api/user/", 2, []string{"adty_user:write", "adty_user:group"}, []authz.Permission{authz.AdtyUserWrite, authz.AdtyUserGroup}, true},
				{"legacy removal", "DELETE", "/api/models/:id", 0, []string{"model:write"}, []authz.Permission{authz.AdtyModelDelete, authz.ChannelWrite}, true},
				{"removal missing channel", "DELETE", "/api/models/:id", 2, []string{"adty_model:delete"}, []authz.Permission{authz.AdtyModelDelete, authz.ChannelWrite}, false},
				{"removal both", "DELETE", "/api/models/:id", 2, []string{"adty_model:delete", "channel:write"}, []authz.Permission{authz.AdtyModelDelete, authz.ChannelWrite}, true},
				{"wrong method", "PUT", "/api/vendors/operations", 0, []string{"model:write"}, []authz.Permission{authz.AdtyModelDelete}, false},
				{"adjacent new route", "PUT", "/api/adty/admin/users/:id/limits", 0, []string{"user:write"}, []authz.Permission{authz.AdtyUserWrite}, false},
				{"manage has no user write delete alias", "POST", "/api/user/manage", 0, []string{"user:write"}, []authz.Permission{authz.AdtyUserDelete}, false},
				{"unknown version", "POST", "/api/vendors/operations", 99, []string{"adty_model:write", "adty_model:delete"}, []authz.Permission{authz.AdtyModelWrite, authz.AdtyModelDelete}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					engine := gin.New()
					engine.Handle(tc.method, tc.path, func(c *gin.Context) {
						c.Set("id", 905)
						c.Set("role", common.RoleAdminUser)
						c.Set("access_token_legacy", false)
						c.Set("access_token_scope_version", tc.version)
						c.Set("access_token_scopes", tc.scopes)
						if err := adtyAuthorizeAdminActions(c, tc.permissions...); err != nil {
							c.Status(http.StatusForbidden)
							return
						}
						c.Status(http.StatusNoContent)
					})
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, httptest.NewRequest(tc.method, strings.ReplaceAll(tc.path, ":id", "17"), nil))
					want := http.StatusForbidden
					if tc.allow {
						want = http.StatusNoContent
					}
					assert.Equal(t, want, response.Code)
				})
			}
		})
	}
}

func TestAdtyModelRemovalChecksPATBeforeMutationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("set " + dialect.env)
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			setupMetadataActionPermissions(t, db)
			for _, batch := range []bool{false, true} {
				for i, grant := range []struct {
					version int
					scopes  []string
					allow   bool
				}{
					{2, []string{"adty_model:delete"}, false},
					{2, []string{"adty_model:delete", "channel:write"}, true},
					{0, []string{"model:write"}, true},
					{2, []string{"model:write"}, false},
				} {
					t.Run(fmt.Sprintf("batch=%t/grant=%d", batch, i), func(t *testing.T) {
						row := model.Model{ModelName: fmt.Sprintf("scope-removal-%t-%d", batch, i), Status: 1}
						require.NoError(t, row.Insert())
						channel := model.Channel{Name: row.ModelName, Models: row.ModelName + ",keep-model", Type: 1, Group: "default", Status: common.ChannelStatusEnabled}
						require.NoError(t, channel.Insert())
						engine := gin.New()
						method, path, url, body := http.MethodDelete, "/api/models/:id", fmt.Sprintf("/api/models/%d?remove_from_channels=true", row.Id), ""
						handler := DeleteModelMeta
						if batch {
							method, path, url, body = http.MethodPost, "/api/models/delete", "/api/models/delete", fmt.Sprintf(`{"model_ids":[%d],"remove_from_channels":true}`, row.Id)
							handler = BatchDeleteModelMeta
						}
						engine.Handle(method, path, func(c *gin.Context) {
							c.Set("id", 1)
							c.Set("role", common.RoleRootUser)
							c.Set("access_token_legacy", false)
							c.Set("access_token_scope_version", grant.version)
							c.Set("access_token_scopes", grant.scopes)
							handler(c)
						})
						request := httptest.NewRequest(method, url, strings.NewReader(body))
						request.Header.Set("Content-Type", "application/json")
						response := httptest.NewRecorder()
						engine.ServeHTTP(response, request)
						var count int64
						require.NoError(t, db.Model(&model.Model{}).Where("id = ?", row.Id).Count(&count).Error)
						var stored model.Channel
						require.NoError(t, db.First(&stored, channel.Id).Error)
						if grant.allow {
							assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
							assert.Zero(t, count)
							assert.Equal(t, "keep-model", stored.Models)
						} else {
							assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
							assert.EqualValues(t, 1, count)
							assert.Equal(t, channel.Models, stored.Models)
						}
					})
				}
			}
		})
	}
}
