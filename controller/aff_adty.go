package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type affConfirmReq struct {
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
}

type affWithdrawReq struct {
	AmountQuota int64  `json:"amount_quota"`
	IdemKey     string `json:"idem_key"`
	PayAccount  string `json:"pay_account"`
}

// GetAffSelf GET /api/aff/self
func GetAffSelf(c *gin.Context) {
	uid := c.GetInt("id")
	data, err := model.GetAffSelfSummary(uid)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// ListAffCommissionsSelf GET /api/aff/commissions/self
func ListAffCommissionsSelf(c *gin.Context) {
	uid := c.GetInt("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	_, _ = model.ReleaseDueAffCommissions(uid)
	rows, total, err := model.ListAffCommissionsByInviter(uid, limit, offset)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// CreateAffWithdraw POST /api/aff/withdraw
func CreateAffWithdraw(c *gin.Context) {
	uid := c.GetInt("id")
	var req affWithdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	row, err := model.CreateAffWithdrawal(uid, req.AmountQuota, strings.TrimSpace(req.IdemKey), req.PayAccount)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		params := map[string]any{"amount_quota": req.AmountQuota}
		switch {
		case errors.Is(err, model.ErrAffWithdrawBelowMin):
			status = http.StatusBadRequest
			code = "below_min"
			params["min"] = model.GetAffMinWithdrawAmount()
		case errors.Is(err, model.ErrAffWithdrawInsufficient):
			status = http.StatusBadRequest
			code = "insufficient_aff_quota"
			if u, uerr := model.GetUserById(uid, false); uerr == nil && u != nil {
				params["balance"] = u.AffQuota
			}
		}
		adtyFail(c, status, code, err.Error(), params)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// ListAffWithdrawalsSelf GET /api/aff/withdrawals
func ListAffWithdrawalsSelf(c *gin.Context) {
	uid := c.GetInt("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := model.ListAffWithdrawalsByUser(uid, limit, offset)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// GetAffWithdrawalSelf GET /api/aff/withdrawals/:id
func GetAffWithdrawalSelf(c *gin.Context) {
	uid := c.GetInt("id")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	row, err := model.GetAffWithdrawalById(id)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrAffWithdrawNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	if row.UserId != uid {
		adtyFail(c, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// --- admin ---

// AdminListAffCommissions GET /api/admin/aff/commissions
func AdminListAffCommissions(c *gin.Context) {
	status := c.Query("status")
	inviterId, _ := strconv.Atoi(c.Query("inviter_id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := model.ListAffCommissionsAdmin(status, inviterId, limit, offset)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// AdminVoidAffCommission POST /api/admin/aff/commissions/:id/void
func AdminVoidAffCommission(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	_ = c.ShouldBindJSON(&affConfirmReq{}) // confirm 已废弃
	row, err := model.VoidAffCommission(id, c.GetInt("id"))
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrAffCommissionNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		} else if errors.Is(err, model.ErrAffCommissionStatusInvalid) {
			status = http.StatusConflict
			code = "status_invalid"
		} else if errors.Is(err, model.ErrAffCommissionAlreadyWithdrawn) {
			status = http.StatusConflict
			code = "commission_already_withdrawn"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminListAffWithdrawals GET /api/admin/aff/withdrawals
func AdminListAffWithdrawals(c *gin.Context) {
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := model.ListAffWithdrawalsAdmin(status, limit, offset)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// adminAffWithdrawConfirm 兼容旧客户端仍可读 body；confirm 字段已废弃，恒为通过。
func adminAffWithdrawConfirm(c *gin.Context) bool {
	_ = c.ShouldBindJSON(&affConfirmReq{})
	return true
}

// AdminApproveAffWithdrawal POST /api/admin/aff/withdrawals/:id/approve
func AdminApproveAffWithdrawal(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	if !adminAffWithdrawConfirm(c) {
		return
	}
	row, err := model.ApproveAffWithdrawal(id, c.GetInt("id"))
	if err != nil {
		writeAffWithdrawErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminRejectAffWithdrawal POST /api/admin/aff/withdrawals/:id/reject
func AdminRejectAffWithdrawal(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req affConfirmReq
	_ = c.ShouldBindJSON(&req) // confirm 已废弃；仍读 Reason
	row, err := model.RejectAffWithdrawal(id, c.GetInt("id"), req.Reason)
	if err != nil {
		writeAffWithdrawErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminMarkAffWithdrawalPaid POST /api/admin/aff/withdrawals/:id/mark-paid
func AdminMarkAffWithdrawalPaid(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	if !adminAffWithdrawConfirm(c) {
		return
	}
	row, err := model.MarkAffWithdrawalPaid(id, c.GetInt("id"))
	if err != nil {
		writeAffWithdrawErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminMarkAffWithdrawalFailed POST /api/admin/aff/withdrawals/:id/mark-failed
func AdminMarkAffWithdrawalFailed(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	if !adminAffWithdrawConfirm(c) {
		return
	}
	row, err := model.MarkAffWithdrawalPayFailed(id, c.GetInt("id"))
	if err != nil {
		writeAffWithdrawErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminMarkAffWithdrawalUnknown POST /api/admin/aff/withdrawals/:id/mark-unknown
func AdminMarkAffWithdrawalUnknown(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	if !adminAffWithdrawConfirm(c) {
		return
	}
	row, err := model.MarkAffWithdrawalPayUnknown(id, c.GetInt("id"))
	if err != nil {
		writeAffWithdrawErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminRetryAffWithdrawal POST /api/admin/aff/withdrawals/:id/retry
func AdminRetryAffWithdrawal(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	if !adminAffWithdrawConfirm(c) {
		return
	}
	row, err := model.RetryAffWithdrawal(id, c.GetInt("id"))
	if err != nil {
		if errors.Is(err, model.ErrAffWithdrawRetryForbidden) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error(), "code": "retry_forbidden"})
			return
		}
		writeAffWithdrawErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminListAffSelfInviteReviews GET /api/admin/aff/self-invite-reviews
func AdminListAffSelfInviteReviews(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	status := c.DefaultQuery("status", "pending")
	// reuse risk_reviews filtered by kind
	rows, total, err := model.ListRiskReviewsByKind(model.RiskReviewKindAffSelfInvite, status, limit, offset)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

func writeAffWithdrawErr(c *gin.Context, err error) {
	status := http.StatusOK
	code := "bad_request"
	if errors.Is(err, model.ErrAffWithdrawNotFound) {
		status = http.StatusNotFound
		code = "not_found"
	} else if errors.Is(err, model.ErrAffWithdrawStatusInvalid) {
		status = http.StatusConflict
		code = "status_invalid"
	}
	adtyFail(c, status, code, err.Error(), nil)
}
