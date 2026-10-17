package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type adminRefundReq struct {
	Amount    float64 `json:"amount"`
	Reason    string  `json:"reason"`
	RequestId string  `json:"request_id"`
	Confirm   bool    `json:"confirm"`
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
