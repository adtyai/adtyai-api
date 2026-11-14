package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetAdtyPriceSnapshotBackfill previews historical rows missing a complete
// G.2-11 decimal snapshot. It does not write anything.
func GetAdtyPriceSnapshotBackfill(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		denyAdtyAdminAction(c)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	report, err := service.PreviewPriceSnapshotBackfill(c.GetInt("id"), limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}

// PostAdtyPriceSnapshotBackfill is the Root-only field fill of official
// snapshot strings from already-stored amounts and ratios. Historical reserved
// and actual amounts are not rewritten. dry_run=true previews the same work.
func PostAdtyPriceSnapshotBackfill(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		denyAdtyAdminAction(c)
		return
	}
	var body struct {
		DryRun bool `json:"dry_run"`
		Limit  int  `json:"limit"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var (
		report *service.PriceSnapshotBackfillReport
		err    error
	)
	if body.DryRun {
		report, err = service.PreviewPriceSnapshotBackfill(c.GetInt("id"), body.Limit)
	} else {
		report, err = service.ApplyPriceSnapshotBackfill(c.GetInt("id"), body.Limit)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !body.DryRun {
		recordManageAuditFor(c, 0, "adty.billing.price_snapshot_backfill", map[string]any{
			"filled_tasks": report.FilledTasks, "filled_operations": report.FilledOperations,
			"skipped": report.Skipped, "amounts_preserved": report.AmountsPreserved,
		})
	}
	common.ApiSuccess(c, report)
}
