package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type adtySiteTierBatchRequest struct {
	Tiers *[]model.AdtySiteTier `json:"tiers"`
}

func GetAdtySiteTiers(c *gin.Context) {
	tiers, err := service.ListAdtySiteTiers()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, tiers)
}

func PutAdtySiteTiers(c *gin.Context) {
	var body adtySiteTierBatchRequest
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if body.Tiers == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtySiteTiers(*body.Tiers); err != nil {
		if err == service.ErrAdtySiteTierInvalid {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.site.tiers", map[string]any{"count": len(*body.Tiers)})
	GetAdtySiteTiers(c)
}
