package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
)

type adtyMethodBonusBody struct {
	Method  string `json:"method"`
	Enabled bool   `json:"enabled"`
	Rate    string `json:"rate"`
	Allowed *bool  `json:"allowed"`
}

// PutAdtyMainMethodBonus lets Root set one main-site payment-method gift rule.
func PutAdtyMainMethodBonus(c *gin.Context) {
	var body adtyMethodBonusBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := adty_setting.SetMainMethodBonus(body.Method, body.Enabled, body.Rate); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, adty_setting.GetTopupSetting().MethodBonus)
}

// PutAdtySiteBonusMethodAuth lets Root authorize one method for a site.
func PutAdtySiteBonusMethodAuth(c *gin.Context) {
	siteID, err := strconv.Atoi(c.Param("id"))
	if err != nil || siteID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body adtyMethodBonusBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.Allowed == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := adty_setting.AuthorizeSiteBonusMethod(siteID, body.Method, *body.Allowed); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": siteID, "allowed": *body.Allowed})
}

// PutAdtySiteMethodBonus lets a site owner save a gift rule inside Root's authorization.
func PutAdtySiteMethodBonus(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body adtyMethodBonusBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := adty_setting.SetSiteMethodBonus(site.Id, body.Method, body.Enabled, body.Rate); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": site.Id})
}
