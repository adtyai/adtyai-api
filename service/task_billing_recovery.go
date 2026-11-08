package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const adtyDurableBillingContextKey = "adty_durable_billing"
const adtyBillingOperationContextKey = "adty_billing_operation_key"

var activeBillingReservations sync.Map
var billingRecoveryOnce sync.Once
var billingProcessLeaseOwner = common.NewRequestId()

// AttachTaskBillingReservation connects the pre-upstream reservation to the
// durable task barrier. InsertWithContext commits the task and settlement
// intent together, so death before SettleBilling cannot strand the charge.
func AttachTaskBillingReservation(info *relaycommon.RelayInfo, task *model.Task) {
	if info == nil || task == nil {
		return
	}
	if session, ok := info.Billing.(*BillingSession); ok && session.operationKey != "" {
		task.PrivateData.BillingOperationKey = session.operationKey
		quota := session.tokenConsumed
		task.PrivateData.TokenConsumedQuota = &quota
	}
}

func projectBillingEarnings(key string) {
	if err := projectAdtySiteEarningEvent(context.Background(), key); err != nil {
		common.SysError(common.LogText("site earning projection remains pending: %s", err.Error()))
	}
}

func projectAdtySiteEarningEvent(ctx context.Context, key string) error {
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []model.AdtySiteEarningEvent
		if err := model.AdtyLockForUpdate(tx).Where("operation_key = ?", key).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 || rows[0].Projected || rows[0].ReviewStatus == model.AdtyEarningNeedsReview {
			return nil
		}
		event := rows[0]
		revenue, platform, earning := AdtySiteEarningSplit(int64(event.Quota), decimal.NewFromFloat(event.Markup), decimal.NewFromFloat(event.CostFactor))
		kind := model.AdtySiteEarningUsage
		if event.Quota < 0 {
			kind = model.AdtySiteEarningRefund
		}
		if event.ReviewStatus == model.AdtyEarningOriginalSkipped || event.RiskFlag != "" && !event.RiskReleased {
			kind = model.AdtySiteEarningSkipped
		}
		// A separate day slot keeps durable events distinguishable from legacy
		// log-backed earnings during migration, without changing their kind.
		if err := upsertAdtySiteEarningTx(tx, event.Day+"+op", adtySiteEarningEntry{SiteId: event.SiteId, UserId: event.UserId,
			Kind: kind, Reason: event.RiskFlag, Quota: int64(event.Quota), Revenue: revenue, Platform: platform, Earning: earning}); err != nil {
			return err
		}
		return tx.Model(&model.AdtySiteEarningEvent{}).Where("operation_key = ?", key).Update("projected", true).Error
	})
}

// RecoverBillingOperations retries durable decisions and expires only unbound
// reservations whose heartbeat has stopped.
func RecoverBillingOperations(ctx context.Context, limit int) error {
	limit = min(max(limit, 1), 1000)
	cutoff := time.Now().Add(-2 * time.Minute).Unix()
	var abandoned []model.BillingOperation
	if err := model.DB.WithContext(ctx).Where("state = ? AND reference_id = 0 AND updated_at < ?", model.BillingReserved, cutoff).
		Order("updated_at").Limit(limit).Find(&abandoned).Error; err != nil {
		return err
	}
	for _, op := range abandoned {
		if err := model.QueueExpiredBillingRefund(op.OperationKey, cutoff); err != nil {
			return err
		}
	}
	var pending []model.BillingOperation
	if err := model.DB.WithContext(ctx).Where("state IN ?", []string{model.BillingSettlePending, model.BillingRefundPending}).
		Order("updated_at, operation_key").Limit(limit).Find(&pending).Error; err != nil {
		return err
	}
	var first error
	for _, op := range pending {
		if _, _, err := model.ApplyBillingSettlement(op.OperationKey); err != nil && first == nil {
			first = err
		}
	}
	var tasks []model.Task
	if err := model.DB.WithContext(ctx).Where("billing_pending = ? AND billing_initial_pending = ?", true, false).
		Where("status IN ?", []string{model.TaskStatusSuccess, model.TaskStatusFailure}).Order("id").Limit(limit).Find(&tasks).Error; err != nil {
		return err
	}
	for i := range tasks {
		task := &tasks[i]
		if task.Status == model.TaskStatusFailure {
			RefundTaskQuota(ctx, task, task.FailReason)
			continue
		}
		if GetTaskAdaptorFunc == nil {
			continue
		}
		adaptor := GetTaskAdaptorFunc(task.Platform)
		if adaptor == nil {
			continue
		}
		result := task.PrivateData.CompletionBilling
		if result == nil {
			result = &relaycommon.TaskInfo{Status: string(task.Status)}
		}
		finalizeTerminalTask(ctx, adaptor, task, result)
	}
	var mjs []model.Midjourney
	if err := model.DB.WithContext(ctx).Where("billing_pending = ? OR billing_charge_pending = ?", true, true).
		Order("id").Limit(limit).Find(&mjs).Error; err != nil {
		return err
	}
	for i := range mjs {
		task := &mjs[i]
		if task.Status == "FAILURE" {
			RefundMidjourneyQuota(ctx, task, task.FailReason)
		} else if task.BillingChargePending {
			_, err := settleDurableMidjourney(task)
			if err != nil && first == nil {
				first = err
			}
		} else {
			if err := model.DB.Model(task).Update("billing_pending", false).Error; err != nil && first == nil {
				first = err
			}
		}
	}
	var events []model.AdtySiteEarningEvent
	if err := model.DB.WithContext(ctx).Where("projected = ? AND (review_status <> ? OR review_status IS NULL)", false, model.AdtyEarningNeedsReview).Order("created_at, operation_key").Limit(limit).Find(&events).Error; err != nil {
		return err
	}
	for _, event := range events {
		if err := projectAdtySiteEarningEvent(ctx, event.OperationKey); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// StartBillingRecovery runs on every serving node, including non-master nodes,
// so long requests renew their reservations. Process death stops heartbeats;
// any node can then atomically recover the expired reservation.
func StartBillingRecovery() {
	billingRecoveryOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				keys := []string{}
				activeBillingReservations.Range(func(key, _ any) bool { keys = append(keys, key.(string)); return true })
				if len(keys) > 0 {
					if err := model.DB.Model(&model.BillingOperation{}).Where("operation_key IN ? AND state = ? AND lease_owner = ?", keys, model.BillingReserved, billingProcessLeaseOwner).
						UpdateColumn("updated_at", time.Now().Unix()).Error; err != nil {
						common.SysError(common.LogText("billing reservation heartbeat failed: %s", err.Error()))
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := RecoverBillingOperations(ctx, 100); err != nil {
					common.SysError(common.LogText("billing recovery remains pending: %s", err.Error()))
				}
				cancel()
				<-ticker.C
			}
		}()
	})
}
