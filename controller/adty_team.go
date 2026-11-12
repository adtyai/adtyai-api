package controller

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func PostAdtyTeam(c *gin.Context) {
	var body struct {
		SiteId   int    `json:"site_id"`
		Name     string `json:"name"`
		Discount string `json:"discount"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	discount, err := decimal.NewFromString(body.Discount)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	created, err := service.CreateAdtyTeam(c.GetInt("id"), body.SiteId, body.Name, discount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, created)
}

func PostAdtyTeamInvite(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	code, err := service.CreateAdtyTeamInvite(c.GetInt("id"), int64(id), 24*time.Hour)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"code": code})
}

func PostAdtyTeamAccept(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	member, err := service.AcceptAdtyTeamInvite(c.GetInt("id"), body.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}
