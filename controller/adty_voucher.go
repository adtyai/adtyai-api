package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func PostAdtyVoucherRedeem(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	result, err := service.RedeemAdtyVoucher(c.GetInt("id"), body.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func PostAdtySiteVoucher(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		Purpose   string `json:"purpose"`
		AmountUSD string `json:"amount_usd"`
		Source    string `json:"source"`
		Reason    string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	amount, err := decimal.NewFromString(body.AmountUSD)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionVoucherIssue, site.Id) {
		return
	}
	issued, err := service.IssueAdtyVoucher(service.AdtyVoucherIssueInput{
		IssuerId: c.GetInt("id"), Purpose: body.Purpose, SiteId: site.Id, AmountUSD: amount, Source: body.Source, Reason: body.Reason,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, issued)
}

func PostAdtyAdminVoucher(c *gin.Context) {
	var body struct {
		Purpose   string `json:"purpose"`
		SiteId    int    `json:"site_id"`
		AmountUSD string `json:"amount_usd"`
		Reason    string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	amount, err := decimal.NewFromString(body.AmountUSD)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	issued, err := service.IssueAdtyVoucher(service.AdtyVoucherIssueInput{
		IssuerId: c.GetInt("id"), Purpose: body.Purpose, SiteId: body.SiteId, AmountUSD: amount, Reason: body.Reason,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, issued)
}

func PostAdtyVoucherFreeze(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	// Root admin freeze stays on the existing G.14.7 path and is not part of ISS-R2-0048.
	if c.GetInt("role") != common.RoleRootUser && !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionVoucherFreeze, id) {
		return
	}
	if err := service.FreezeAdtyVoucher(c.GetInt("id"), int64(id)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": model.AdtyVoucherStatusFrozen})
}

func PutAdtyVoucherMarginAuthorization(c *gin.Context) {
	siteId, ok := adtyPathId(c)
	if !ok {
		return
	}
	var body struct {
		Allowed bool `json:"allowed"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SetAdtyVoucherMarginAuthorization(c.GetInt("id"), siteId, body.Allowed); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": siteId, "allowed": body.Allowed})
}

func PostAdtyVoucherCancel(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionVoucherCancel, id) {
		return
	}
	if err := service.CancelAdtyVoucher(c.GetInt("id"), int64(id)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": model.AdtyVoucherStatusCancelled})
}

func PutAdtyVoucherIssuanceLimit(c *gin.Context) {
	var body struct {
		LimitUSD string `json:"limit_usd"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	limit, err := decimal.NewFromString(body.LimitUSD)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SetAdtyVoucherIssuanceLimit(c.GetInt("id"), limit); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"limit_usd": limit.String()})
}

// GetAdtyAdminVouchers lists every code for Root, optionally by status and purpose.
func GetAdtyAdminVouchers(c *gin.Context) {
	page := common.GetPageQuery(c)
	items, total, err := service.ListAdtyVouchers(service.AdtyVoucherListFilter{
		Status: strings.TrimSpace(c.Query("status")), Purpose: strings.TrimSpace(c.Query("purpose")),
	}, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"total": total, "items": items, "page": page.GetPage(), "page_size": page.GetPageSize()})
}

// GetAdtySiteVouchers lists the codes this owner issued from this site's
// wallets, and whether Root allows issuing from margin.
func GetAdtySiteVouchers(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := service.ListAdtyVouchers(service.AdtyVoucherListFilter{
		Status: strings.TrimSpace(c.Query("status")), IssuerId: c.GetInt("id"), SiteId: site.Id,
	}, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"total": total, "items": items, "page": page.GetPage(), "page_size": page.GetPageSize(),
		"margin_allowed": service.AdtyVoucherMarginAuthorized(site.Id),
	})
}

func GetAdtyVoucherIssuanceLimit(c *gin.Context) {
	view, err := service.LoadAdtyVoucherIssuance()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func GetAdtyVoucherMarginAuthorization(c *gin.Context) {
	siteId, ok := adtyPathId(c)
	if !ok {
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": siteId, "allowed": service.AdtyVoucherMarginAuthorized(siteId)})
}

// PostAdtyAdminVoucherCancel lets Root cancel any unredeemed code and release
// its source. The route is Root-only, so no owner step-up applies.
func PostAdtyAdminVoucherCancel(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	if err := service.CancelAdtyVoucher(c.GetInt("id"), int64(id)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": model.AdtyVoucherStatusCancelled})
}
