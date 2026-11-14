package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	priceSnapshotBackfillDefaultLimit = 200
	priceSnapshotBackfillMaxLimit     = 1000
	priceSnapshotBackfillSkipSample   = 50
)

var ErrPriceSnapshotBackfillForbidden = common.NewMessage("Unauthorized, insufficient privileges")

type PriceSnapshotBackfillSkip struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type PriceSnapshotBackfillReport struct {
	DryRun            bool                        `json:"dry_run"`
	Limit             int                         `json:"limit"`
	TasksScanned      int                         `json:"tasks_scanned"`
	OperationsScanned int                         `json:"operations_scanned"`
	AlreadyComplete   int                         `json:"already_complete"`
	FilledTasks       int                         `json:"filled_tasks"`
	FilledOperations  int                         `json:"filled_operations"`
	Skipped           int                         `json:"skipped"`
	SkippedSample     []PriceSnapshotBackfillSkip `json:"skipped_sample,omitempty"`
	Remaining         bool                        `json:"remaining"`
	AmountsPreserved  bool                        `json:"amounts_preserved"`
	MoreRemain        bool                        `json:"more_remain"`
}

func PreviewPriceSnapshotBackfill(actorID int, limit int) (*PriceSnapshotBackfillReport, error) {
	return runPriceSnapshotBackfill(actorID, limit, true)
}

func ApplyPriceSnapshotBackfill(actorID int, limit int) (*PriceSnapshotBackfillReport, error) {
	return runPriceSnapshotBackfill(actorID, limit, false)
}

func runPriceSnapshotBackfill(actorID int, limit int, dryRun bool) (*PriceSnapshotBackfillReport, error) {
	if err := requirePriceSnapshotBackfillRoot(actorID); err != nil {
		return nil, err
	}
	limit = normalizePriceSnapshotBackfillLimit(limit)
	report := &PriceSnapshotBackfillReport{DryRun: dryRun, Limit: limit, AmountsPreserved: true}
	if err := backfillHistoricalTaskSnapshots(model.DB, limit, dryRun, report); err != nil {
		return nil, err
	}
	if report.filledCount() < limit {
		if err := backfillHistoricalOperationSnapshots(model.DB, limit-report.filledCount(), dryRun, report); err != nil {
			return nil, err
		}
	}
	report.MoreRemain = report.Remaining
	return report, nil
}

func (r *PriceSnapshotBackfillReport) filledCount() int {
	if r == nil {
		return 0
	}
	return r.FilledTasks + r.FilledOperations
}

func normalizePriceSnapshotBackfillLimit(limit int) int {
	if limit <= 0 {
		return priceSnapshotBackfillDefaultLimit
	}
	if limit > priceSnapshotBackfillMaxLimit {
		return priceSnapshotBackfillMaxLimit
	}
	return limit
}

func requirePriceSnapshotBackfillRoot(actorID int) error {
	if actorID <= 0 {
		return ErrPriceSnapshotBackfillForbidden
	}
	var actor model.User
	if err := model.DB.First(&actor, actorID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPriceSnapshotBackfillForbidden
		}
		return err
	}
	if actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled || actor.StaffSiteId != 0 {
		return ErrPriceSnapshotBackfillForbidden
	}
	return nil
}

func backfillHistoricalTaskSnapshots(db *gorm.DB, limit int, dryRun bool, report *PriceSnapshotBackfillReport) error {
	var after int64
	for report.filledCount() < limit {
		var tasks []model.Task
		if err := db.Where("id > ?", after).Order("id").Limit(200).Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}
		for i := range tasks {
			task := &tasks[i]
			after = task.ID
			report.TasksScanned++
			outcome, skip, amountsOK, filledOp, err := backfillOneTaskSnapshot(db, task, dryRun)
			if err != nil {
				return err
			}
			if !amountsOK {
				report.AmountsPreserved = false
			}
			switch outcome {
			case "filled":
				report.FilledTasks++
				if filledOp {
					report.FilledOperations++
				}
				if report.filledCount() >= limit {
					var more int64
					if err := db.Model(&model.Task{}).Where("id > ?", task.ID).Limit(1).Count(&more).Error; err != nil {
						return err
					}
					if more > 0 {
						report.Remaining = true
					}
					return nil
				}
			case "already_complete":
				report.AlreadyComplete++
			case "skipped":
				notePriceSnapshotSkip(report, skip)
			}
		}
	}
	return nil
}

func backfillHistoricalOperationSnapshots(db *gorm.DB, limit int, dryRun bool, report *PriceSnapshotBackfillReport) error {
	if limit <= 0 {
		report.Remaining = true
		return nil
	}
	var after string
	filledThisPass := 0
	for filledThisPass < limit {
		var rows []model.BillingOperation
		q := db.Where("price_snapshot IS NULL OR price_snapshot = ''")
		if after != "" {
			q = q.Where("operation_key > ?", after)
		}
		if err := q.Order("operation_key").Limit(200).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			row := &rows[i]
			after = row.OperationKey
			report.OperationsScanned++
			if row.RequestPriceSnapshot() != nil {
				report.AlreadyComplete++
				continue
			}
			src, ok := historicalSourceFromOperation(row)
			if !ok {
				notePriceSnapshotSkip(report, PriceSnapshotBackfillSkip{Kind: "billing_operation", ID: row.OperationKey, Reason: model.PriceSnapshotBackfillSkipNoStoredPricing})
				continue
			}
			snap, reason := model.BuildHistoricalRequestPriceSnapshot(src)
			if reason == model.PriceSnapshotBackfillSkipAlreadyComplete {
				report.AlreadyComplete++
				continue
			}
			if snap == nil {
				notePriceSnapshotSkip(report, PriceSnapshotBackfillSkip{Kind: "billing_operation", ID: row.OperationKey, Reason: reason})
				continue
			}
			if dryRun {
				report.FilledOperations++
				filledThisPass++
				if filledThisPass >= limit {
					report.Remaining = true
					return nil
				}
				continue
			}
			preserved, err := persistOperationPriceSnapshot(db, row, snap)
			if err != nil {
				return err
			}
			if !preserved {
				report.AmountsPreserved = false
			}
			report.FilledOperations++
			filledThisPass++
			if filledThisPass >= limit {
				report.Remaining = true
				return nil
			}
		}
	}
	return nil
}

func backfillOneTaskSnapshot(db *gorm.DB, task *model.Task, dryRun bool) (outcome string, skip PriceSnapshotBackfillSkip, amountsOK bool, filledOp bool, err error) {
	amountsOK = true
	src, ok := historicalSourceFromTask(task)
	if !ok {
		return "", PriceSnapshotBackfillSkip{}, true, false, nil
	}
	snap, reason := model.BuildHistoricalRequestPriceSnapshot(src)
	if reason == model.PriceSnapshotBackfillSkipAlreadyComplete {
		return "already_complete", PriceSnapshotBackfillSkip{}, true, false, nil
	}
	id := fmt.Sprintf("%d", task.ID)
	if task.TaskID != "" {
		id = task.TaskID
	}
	if snap == nil {
		return "skipped", PriceSnapshotBackfillSkip{Kind: "task", ID: id, Reason: reason}, true, false, nil
	}
	if dryRun {
		return "filled", PriceSnapshotBackfillSkip{}, true, false, nil
	}
	preserved, err := persistTaskPriceSnapshot(db, task, snap)
	if err != nil {
		return "", PriceSnapshotBackfillSkip{}, false, false, err
	}
	if key := task.PrivateData.BillingOperationKey; key != "" {
		var op model.BillingOperation
		if loadErr := db.Where("operation_key = ?", key).First(&op).Error; loadErr == nil && op.RequestPriceSnapshot() == nil {
			ok, persistErr := persistOperationPriceSnapshot(db, &op, snap)
			if persistErr != nil {
				return "", PriceSnapshotBackfillSkip{}, false, false, persistErr
			}
			if !ok {
				preserved = false
			} else {
				filledOp = true
			}
		} else if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return "", PriceSnapshotBackfillSkip{}, false, false, loadErr
		}
	}
	return "filled", PriceSnapshotBackfillSkip{}, preserved, filledOp, nil
}

func historicalSourceFromTask(task *model.Task) (model.HistoricalPriceSnapshotSource, bool) {
	if task == nil || task.PrivateData.BillingContext == nil {
		return model.HistoricalPriceSnapshotSource{}, false
	}
	bc := task.PrivateData.BillingContext
	accepted := task.SubmitTime
	if accepted == 0 {
		accepted = task.CreatedAt
	}
	group := task.Group
	return model.HistoricalPriceSnapshotSource{
		ModelName:       firstNonEmpty(bc.OriginModelName, task.Properties.OriginModelName),
		BillingGroup:    group,
		ModelRatio:      bc.ModelRatio,
		ModelPrice:      bc.ModelPrice,
		GroupRatio:      bc.GroupRatio,
		SiteMarkup:      bc.SiteMarkup,
		SiteCostFactor:  bc.SiteCostFactor,
		SiteId:          bc.SiteId,
		SiteOwnerUserId: bc.SiteOwnerUserId,
		PerCallBilling:  bc.PerCallBilling,
		Tiered:          bc.TieredSnapshot != nil && bc.TieredSnapshot.BillingMode == "tiered_expr",
		AcceptedAt:      accepted,
		Existing:        bc.PriceSnapshot,
	}, true
}

func historicalSourceFromOperation(row *model.BillingOperation) (model.HistoricalPriceSnapshotSource, bool) {
	if row == nil {
		return model.HistoricalPriceSnapshotSource{}, false
	}
	if row.BillingContext == "" {
		return model.HistoricalPriceSnapshotSource{}, false
	}
	var bc model.TaskBillingContext
	if err := common.UnmarshalJsonStr(row.BillingContext, &bc); err != nil {
		return model.HistoricalPriceSnapshotSource{}, false
	}
	return model.HistoricalPriceSnapshotSource{
		ModelName:       bc.OriginModelName,
		ModelRatio:      bc.ModelRatio,
		ModelPrice:      bc.ModelPrice,
		GroupRatio:      bc.GroupRatio,
		SiteMarkup:      firstPositive(bc.SiteMarkup, row.SiteMarkup),
		SiteCostFactor:  firstPositive(bc.SiteCostFactor, row.SiteCostFactor),
		SiteId:          firstPositiveInt(bc.SiteId, row.SiteId),
		SiteOwnerUserId: firstPositiveInt(bc.SiteOwnerUserId, row.SiteOwnerUserId),
		PerCallBilling:  bc.PerCallBilling,
		Tiered:          bc.TieredSnapshot != nil && bc.TieredSnapshot.BillingMode == "tiered_expr",
		AcceptedAt:      row.CreatedAt,
		Existing:        row.RequestPriceSnapshot(),
	}, true
}

func persistTaskPriceSnapshot(db *gorm.DB, task *model.Task, snap *model.RequestPriceSnapshot) (bool, error) {
	preserved := true
	err := db.Transaction(func(tx *gorm.DB) error {
		var current model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, task.ID).Error; err != nil {
			return err
		}
		if current.PrivateData.BillingContext == nil {
			return nil
		}
		bc := current.PrivateData.BillingContext
		reservedQuota := current.Quota
		modelRatio, groupRatio := bc.ModelRatio, bc.GroupRatio
		siteMarkup, siteCost := bc.SiteMarkup, bc.SiteCostFactor
		if bc.PriceSnapshot != nil && bc.PriceSnapshot.HasOfficialFields() {
			return nil
		}
		bc.PriceSnapshot = snap
		if err := tx.Model(&current).Update("private_data", current.PrivateData).Error; err != nil {
			return err
		}
		var after model.Task
		if err := tx.First(&after, task.ID).Error; err != nil {
			return err
		}
		if after.Quota != reservedQuota || after.PrivateData.BillingContext == nil ||
			after.PrivateData.BillingContext.ModelRatio != modelRatio ||
			after.PrivateData.BillingContext.GroupRatio != groupRatio ||
			after.PrivateData.BillingContext.SiteMarkup != siteMarkup ||
			after.PrivateData.BillingContext.SiteCostFactor != siteCost {
			preserved = false
			return errors.New("historical billing amounts or ratios changed during snapshot backfill")
		}
		task.PrivateData = after.PrivateData
		return nil
	})
	return preserved, err
}

func persistOperationPriceSnapshot(db *gorm.DB, row *model.BillingOperation, snap *model.RequestPriceSnapshot) (bool, error) {
	encoded, err := model.EncodeRequestPriceSnapshot(snap)
	if err != nil || encoded == "" {
		return true, err
	}
	preserved := true
	err = db.Transaction(func(tx *gorm.DB) error {
		var current model.BillingOperation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_key = ?", row.OperationKey).First(&current).Error; err != nil {
			return err
		}
		if current.RequestPriceSnapshot() != nil {
			return nil
		}
		reserved, actual, applied := current.ReservedQuota, current.ActualQuota, current.AppliedQuota
		markup, cost := current.SiteMarkup, current.SiteCostFactor
		if err := tx.Model(&current).Update("price_snapshot", encoded).Error; err != nil {
			return err
		}
		var after model.BillingOperation
		if err := tx.Where("operation_key = ?", row.OperationKey).First(&after).Error; err != nil {
			return err
		}
		if after.ReservedQuota != reserved || after.ActualQuota != actual || after.AppliedQuota != applied ||
			after.SiteMarkup != markup || after.SiteCostFactor != cost {
			preserved = false
			return errors.New("historical billing amounts or ratios changed during snapshot backfill")
		}
		row.PriceSnapshot = after.PriceSnapshot
		return nil
	})
	return preserved, err
}

func notePriceSnapshotSkip(report *PriceSnapshotBackfillReport, skip PriceSnapshotBackfillSkip) {
	if report == nil || skip.Reason == "" {
		return
	}
	report.Skipped++
	if len(report.SkippedSample) < priceSnapshotBackfillSkipSample {
		report.SkippedSample = append(report.SkippedSample, skip)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstPositive(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
