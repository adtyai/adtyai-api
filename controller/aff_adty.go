package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type affConfirmReq struct {
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
}

type affWithdrawReq struct {
	AmountQuota int64  `json:"amount_quota"`
	IdemKey     string `json:"idem_key"`
	PayMethod   string `json:"pay_method"`
	PayAccount  string `json:"pay_account"`
	UsdtChain   string `json:"usdt_chain"`
	PayoutQrId  int64  `json:"payout_qr_id"`
}

type affMarkPaidReq struct {
	PayoutRef string `json:"payout_ref"`
}

// GetAffSelf GET /api/aff/self
func GetAffSelf(c *gin.Context) {
	uid := c.GetInt("id")
	data, err := model.GetAffSelfSummary(uid)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// ListAffCommissionsSelf GET /api/aff/commissions/self
func ListAffCommissionsSelf(c *gin.Context) {
	uid := c.GetInt("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	status := strings.TrimSpace(c.Query("status"))
	switch status {
	case "", model.AffCommissionStatusFrozen, model.AffCommissionStatusAvailable, model.AffCommissionStatusVoided, model.AffCommissionStatusBlocked:
	default:
		adtyFail(c, http.StatusBadRequest, "invalid_status", "invalid request", nil)
		return
	}
	_, _ = model.ReleaseDueAffCommissions(uid)
	rows, total, err := model.ListAffCommissionsAdminFiltered(model.AffAdminListFilter{
		InviterId: uid,
		Status:    status,
		Limit:     limit,
		Offset:    offset,
	}, true)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// CreateAffWithdraw POST /api/aff/withdraw
func CreateAffWithdraw(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	uid := c.GetInt("id")
	var req affWithdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	row, err := model.CreateAffWithdrawal(uid, model.AffWithdrawCreateInput{
		AmountQuota: req.AmountQuota,
		IdemKey:     strings.TrimSpace(req.IdemKey),
		PayMethod:   req.PayMethod,
		PayAccount:  req.PayAccount,
		UsdtChain:   req.UsdtChain,
		PayoutQrId:  req.PayoutQrId,
	})
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		message := affClientMessage(err)
		params := map[string]any{"amount_quota": req.AmountQuota}
		switch {
		case errors.Is(err, setting.ErrUsdtChainRequired), errors.Is(err, setting.ErrUsdtChainInvalid):
			status = http.StatusBadRequest
			code = "invalid_usdt_chain"
		case errors.Is(err, setting.ErrUsdtPayoutAddressInvalid):
			status = http.StatusBadRequest
			code = "withdraw_address_invalid"
		case errors.Is(err, model.ErrAffWithdrawMethodInvalid):
			status = http.StatusBadRequest
			code = "invalid_pay_method"
			message = "invalid request"
		case errors.Is(err, model.ErrAffAlipayAccountInvalid):
			status = http.StatusBadRequest
			code = "invalid_alipay_account"
			message = "invalid request"
		case errors.Is(err, model.ErrAffPayoutQrInvalid):
			status = http.StatusBadRequest
			code = "invalid_payout_qr"
			message = "invalid request"
		case errors.Is(err, model.ErrAffWithdrawIdempotent), errors.Is(err, model.ErrAffWithdrawIdempotentMismatch):
			// 别人占用了同一个幂等键，或本人同一键但载荷不同。不返回原单，也不回显收款信息。
			status = http.StatusConflict
			code = "bad_request"
			message = "invalid request"
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
		adtyFail(c, status, code, message, params)
		return
	}
	if row == nil || row.UserId != uid {
		adtyFail(c, http.StatusNotFound, "not_found", "not found", nil)
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
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
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
		adtyFail(c, status, code, affClientMessage(err), nil)
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
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	inviterId, _ := strconv.Atoi(c.Query("inviter_id"))
	inviteeId, _ := strconv.Atoi(c.Query("invitee_id"))
	rows, total, err := model.ListAffCommissionsAdminFiltered(model.AffAdminListFilter{
		Status:    c.Query("status"),
		InviterId: inviterId,
		InviteeId: inviteeId,
		Keyword:   c.Query("keyword"),
		Limit:     limit,
		Offset:    offset,
	}, false)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
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
		adtyFail(c, status, code, affClientMessage(err), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminListAffWithdrawals GET /api/admin/aff/withdrawals
func AdminListAffWithdrawals(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	userId, _ := strconv.Atoi(c.Query("user_id"))
	rows, total, err := model.ListAffWithdrawalsAdminFiltered(model.AffAdminListFilter{
		Status:    c.Query("status"),
		PayMethod: c.Query("pay_method"),
		UserId:    userId,
		Keyword:   c.Query("keyword"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
		return
	}
	counts, err := model.CountAffWithdrawalBuckets()
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total, "counts": counts})
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
	if strings.TrimSpace(req.Reason) == "" {
		adtyFail(c, http.StatusBadRequest, "reason_required", "reason required", nil)
		return
	}
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
	var req affMarkPaidReq
	_ = c.ShouldBindJSON(&req)
	row, err := model.MarkAffWithdrawalPaid(id, c.GetInt("id"), req.PayoutRef)
	if err != nil {
		if errors.Is(err, model.ErrAffPayoutRefInvalid) {
			adtyFail(c, http.StatusBadRequest, "invalid_payout_ref", "invalid request", nil)
			return
		}
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
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": affClientMessage(err), "code": "retry_forbidden"})
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
	rows, total, err := model.ListAffSelfInviteReviews(status, limit, offset)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
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
	adtyFail(c, status, code, affClientMessage(err), nil)
}

func affClientMessage(err error) string {
	if err == nil {
		return "invalid request"
	}
	switch {
	case errors.Is(err, model.ErrAffCommissionNotFound),
		errors.Is(err, model.ErrAffWithdrawNotFound),
		errors.Is(err, model.ErrRiskReviewNotFound),
		errors.Is(err, model.ErrAffPayoutQrNotFound):
		return "not found"
	case errors.Is(err, model.ErrAffCommissionStatusInvalid),
		errors.Is(err, model.ErrAffWithdrawStatusInvalid),
		errors.Is(err, model.ErrRiskReviewStatusInvalid):
		return "status invalid"
	case errors.Is(err, model.ErrAffWithdrawBelowMin):
		return "below minimum"
	case errors.Is(err, model.ErrAffWithdrawInsufficient),
		errors.Is(err, model.ErrAffCommissionAlreadyWithdrawn):
		return "insufficient balance"
	case errors.Is(err, model.ErrAffWithdrawRetryForbidden):
		return "retry forbidden"
	case errors.Is(err, model.ErrRiskReviewOrderRefunded):
		return "order refunded"
	case errors.Is(err, model.ErrAffWithdrawMethodInvalid),
		errors.Is(err, model.ErrAffAlipayAccountInvalid),
		errors.Is(err, model.ErrAffPayoutQrInvalid),
		errors.Is(err, model.ErrAffPayoutRefInvalid),
		errors.Is(err, model.ErrAffWithdrawIdempotent),
		errors.Is(err, model.ErrAffWithdrawIdempotentMismatch),
		errors.Is(err, setting.ErrUsdtChainRequired),
		errors.Is(err, setting.ErrUsdtChainInvalid),
		errors.Is(err, setting.ErrUsdtPayoutAddressInvalid):
		return "invalid request"
	default:
		common.SysError(common.LogText("aff request failed: %s", err.Error()))
		return "invalid request"
	}
}
