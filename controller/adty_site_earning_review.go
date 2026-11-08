package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetAdtySiteEarningReviews exposes unresolved historical task adjustments to
// finance. The original site and stable task root remain visible after transfer.
func GetAdtySiteEarningReviews(c *gin.Context) {
	page := common.GetPageQuery(c)
	query := model.DB.Model(&model.AdtySiteEarningEvent{}).Where("review_status = ?", model.AdtyEarningNeedsReview)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var events []model.AdtySiteEarningEvent
	if err := query.Order("created_at, operation_key").Limit(page.GetPageSize()).Offset(page.GetStartIdx()).Find(&events).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"total": total, "items": events})
}

// ReviewAdtySiteEarning resolves an original charge's disposition, rather than
// treating each refund or adjustment as an independent finance decision.
func ReviewAdtySiteEarning(c *gin.Context) {
	var body struct {
		OperationKey   string `json:"operation_key"`
		OriginalCharge string `json:"original_charge"`
		Reason         string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || strings.TrimSpace(body.OperationKey) == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.ReviewAdtySiteEarningRoot(body.OperationKey, c.GetInt("id"), body.OriginalCharge, body.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "adty.finance.site_earning_review", map[string]any{
		"operation_key": body.OperationKey, "original_charge": body.OriginalCharge, "reason": strings.TrimSpace(body.Reason),
	})
	common.ApiSuccess(c, gin.H{"operation_key": body.OperationKey, "original_charge": body.OriginalCharge})
}
