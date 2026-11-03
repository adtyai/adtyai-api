package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetAdtyUserLimits 返回一个用户的请求限制（任务 7.4，用户·查看）。-1 表示跟默认。
func GetAdtyUserLimits(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	limit, err := model.GetAdtyUserLimit(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, limit)
}

// PutAdtyUserLimits 设一个用户的请求限制（任务 7.4，用户·编辑）。每项不小于 -1。
func PutAdtyUserLimits(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		RPM         int `json:"rpm"`
		TPM         int `json:"tpm"`
		Concurrency int `json:"concurrency"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.RPM < -1 || body.TPM < -1 || body.Concurrency < -1 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	limit := model.AdtyUserLimit{UserId: userID, RPM: body.RPM, TPM: body.TPM, Concurrency: body.Concurrency}
	if err := model.SaveAdtyUserLimit(limit); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, userID, "adty.user.limits", map[string]any{"rpm": body.RPM, "tpm": body.TPM, "concurrency": body.Concurrency})
	common.ApiSuccess(c, limit)
}
