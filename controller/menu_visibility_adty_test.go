package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func menuVisCtx(t *testing.T, method, body string, role int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "/api/admin/menu-visibility", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, "/api/admin/menu-visibility", nil)
	}
	c.Request = r
	c.Set("id", 1)
	c.Set("role", role)
	c.Set("username", "test")
	return c, w
}

func TestAdminPutMenuVisibility_RootReturns410(t *testing.T) {
	c, w := menuVisCtx(t, http.MethodPut, `{"user":{"wallet":false}}`, common.RoleRootUser)
	AdminPutMenuVisibility(c)
	assert.Equal(t, http.StatusGone, w.Code)
	var resp struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "MENU_VISIBILITY_REMOVED", resp.Code)
	assert.Contains(t, resp.Message, "SidebarModulesAdmin")
}

func TestAdminPutMenuVisibility_NonRootReturns403(t *testing.T) {
	for _, role := range []int{common.RoleAdminUser, common.RoleCommonUser} {
		t.Run(roleName(role), func(t *testing.T) {
			c, w := menuVisCtx(t, http.MethodPut, `{"user":{"wallet":false}}`, role)
			AdminPutMenuVisibility(c)
			assert.Equal(t, http.StatusForbidden, w.Code)
			var resp struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "root_required", resp.Code)
		})
	}
}

func roleName(role int) string {
	switch role {
	case common.RoleRootUser:
		return "root"
	case common.RoleAdminUser:
		return "admin"
	default:
		return "user"
	}
}
