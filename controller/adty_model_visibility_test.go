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

// 下架（status=0）与删除是同一个业务动作：目录里看不到、也下不了单。
// 只有「模型·编辑」的账号原来能靠 `PUT /api/models/?status_only=true` 把模型撤下来，
// 这就是「没有下架权限的账号做出了下架效果」（ISS-R4-0117）。
func TestAdtyModelDelistNeedsDeletePermissionDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("set " + dialect.env)
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			setupMetadataActionPermissions(t, db)
			update := func(t *testing.T, row *model.Model, query string) *httptest.ResponseRecorder {
				t.Helper()
				engine := gin.New()
				engine.Handle(http.MethodPut, "/api/models/", func(c *gin.Context) {
					c.Set("id", 905)
					c.Set("role", common.RoleAdminUser)
					UpdateModelMeta(c)
				})
				body := fmt.Sprintf(`{"id":%d,"model_name":%q,"status":%d}`, row.Id, row.ModelName, row.Status)
				request := httptest.NewRequest(http.MethodPut, "/api/models/"+query, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				return response
			}
			storedStatus := func(t *testing.T, id int) int {
				t.Helper()
				var stored model.Model
				require.NoError(t, db.First(&stored, id).Error)
				return stored.Status
			}

			visible := model.Model{ModelName: "delist-visible", Status: 1}
			require.NoError(t, visible.Insert())
			hidden := model.Model{ModelName: "delist-hidden", Status: 0}
			require.NoError(t, hidden.Insert())

			require.NoError(t, authz.SetUserPermissions(905, authz.PermissionsMap{"adty_model": {"read": true, "write": true}}))

			// 编辑元数据（不动 status）只要「模型·编辑」。
			editable := visible
			editable.Description = "edited without delisting"
			assert.Equal(t, http.StatusOK, update(t, &editable, "").Code)
			assert.EqualValues(t, 1, storedStatus(t, visible.Id), "editing metadata must not delist")

			// 下架要「模型·删除」，status_only 的快捷路径也一样。
			delist := visible
			delist.Status = 0
			denied := update(t, &delist, "?status_only=true")
			assert.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
			assert.Contains(t, denied.Body.String(), AdtyPermissionDeniedCode)
			assert.EqualValues(t, 1, storedStatus(t, visible.Id), "a denied delist must not hide the model")

			// 整包更新漏掉 status 字段（反序列化成 0）同样是下架，不能靠换个接口绕过去。
			engine := gin.New()
			engine.Handle(http.MethodPut, "/api/models/", func(c *gin.Context) {
				c.Set("id", 905)
				c.Set("role", common.RoleAdminUser)
				UpdateModelMeta(c)
			})
			request := httptest.NewRequest(http.MethodPut, "/api/models/", strings.NewReader(fmt.Sprintf(`{"id":%d,"model_name":%q}`, visible.Id, visible.ModelName)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			assert.EqualValues(t, 1, storedStatus(t, visible.Id))

			// 已经是隐藏状态的模型再保存一次不是新的下架动作。
			assert.Equal(t, http.StatusOK, update(t, &hidden, "?status_only=true").Code)

			// 拿到「模型·删除」之后两条路都通。
			require.NoError(t, authz.SetUserPermissions(905, authz.PermissionsMap{"adty_model": {"read": true, "write": true, "delete": true}}))
			assert.Equal(t, http.StatusOK, update(t, &delist, "?status_only=true").Code)
			assert.EqualValues(t, 0, storedStatus(t, visible.Id), "the delisting itself is unchanged")

			// 新建只要求「模型·创建」：没有创建权限的编辑账号也不能建。
			require.NoError(t, authz.SetUserPermissions(905, authz.PermissionsMap{"adty_model": {"read": true, "write": true}}))
			create := gin.New()
			create.Handle(http.MethodPost, "/api/models/", func(c *gin.Context) {
				c.Set("id", 905)
				c.Set("role", common.RoleAdminUser)
				CreateModelMeta(c)
			})
			createRequest := httptest.NewRequest(http.MethodPost, "/api/models/", strings.NewReader(`{"model_name":"created-without-create"}`))
			createRequest.Header.Set("Content-Type", "application/json")
			createResponse := httptest.NewRecorder()
			create.ServeHTTP(createResponse, createRequest)
			assert.Equal(t, http.StatusForbidden, createResponse.Code, createResponse.Body.String())
			var count int64
			require.NoError(t, db.Model(&model.Model{}).Where("model_name = ?", "created-without-create").Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}
