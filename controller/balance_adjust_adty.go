package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type createBalanceAdjustReq struct {
	UserId                 int    `json:"user_id"`
	Account                string `json:"account"`
	Amount                 int64  `json:"amount"`
	Reason                 string `json:"reason"`
	ChatwootConversationId string `json:"chatwoot_conversation_id"`
}

type approveBalanceAdjustReq struct {
	Confirm           bool   `json:"confirm"`
	SelfApproveReason string `json:"self_approve_reason"`
	Reason            string `json:"reason"` // 兼容字段：自批原因
}

type rejectBalanceAdjustReq struct {
	Confirm      bool   `json:"confirm"`
	Reason       string `json:"reason"`
	RejectReason string `json:"reject_reason"`
}

// AdminCreateBalanceAdjustment POST /api/admin/balance-adjustments
func AdminCreateBalanceAdjustment(c *gin.Context) {
	if adminFeatureFrozen {
		respondAdminFeatureFrozen(c)
		return
	}
	var req createBalanceAdjustReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	applicantId := c.GetInt("id")
	row, err := model.CreateBalanceAdjustRequest(req.UserId, req.Account, req.Amount, req.Reason, req.ChatwootConversationId, applicantId)
	if err != nil {
		status, code := mapBalanceAdjustErr(err)
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminListBalanceAdjustments GET /api/admin/balance-adjustments
func AdminListBalanceAdjustments(c *gin.Context) {
	if adminFeatureFrozen {
		respondAdminFeatureFrozen(c)
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	userId, _ := strconv.Atoi(c.Query("user_id"))
	applicantId, _ := strconv.Atoi(c.Query("applicant_id"))
	offset, _ := strconv.Atoi(c.DefaultQuery("p", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, total, err := model.ListBalanceAdjustRequests(status, userId, applicantId, offset, limit)
	if err != nil {
		adtyFail(c, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// AdminApproveBalanceAdjustment POST /api/admin/balance-adjustments/:id/approve
func AdminApproveBalanceAdjustment(c *gin.Context) {
	if adminFeatureFrozen {
		respondAdminFeatureFrozen(c)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req approveBalanceAdjustReq
	_ = c.ShouldBindJSON(&req)
	selfReason := strings.TrimSpace(req.SelfApproveReason)
	if selfReason == "" {
		selfReason = strings.TrimSpace(req.Reason)
	}
	approverId := c.GetInt("id")

	result, err := model.ApproveBalanceAdjust(id, approverId, req.Confirm, selfReason)
	if err != nil {
		status, code := mapBalanceAdjustErr(err)
		adtyFail(c, status, code, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// AdminRejectBalanceAdjustment POST /api/admin/balance-adjustments/:id/reject
func AdminRejectBalanceAdjustment(c *gin.Context) {
	if adminFeatureFrozen {
		respondAdminFeatureFrozen(c)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req rejectBalanceAdjustReq
	_ = c.ShouldBindJSON(&req)
	reason := strings.TrimSpace(req.RejectReason)
	if reason == "" {
		reason = strings.TrimSpace(req.Reason)
	}
	approverId := c.GetInt("id")
	row, err := model.RejectBalanceAdjust(id, approverId, req.Confirm, reason)
	if err != nil {
		status, code := mapBalanceAdjustErr(err)
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

func mapBalanceAdjustErr(err error) (int, string) {
	switch {
	case errors.Is(err, model.ErrBalanceAdjustSelfForbidden):
		return http.StatusForbidden, "self_approve_forbidden"
	case errors.Is(err, model.ErrBalanceAdjustConflict):
		return http.StatusConflict, "already_decided"
	case errors.Is(err, model.ErrBalanceAdjustNotFound),
		errors.Is(err, model.ErrBalanceAdjustUserMissing),
		errors.Is(err, model.ErrAdjustQuotaUserNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, model.ErrBalanceAdjustBadAmount),
		errors.Is(err, model.ErrBalanceAdjustAmountTooLarge),
		errors.Is(err, model.ErrAdjustQuotaZeroDelta):
		return http.StatusBadRequest, "amount_invalid"
	case errors.Is(err, model.ErrBalanceAdjustReasonRequired),
		errors.Is(err, model.ErrBalanceAdjustRejectRequired),
		errors.Is(err, model.ErrBalanceAdjustSelfReasonReq):
		return http.StatusBadRequest, "reason_required"
	case errors.Is(err, model.ErrBalanceAdjustConfirmRequired):
		return http.StatusBadRequest, "confirm_required"
	case errors.Is(err, model.ErrBalanceAdjustBadAccount),
		errors.Is(err, model.ErrAdjustQuotaBadAccount):
		return http.StatusBadRequest, "invalid_account"
	case errors.Is(err, model.ErrBalanceAdjustUserRequired):
		return http.StatusBadRequest, "bad_request"
	case errors.Is(err, model.ErrAdjustQuotaInsufficient):
		return http.StatusBadRequest, "insufficient_balance"
	case errors.Is(err, model.ErrAdjustQuotaInjectedFail):
		return http.StatusInternalServerError, "adjust_failed"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
