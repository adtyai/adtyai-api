package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func manageUserQuota(c *gin.Context, req ManageRequest) {
	action := "generic"
	params := model.AuditFields{
		"target_user_id":  req.Id,
		"mode":            req.Mode,
		"requested_quota": req.Value,
	}
	switch req.Mode {
	case "add":
		action = "user.quota_add"
	case "subtract":
		action = "user.quota_subtract"
	case "override":
		action = "user.quota_override"
	default:
		params["action"] = "add_quota"
		params["method"] = c.Request.Method
		params["route"] = c.FullPath()
	}
	success := false
	skipAdtyQuotaAudit := false
	defer func() {
		if skipAdtyQuotaAudit {
			markAuditLogged(c)
			return
		}
		content := auditContentEN(action, params)
		if !success {
			// Failed requests have no committed balance changes to render.
			content = "Failed user quota adjustment"
		}
		model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), content, c.ClientIP(), action, params,
			auditOperatorInfo(c), &model.AuditRequestInfo{
				Method: c.Request.Method, Route: c.FullPath(), Status: c.Writer.Status(), Success: success,
			}, c)
		markAuditLogged(c)
	}()

	// 二开：原因和幂等键，同一事务写余额流水（任务 7.2）；原因只进审计，不进用户日志。
	params["reason"] = strings.TrimSpace(req.Reason)
	adjustment, err := model.AdjustUserQuotaWithLedger(req.Id, c.GetInt("id"), c.GetInt("role"), req.Mode, req.Value, req.Reason, req.IdempotencyKey)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrAdtyAdjustReasonRequired):
			params["failure_reason"] = "reason_required"
			common.ApiErrorT(c, "Enter a reason for the adjustment")
		case errors.Is(err, model.ErrAdtyAdjustReasonTooLong):
			params["failure_reason"] = "reason_too_long"
			common.ApiErrorT(c, "The reason must be at most 200 characters")
		case errors.Is(err, model.ErrAdtyAdjustKeyTooLong):
			params["failure_reason"] = "invalid_idempotency_key"
			common.ApiErrorT(c, "Invalid idempotency key")
		case errors.Is(err, model.ErrAdtyIdempotencyConflict):
			params["failure_reason"] = "idempotency_conflict"
			common.ApiErrorT(c, "This request was already used for another operation")
		case errors.Is(err, model.ErrInvalidUserQuotaAdjustment):
			params["failure_reason"] = "invalid_parameters"
			if (req.Mode == "add" || req.Mode == "subtract") && req.Value <= 0 {
				common.ApiErrorT(c, "Quota change amount cannot be zero")
			} else {
				common.ApiErrorT(c, "Invalid parameters")
			}
		case errors.Is(err, model.ErrUserQuotaPermission):
			params["failure_reason"] = "permission_denied"
			common.ApiErrorT(c, "No permission to update users of same or higher permission level")
		case errors.Is(err, gorm.ErrRecordNotFound):
			params["failure_reason"] = "target_not_found"
			common.ApiErrorT(c, "User does not exist")
		case errors.Is(err, model.ErrWalletQuotaLimitExceeded):
			params["failure_reason"] = "quota_limit_exceeded"
			common.ApiError(c, err)
		default:
			params["failure_reason"] = "database_error"
			common.ApiError(c, err)
		}
		return
	}

	params["target_username"] = adjustment.Username
	params["from"] = adjustment.Before
	params["to"] = adjustment.After
	if req.Mode != "override" {
		params["quota"] = req.Value
	}
	success = true
	if adjustment.Replayed {
		// 重复提交：返回第一次的结果，不再写审计和用户日志。
		skipAdtyQuotaAudit = true
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}
	userParams := model.AuditFields{}
	for key, value := range params {
		if key != "reason" {
			userParams[key] = value
		}
	}
	operation := model.AuditOperation{Action: action, Params: userParams}
	model.RecordLogWithAdminInfo(adjustment.UserID, model.LogTypeTopup,
		auditContentEN(action, userParams), auditOperatorInfo(c), &operation, c)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
