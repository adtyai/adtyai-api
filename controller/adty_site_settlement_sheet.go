package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAdtyBusinessTimezone(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	view, err := service.LoadAdtyBusinessTimezone()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdtyBusinessTimezone(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body struct {
		Timezone string `json:"timezone"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtyBusinessTimezone(body.Timezone); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

func GetAdtyOwnerSettlementSheets(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	rows, err := service.ListAdtySiteSettlementSheets(site.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func GetAdtyAdminSettlementSheets(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	siteID, _ := strconv.Atoi(c.Query("site_id"))
	if siteID > 0 {
		rows, err := service.ListAdtySiteSettlementSheets(siteID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, rows)
		return
	}
	rows, err := service.ListAllAdtySiteSettlementSheets()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}
