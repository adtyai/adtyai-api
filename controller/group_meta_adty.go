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
		adtyInternalError(c, err)
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
		writeGroupMetaWriteError(c, err)
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
	view, err := model.CreateGroupWithRatio(model.DB, req)
	if err != nil {
		writeGroupMetaWriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// AdminSaveGroupBilling PUT /api/admin/groups/billing (Root)
// Writes ratio, visibility, and display names together. Does not add billing codes.
func AdminSaveGroupBilling(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req struct {
		GroupRatio       map[string]float64 `json:"group_ratio"`
		UserUsableGroups map[string]string  `json:"user_usable_groups"`
		DisplayNames     map[string]string  `json:"display_names"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "Invalid parameters", nil)
		return
	}
	err := model.SaveGroupBillingTable(req.GroupRatio, req.UserUsableGroups, req.DisplayNames)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}
	switch {
	case errors.Is(err, model.ErrGroupBillingExtraCode), errors.Is(err, model.ErrGroupBillingKeySet), errors.Is(err, model.ErrGroupBillingUsable):
		adtyFail(c, http.StatusBadRequest, "invalid_code", "Invalid parameters", nil)
	case errors.Is(err, model.ErrGroupBillingRatio):
		adtyFail(c, http.StatusBadRequest, "invalid_ratio", "Ratio must be a number", nil)
	case errors.Is(err, model.ErrGroupMetaDisplayName), errors.Is(err, model.ErrGroupMetaInvalidCode):
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "Invalid parameters", nil)
	default:
		adtyInternalError(c, err)
	}
}

func writeGroupMetaWriteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrGroupMetaCodeImmutable):
		rootReject(c, http.StatusBadRequest, "code_immutable", "Group code cannot be changed")
	case errors.Is(err, model.ErrGroupMetaInvalidCode):
		rootReject(c, http.StatusBadRequest, "invalid_code", "Invalid parameters")
	case errors.Is(err, model.ErrGroupMetaDisplayName):
		rootReject(c, http.StatusBadRequest, "display_name_required", "Display name is required")
	case errors.Is(err, model.ErrGroupMetaExists):
		rootReject(c, http.StatusBadRequest, "exists", "Group already exists")
	default:
		adtyInternalError(c, err)
	}
}
