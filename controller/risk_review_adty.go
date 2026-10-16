package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type adminRiskReviewConfirmReq struct {
	Confirm bool `json:"confirm"`
}

// AdminListRiskReviews GET /api/admin/risk-reviews
func AdminListRiskReviews(c *gin.Context) {
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := model.ListRiskReviews(status, limit, offset)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"total":   total,
	})
}

// AdminGetRiskReview GET /api/admin/risk-reviews/:id
func AdminGetRiskReview(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	row, err := model.GetRiskReviewById(id)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrRiskReviewNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminApproveRiskReview POST /api/admin/risk-reviews/:id/approve
// 动钱接口；成功后写最小审计（谁/哪条/何时/旧→新/金额）。不再要求 confirm。
func AdminApproveRiskReview(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	_ = c.ShouldBindJSON(&adminRiskReviewConfirmReq{}) // confirm 已废弃
	row, err := model.ApproveRiskReview(id, c.GetInt("id"))
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		var params map[string]any
		if errors.Is(err, model.ErrRiskReviewNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		} else if errors.Is(err, model.ErrRiskReviewStatusInvalid) {
			status = http.StatusConflict
			code = "status_invalid"
		} else if errors.Is(err, model.ErrRiskReviewOrderRefunded) {
			status = http.StatusConflict
			code = "risk_review_order_refunded"
			var re *model.RiskReviewOrderRefundedError
			if errors.As(err, &re) {
				params = map[string]any{"order_no": re.OrderNo, "refund_status": re.RefundStatus}
			}
		}
		adtyFail(c, status, code, err.Error(), params)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminRejectRiskReview POST /api/admin/risk-reviews/:id/reject
// 不入账；写最小审计。不再要求 confirm。
func AdminRejectRiskReview(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	_ = c.ShouldBindJSON(&adminRiskReviewConfirmReq{}) // confirm 已废弃
	row, err := model.RejectRiskReview(id, c.GetInt("id"))
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrRiskReviewNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		} else if errors.Is(err, model.ErrRiskReviewStatusInvalid) {
			status = http.StatusConflict
			code = "status_invalid"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}
