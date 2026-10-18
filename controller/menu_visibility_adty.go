package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetMenuVisibility GET /api/admin/menu-visibility (AdminAuth)
// Read-only compatibility after merge: returns empty user/admin maps once
// Visible options are cleared to "{}". Prefer SidebarModulesAdmin / 系统设置·侧边栏模块.
func AdminGetMenuVisibility(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    model.BuildMenuVisibilityView(),
	})
}

// rootReject writes a reject response only (A39/A08 Root-only writes; no audit).
func rootReject(c *gin.Context, status int, code, msg string) {
	c.JSON(status, gin.H{"success": false, "message": msg, "code": code})
}

// requireRoot rejects non-Root callers with 403 (no audit). Returns false if rejected.
func requireRoot(c *gin.Context) bool {
	if c.GetInt("role") < common.RoleRootUser {
		rootReject(c, http.StatusForbidden, "root_required", "only root can perform this change")
		return false
	}
	return true
}

// AdminPutMenuVisibility PUT /api/admin/menu-visibility
// Removed: independent Visible write path is retired. Root still gated first (403),
// then 410 MENU_VISIBILITY_REMOVED. Write via 系统设置·侧边栏模块 / SidebarModulesAdmin.
func AdminPutMenuVisibility(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	rootReject(c, http.StatusGone, "MENU_VISIBILITY_REMOVED",
		"menu-visibility API removed; use 系统设置·侧边栏模块 (SidebarModulesAdmin)")
}
