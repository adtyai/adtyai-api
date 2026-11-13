package service

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// RecordAdtySiteEarningLine stores one earning fact with model and billing group.
// It does not accept or store an upstream channel name.
func RecordAdtySiteEarningLine(siteID, userID int, modelName, group, requestID string, quota int64, k, d float64) error {
	modelName, group, requestID = strings.TrimSpace(modelName), strings.TrimSpace(group), strings.TrimSpace(requestID)
	if siteID <= 0 || userID <= 0 || quota == 0 || modelName == "" || group == "" || k < 1 || d <= 0 || d > 1 || math.IsNaN(k) || math.IsInf(k, 0) {
		return common.NewMessage("Invalid parameters")
	}
	revenue, platform, earning := AdtySiteEarningSplit(quota, decimal.NewFromFloat(k), decimal.NewFromFloat(d))
	row := model.AdtySiteEarningLine{
		SiteId: siteID, UserId: userID, ModelName: modelName, Group: group, RequestId: requestID,
		Quota: quota, Markup: decimal.NewFromFloat(k), CostFactor: decimal.NewFromFloat(d),
		Revenue: revenue, Platform: platform, Earning: earning,
	}
	return model.DB.Create(&row).Error
}

// ListAdtyOwnerEarningLines returns recent earning facts for one site.
func ListAdtyOwnerEarningLines(siteID, offset, limit int) ([]model.AdtySiteEarningLine, error) {
	if siteID <= 0 {
		return nil, common.NewMessage("Invalid parameters")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var rows []model.AdtySiteEarningLine
	err := model.DB.Where("site_id = ?", siteID).Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, err
}
