package controller

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type adminRefundReq struct {
	Amount        float64                  `json:"amount"`
	Reason        string                   `json:"reason"`
	RequestId     string                   `json:"request_id"`
	Confirm       bool                     `json:"confirm"`
	AmountConfirm float64                  `json:"amount_confirm"`
	Preview       *model.RefundPreviewEcho `json:"preview"`
}

// AdminPreviewTopupRefund GET /api/admin/orders/:no/refund-preview?amount=
func AdminPreviewTopupRefund(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		adtyFail(c, http.StatusForbidden, "AUTH_INSUFFICIENT_PRIVILEGE", "root required", nil)
		return
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Query("amount")), 64)
	if err != nil || amount <= 0 {
		adtyFail(c, http.StatusBadRequest, "refund_amount_invalid", model.ErrRefundAmountInvalid.Error(), nil)
		return
	}
	preview, err := model.PreviewTopupRefund(c.Param("no"), amount)
	if err != nil {
		writeRefundError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}

func writeRefundError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrTopupOrderNotFound):
		adtyFail(c, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, model.ErrRefundIdempotent):
		adtyFail(c, http.StatusConflict, "refund_idempotent", err.Error(), nil)
	case errors.Is(err, model.ErrRefundAmountInvalid):
		adtyFail(c, http.StatusBadRequest, "refund_amount_invalid", err.Error(), nil)
	case errors.Is(err, model.ErrRefundExceedsAvailable):
		adtyFail(c, http.StatusBadRequest, "refund_exceeds_available", err.Error(), nil)
	case errors.Is(err, model.ErrTopupOrderStatusInvalid):
		adtyFail(c, http.StatusConflict, "order_status_invalid", err.Error(), nil)
	default:
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
	}
}

// AdminRefundTopupOrder POST /api/admin/orders/:no/refund
// 敏感操作：必带 amount、reason、request_id；系统只记账。不再要求 confirm。
// 审查重审 P2-1：动钱均 root（与 complete 对齐）。
func AdminRefundTopupOrder(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		adtyFail(c, http.StatusForbidden, "AUTH_INSUFFICIENT_PRIVILEGE", "Unauthorized, insufficient privileges", nil)
		return
	}
	orderNo := strings.TrimSpace(c.Param("no"))
	if orderNo == "" {
		adtyFail(c, http.StatusBadRequest, "order_no_required", "order no required", nil)
		return
	}
	var req adminRefundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	if strings.TrimSpace(req.RequestId) == "" {
		adtyFail(c, http.StatusBadRequest, "request_id_required", "request_id required", nil)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		adtyFail(c, http.StatusBadRequest, "reason_required", "reason required", nil)
		return
	}
	if req.Amount <= 0 {
		adtyFail(c, http.StatusBadRequest, "amount_invalid", "amount must be > 0", nil)
		return
	}
	if req.Preview == nil {
		adtyFail(c, http.StatusBadRequest, "preview_required", model.ErrRefundPreviewRequired.Error(), nil)
		return
	}
	if req.Amount >= model.RefundLargeAmountUSD && math.Abs(req.Amount-req.AmountConfirm) > 1e-9 {
		adtyFail(c, http.StatusBadRequest, "amount_confirm_required", model.ErrRefundAmountConfirmRequired.Error(), nil)
		return
	}
	quoted, err := model.PreviewTopupRefund(orderNo, req.Amount)
	if err != nil {
		writeRefundError(c, err)
		return
	}
	if !model.RefundPreviewMatches(quoted, *req.Preview) {
		adtyFail(c, http.StatusConflict, "preview_stale", model.ErrRefundPreviewStale.Error(), nil)
		return
	}

	result, err := model.RefundTopupOrder(orderNo, req.Amount, req.Reason, req.RequestId, c.GetInt("id"))
	if err != nil {
		status := http.StatusOK
		code := "refund_failed"
		msg := err.Error()
		var params map[string]any
		switch {
		case errors.Is(err, model.ErrRefundIdempotent):
			status = http.StatusConflict
			code = "refund_idempotent"
		case errors.Is(err, model.ErrRefundExceedsAvailable):
			status = http.StatusBadRequest
			code = "refund_exceeds_available"
			msg = "refund exceeds available"
			params = map[string]any{"amount": req.Amount}
			var ex *model.RefundExceedsAvailableError
			if errors.As(err, &ex) {
				params["available"] = ex.Available
				params["amount"] = ex.Amount
			}
		case errors.Is(err, model.ErrRefundRequestIdRequired),
			errors.Is(err, model.ErrRefundReasonRequired),
			errors.Is(err, model.ErrRefundAmountInvalid),
			errors.Is(err, model.ErrRefundConfirmRequired):
			status = http.StatusBadRequest
			code = "bad_request"
		case errors.Is(err, model.ErrTopupOrderNotFound):
			status = http.StatusNotFound
			code = "not_found"
		case errors.Is(err, model.ErrRefundOrderNotPaid),
			errors.Is(err, model.ErrTopupOrderStatusInvalid):
			status = http.StatusConflict
			code = "order_status_invalid"
		}
		adtyFail(c, status, code, msg, params)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
