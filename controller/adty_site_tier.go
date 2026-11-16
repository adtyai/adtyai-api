package controller

import (
	"errors"
	"net/http"
	"time"

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

// PutAdtySiteTierAssignment lets Root pin one site to an enabled level
// (mode manual) or hand it back to the hourly evaluation (mode auto). The
// margin requirement, cost factor and markup limit follow the level at once.
func PutAdtySiteTierAssignment(c *gin.Context) {
	siteID, ok := siteIdParam(c)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
		Mode string `json:"mode"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	site, err := service.SetAdtySiteTier(siteID, body.Code, body.Mode, time.Now().Unix())
	switch {
	case errors.Is(err, service.ErrAdtySiteNotFound):
		common.ApiErrorStatus(c, http.StatusNotFound, err)
	case errors.Is(err, service.ErrAdtySiteTierNotFound):
		common.ApiErrorT(c, "This level does not exist or is disabled")
	case errors.Is(err, service.ErrAdtySiteTierMode):
		common.ApiErrorT(c, "Invalid parameters")
	case err != nil:
		common.ApiError(c, err)
	default:
		recordManageAudit(c, "adty.site.tier", map[string]any{"site_id": siteID, "code": site.TierCode, "mode": site.TierMode})
		common.ApiSuccess(c, site)
	}
}
