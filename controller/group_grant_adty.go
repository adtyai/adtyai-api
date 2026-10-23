package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type groupGrantRequest struct {
	UserId int `json:"user_id"`
}

func AdminListGroupGrants(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	if err := model.ValidateGroupMetaCode(code); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_code", "invalid group code", nil)
		return
	}
	rows, err := model.ListGroupGrantViews(code)
	if err != nil {
		adtyInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": len(rows)})
}

func AdminGrantGroup(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	code := strings.TrimSpace(c.Param("code"))
	var req groupGrantRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.UserId <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "user_id is required", nil)
		return
	}
	if err := model.GrantHiddenGroup(req.UserId, code, c.GetInt("id")); err != nil {
		writeGroupGrantError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func AdminRevokeGroupGrant(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	code := strings.TrimSpace(c.Param("code"))
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid user id", nil)
		return
	}
	if err := model.RevokeHiddenGroup(userID, code); err != nil {
		writeGroupGrantError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func writeGroupGrantError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrGroupMetaInvalidCode):
		adtyFail(c, http.StatusBadRequest, "invalid_code", "invalid group code", nil)
	case errors.Is(err, model.ErrGroupGrantNotHidden):
		adtyFail(c, http.StatusBadRequest, "not_hidden", "only a hidden group can be granted to one user", nil)
	case errors.Is(err, model.ErrGroupGrantNoRatio):
		adtyFail(c, http.StatusBadRequest, "no_ratio", "group ratio is missing", nil)
	case errors.Is(err, model.ErrGroupGrantUserMissing):
		adtyFail(c, http.StatusNotFound, "user_missing", "user not found", nil)
	case errors.Is(err, model.ErrGroupGrantExists):
		adtyFail(c, http.StatusConflict, "exists", "this user already has this group", nil)
	case errors.Is(err, model.ErrGroupGrantNotFound):
		adtyFail(c, http.StatusNotFound, "not_found", "grant not found", nil)
	default:
		adtyInternalError(c, err)
	}
}
