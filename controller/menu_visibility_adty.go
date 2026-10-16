package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetMenuVisibility GET /api/admin/menu-visibility (AdminAuth)
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

// AdminPutMenuVisibility PUT /api/admin/menu-visibility (Root only; A39)
// Body: {"user":{...},"admin":{...}} — either or both sides; wholesale replace per side.
// Hiding a menu only hides the entry; it does not change route auth.
func AdminPutMenuVisibility(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req model.MenuVisibilityPutInput
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		rootReject(c, http.StatusBadRequest, "bad_request", "invalid json")
		return
	}
	_, _, err := model.ApplyMenuVisibilityPut(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error(), "code": "bad_request"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.BuildMenuVisibilityView()})
}
