package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAdtyDataRetention(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	view, err := service.ListDataRetentionSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdtyDataRetention(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body struct {
		Policies []service.DataRetentionPolicyInput `json:"policies"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveDataRetentionPolicies(c, c.GetInt("id"), c.GetString("username"), body.Policies); err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := service.ListDataRetentionSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func GetAdtyDataRetentionRuns(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	runs, err := service.ListDataRetentionRuns(limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if runs == nil {
		runs = []service.DataRetentionRunView{}
	}
	common.ApiSuccess(c, gin.H{"runs": runs})
}
