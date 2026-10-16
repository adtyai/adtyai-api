package controller

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type enterprisePriceCreateReq struct {
	UserId    int     `json:"user_id"`
	GroupCode *string `json:"group_code"`
	ModelName *string `json:"model_name"`
	Discount  float64 `json:"discount"`
	ExpiresAt *string `json:"expires_at"` // RFC3339；空/省略=不过期
	Confirm   bool    `json:"confirm"`
}

type enterprisePriceUpdateReq struct {
	GroupCode *string  `json:"group_code"`
	ModelName *string  `json:"model_name"`
	Discount  *float64 `json:"discount"`
	ExpiresAt *string  `json:"expires_at"` // RFC3339；传 "" 清空
	Confirm   bool     `json:"confirm"`
}

type enterprisePriceDeleteReq struct {
	Confirm bool `json:"confirm"`
}

func parseExpiresAt(s *string) (*time.Time, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	v := *s
	if v == "" {
		return nil, true, nil // clear
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, false, err
	}
	return &t, false, nil
}

func mapEnterprisePriceErr(err error) (int, string) {
	switch {
	case errors.Is(err, model.ErrEnterprisePriceConfirmRequired):
		return http.StatusBadRequest, "confirm_required"
	case errors.Is(err, model.ErrEnterprisePriceBadDiscount):
		return http.StatusBadRequest, "bad_discount"
	case errors.Is(err, model.ErrEnterprisePriceConflict):
		return http.StatusBadRequest, "conflict"
	case errors.Is(err, model.ErrEnterprisePriceUserRequired),
		errors.Is(err, model.ErrEnterprisePriceUserMissing):
		return http.StatusBadRequest, "bad_user"
	case errors.Is(err, model.ErrEnterprisePriceNotFound):
		return http.StatusNotFound, "not_found"
	default:
		return http.StatusBadRequest, "error"
	}
}

// AdminListEnterprisePricing GET /api/admin/enterprise-pricing
func AdminListEnterprisePricing(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	offset, _ := strconv.Atoi(c.DefaultQuery("p", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, total, err := model.ListEnterprisePriceRules(userId, offset, limit)
	if err != nil {
		adtyFail(c, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// AdminCreateEnterprisePricing POST /api/admin/enterprise-pricing
func AdminCreateEnterprisePricing(c *gin.Context) {
	var req enterprisePriceCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	exp, _, err := parseExpiresAt(req.ExpiresAt)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid expires_at", nil)
		return
	}
	row, err := model.CreateEnterprisePriceRule(model.EnterprisePriceCreateInput{
		UserId:    req.UserId,
		GroupCode: req.GroupCode,
		ModelName: req.ModelName,
		Discount:  req.Discount,
		ExpiresAt: exp,
		CreatedBy: c.GetInt("id"),
		Confirm:   req.Confirm,
	})
	if err != nil {
		status, code := mapEnterprisePriceErr(err)
		c.JSON(status, gin.H{"success": false, "message": err.Error(), "code": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminUpdateEnterprisePricing PUT /api/admin/enterprise-pricing/:id
func AdminUpdateEnterprisePricing(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req enterprisePriceUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	exp, clearExp, err := parseExpiresAt(req.ExpiresAt)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid expires_at", nil)
		return
	}
	before, _ := model.GetEnterprisePriceRule(id)
	row, err := model.UpdateEnterprisePriceRule(id, model.EnterprisePriceUpdateInput{
		GroupCode:    req.GroupCode,
		ModelName:    req.ModelName,
		Discount:     req.Discount,
		ExpiresAt:    exp,
		ClearExpires: clearExp,
		Confirm:      req.Confirm,
	})
	if err != nil {
		status, code := mapEnterprisePriceErr(err)
		c.JSON(status, gin.H{"success": false, "message": err.Error(), "code": code})
		return
	}
	params := map[string]any{
		"result": "ok", "target_type": "enterprise_price_rule", "target_id": id,
		"confirm": true, "after": row,
	}
	if before != nil {
		params["before"] = before
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminDeleteEnterprisePricing DELETE /api/admin/enterprise-pricing/:id
func AdminDeleteEnterprisePricing(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req enterprisePriceDeleteReq
	_ = c.ShouldBindJSON(&req)
	// 也接受 query ?confirm=true
	if !req.Confirm {
		if c.Query("confirm") == "true" {
			req.Confirm = true
		}
	}
	before, _ := model.GetEnterprisePriceRule(id)
	err = model.DeleteEnterprisePriceRule(id, req.Confirm)
	if err != nil {
		status, code := mapEnterprisePriceErr(err)
		c.JSON(status, gin.H{"success": false, "message": err.Error(), "code": code})
		return
	}
	params := map[string]any{
		"result": "ok", "target_type": "enterprise_price_rule", "target_id": id, "confirm": true,
	}
	if before != nil {
		params["before"] = before
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "deleted"})
}
