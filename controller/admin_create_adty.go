package controller

import (
	"gorm.io/gorm"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminCreateAdminInput is the body of POST /api/admin/admins (A36).
type AdminCreateAdminInput struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        *int   `json:"role,omitempty"`
	Confirm     bool   `json:"confirm"`
}

const auditActionAdminCreate = "admin.create"

// Fixed client messages; raw driver/validator errors go to SysLog only (A36 review note).
const (
	AdminCreateMsgInvalidInput = "invalid input"
	AdminCreateMsgDBError      = "database error"
	AdminCreateMsgCreateFailed = "create failed"
)

func adminCreateReject(c *gin.Context, status int, code, msg string, extra map[string]any) {
	params := map[string]any{"result": status, "reason": code}
	for k, v := range extra {
		params[k] = v
	}
	c.JSON(status, gin.H{"success": false, "message": msg, "code": code})
}

// AdminCreateAdmin POST /api/admin/admins — A36 sensitive entry.
// Root only + confirm=true; always creates role=Admin (never Root). Password is never audited.
func AdminCreateAdmin(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		adminCreateReject(c, http.StatusForbidden, "root_required", "only root can create administrators", nil)
		return
	}
	var req AdminCreateAdminInput
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adminCreateReject(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	who := map[string]any{"username": req.Username}
	if !req.Confirm {
		adminCreateReject(c, http.StatusBadRequest, "confirm_required", "confirm=true required to create an administrator", who)
		return
	}
	if req.Role != nil && *req.Role != common.RoleAdminUser {
		who["requested_role"] = *req.Role
		adminCreateReject(c, http.StatusBadRequest, "role_not_allowed", "this endpoint only creates role=Admin", who)
		return
	}
	if req.Username == "" || req.Password == "" {
		adminCreateReject(c, http.StatusBadRequest, "invalid_params", "username and password required", who)
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}
	u := model.User{
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Email:       req.Email,
		Role:        common.RoleAdminUser,
	}
	if err := common.Validate.Struct(&u); err != nil {
		common.SysLog("admin.create validate: " + err.Error())
		adminCreateReject(c, http.StatusBadRequest, "invalid_params", AdminCreateMsgInvalidInput, who)
		return
	}
	var cnt int64
	if err := model.DB.Unscoped().Model(&model.User{}).Where("username = ?", req.Username).Count(&cnt).Error; err != nil {
		common.SysLog("admin.create db_error: " + err.Error())
		adminCreateReject(c, http.StatusInternalServerError, "db_error", AdminCreateMsgDBError, who)
		return
	}
	if cnt > 0 {
		adminCreateReject(c, http.StatusBadRequest, "username_taken", "username already exists", who)
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error { return u.InsertWithTx(tx, 0) }); err != nil {
		common.SysLog("admin.create create_failed: " + err.Error())
		adminCreateReject(c, http.StatusBadRequest, "create_failed", AdminCreateMsgCreateFailed, who)
		return
	}
	u.FinishInsert(0)
	after := map[string]any{"id": u.Id, "username": u.Username, "display_name": u.DisplayName, "email": u.Email, "role": u.Role}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": after})
}
