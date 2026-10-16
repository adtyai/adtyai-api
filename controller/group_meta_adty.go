package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetGroups GET /api/admin/groups (AdminAuth) — A08 group_meta list.
func AdminGetGroups(c *gin.Context) {
	views, err := model.ListGroupMetaViews(model.DB)
	if err != nil {
		adtyFail(c, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": views})
}

// AdminPutGroup PUT /api/admin/groups/:code (AdminAuth)
// Only display_name / sort; body code change → 400. Root only (A08).
func AdminPutGroup(c *gin.Context) {
	pathCode := strings.TrimSpace(c.Param("code"))
	if !requireRoot(c) {
		return
	}
	var req model.GroupMetaPutInput
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		rootReject(c, http.StatusBadRequest, "bad_request", "invalid json")
		return
	}
	_, newSnap, err := model.UpdateGroupMetaDisplay(model.DB, pathCode, req)
	if err != nil {
		status := http.StatusBadRequest
		code := "bad_request"
		switch {
		case errors.Is(err, model.ErrGroupMetaCodeImmutable):
			code = "code_immutable"
		case errors.Is(err, model.ErrGroupMetaInvalidCode):
			code = "invalid_code"
		case errors.Is(err, model.ErrGroupMetaDisplayName):
			code = "display_name_required"
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error(), "code": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": newSnap})
}

// AdminPostGroup POST /api/admin/groups — create meta row. Root only (A08).
func AdminPostGroup(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req model.GroupMetaCreateInput
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		rootReject(c, http.StatusBadRequest, "bad_request", "invalid json")
		return
	}
	view, err := model.CreateGroupMeta(model.DB, req)
	if err != nil {
		status := http.StatusBadRequest
		code := "bad_request"
		switch {
		case errors.Is(err, model.ErrGroupMetaInvalidCode):
			code = "invalid_code"
		case errors.Is(err, model.ErrGroupMetaExists):
			code = "exists"
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error(), "code": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}
