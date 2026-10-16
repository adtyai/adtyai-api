package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type epayCreateReq struct {
	Name         string `json:"name"`
	PartnerId    string `json:"partner_id"`
	Secret       string `json:"secret"`
	PayAddress   string `json:"pay_address"`
	CallbackBase string `json:"callback_base"`
	DailyLimit   int64  `json:"daily_limit"`
	Enabled      *bool  `json:"enabled"`
}

type epayUpdateReq struct {
	Name         *string `json:"name"`
	PartnerId    *string `json:"partner_id"`
	Secret       *string `json:"secret"`
	PayAddress   *string `json:"pay_address"`
	CallbackBase *string `json:"callback_base"`
	DailyLimit   *int64  `json:"daily_limit"`
	Enabled      *bool   `json:"enabled"`
	Confirm      bool    `json:"confirm"` // 改密钥时必填
}

type epayConfirmReq struct {
	Confirm bool `json:"confirm"`
}

func viewsOfEpay(rows []model.EpayInstance) []model.EpayInstanceView {
	out := make([]model.EpayInstanceView, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].ToView())
	}
	return out
}

// AdminListEpayInstances GET /api/admin/epay-instances
func AdminListEpayInstances(c *gin.Context) {
	rows, err := model.ListEpayInstances()
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": viewsOfEpay(rows)})
}

// AdminGetEpayInstance GET /api/admin/epay-instances/:id
func AdminGetEpayInstance(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	row, err := model.GetEpayInstanceById(id)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrEpayInstanceNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row.ToView()})
}

// AdminCreateEpayInstance POST /api/admin/epay-instances
func AdminCreateEpayInstance(c *gin.Context) {
	var req epayCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := model.CreateEpayInstance(model.CreateEpayInstanceInput{
		Name:         req.Name,
		PartnerId:    req.PartnerId,
		Secret:       req.Secret,
		PayAddress:   req.PayAddress,
		CallbackBase: req.CallbackBase,
		DailyLimit:   req.DailyLimit,
		Enabled:      enabled,
	})
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row.ToView()})
}

// AdminUpdateEpayInstance PUT /api/admin/epay-instances/:id
// 改密钥属敏感操作，须 confirm=true。
func AdminUpdateEpayInstance(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req epayUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid parameters", nil)
		return
	}
	if req.Secret != nil && !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false, "message": "confirm=true required to change secret", "code": "confirm_required",
		})
		return
	}
	// enabled 不得经 PUT 改动：须 POST .../disable 或 .../enable + confirm。
	if req.Enabled != nil {
		msg := "use disable/enable endpoint"
		adtyFail(c, http.StatusBadRequest, "use_disable_endpoint", msg, nil)
		return
	}
	row, err := model.UpdateEpayInstance(id, model.UpdateEpayInstanceInput{
		Name:         req.Name,
		PartnerId:    req.PartnerId,
		Secret:       req.Secret,
		PayAddress:   req.PayAddress,
		CallbackBase: req.CallbackBase,
		DailyLimit:   req.DailyLimit,
	})
	if err != nil {
		status := http.StatusBadRequest
		code := "bad_request"
		if errors.Is(err, model.ErrEpayInstanceNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row.ToView()})
}

// AdminSetEpayInstancePrimary POST /api/admin/epay-instances/:id/set-primary
func AdminSetEpayInstancePrimary(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req epayConfirmReq
	_ = c.ShouldBindJSON(&req)
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false, "message": "confirm=true required", "code": "confirm_required",
		})
		return
	}
	row, err := model.SetEpayInstancePrimary(id)
	if err != nil {
		status := http.StatusBadRequest
		code := "bad_request"
		if errors.Is(err, model.ErrEpayInstanceNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		} else if errors.Is(err, model.ErrEpayInstanceDisabled) {
			status = http.StatusConflict
			code = "status_invalid"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row.ToView()})
}

// AdminDisableEpayInstance POST /api/admin/epay-instances/:id/disable
func AdminDisableEpayInstance(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req epayConfirmReq
	_ = c.ShouldBindJSON(&req)
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false, "message": "confirm=true required", "code": "confirm_required",
		})
		return
	}
	row, err := model.DisableEpayInstance(id)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrEpayInstanceNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row.ToView()})
}

// AdminEnableEpayInstance POST /api/admin/epay-instances/:id/enable
// 重新启用；不自动设主用（enable ≠ set-primary）。须 confirm=true。
func AdminEnableEpayInstance(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	var req epayConfirmReq
	_ = c.ShouldBindJSON(&req)
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false, "message": "confirm=true required", "code": "confirm_required",
		})
		return
	}
	row, err := model.EnableEpayInstance(id)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrEpayInstanceNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row.ToView()})
}
