package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// AuditIntent retains committed evidence independently of log-store availability
// and of cancellable/cleanable system-task execution history. Payload is frozen
// before the business transaction commits; delivery only changes receipt fields.
type AuditIntent struct {
	ID                int64  `gorm:"primaryKey"`
	EventID           string `gorm:"type:varchar(64);uniqueIndex"`
	Version           int
	Payload           string `gorm:"type:text"`
	CreatedAt         int64  `gorm:"type:bigint"`
	Attempts          int
	NextAttemptAt     int64  `gorm:"type:bigint;index:idx_audit_intent_pending,priority:2"`
	DeliveredAt       int64  `gorm:"type:bigint;index:idx_audit_intent_pending,priority:1"`
	LastErrorCategory string `gorm:"type:varchar(64)"`
}

// AppendAuditIntentTx must share the mutation's primary-database transaction.
// It never calls LOG_DB or reads the live user's role/username on another pool.
func AppendAuditIntentTx(tx *gorm.DB, entry AuditLog) error {
	if tx == nil || entry.EventId == "" || entry.CreatedAt == 0 {
		return errors.New("audit intent requires a prepared event and transaction")
	}
	entry.Id = 0
	payload, err := common.Marshal(entry)
	if err != nil {
		return errors.New("audit intent encoding failed")
	}
	return tx.Create(&AuditIntent{
		EventID: entry.EventId, Version: 1, Payload: string(payload), CreatedAt: entry.CreatedAt,
	}).Error
}

func HasPendingAuditIntents() bool {
	if DB == nil {
		return false
	}
	var count int64
	return DB.Model(&AuditIntent{}).Where("delivered_at = ? AND next_attempt_at <= ?", 0, common.GetTimestamp()).Limit(1).Count(&count).Error == nil && count > 0
}

// DeliverAuditIntent retries the persisted event identity. SQL log stores use a
// unique event_id plus payload confirmation to tolerate lost insert/receipt ACKs.
// ClickHouse currently retains the stable logical identity with at-least-once
// physical inserts; its projection deduplication remains a separate requirement.
func DeliverAuditIntent(ctx context.Context, eventID string) error {
	var intent AuditIntent
	if err := DB.WithContext(ctx).Where("event_id = ?", eventID).Take(&intent).Error; err != nil {
		return err
	}
	if intent.DeliveredAt != 0 {
		return nil
	}
	var entry AuditLog
	category := "payload_invalid"
	err := common.UnmarshalJsonStr(intent.Payload, &entry)
	if err == nil && (intent.Version != 1 || entry.EventId != intent.EventID || entry.CreatedAt != intent.CreatedAt) {
		err = errors.New("invalid audit intent envelope")
	}
	if err == nil {
		category = "log_store_write_failed"
		err = writeAuditLog(ctx, entry, true)
	}
	query := DB.WithContext(ctx).Model(&AuditIntent{}).Where("event_id = ? AND delivered_at = ?", eventID, 0)
	if err != nil {
		// Never persist driver errors: they can contain SQL parameters or DSNs.
		if updateErr := query.Updates(map[string]any{
			"attempts": gorm.Expr("attempts + ?", 1), "next_attempt_at": common.GetTimestamp() + 15,
			"last_error_category": category,
		}).Error; updateErr != nil {
			return errors.New("audit delivery retry receipt unavailable")
		}
		return errors.New("audit delivery " + category)
	}
	if err := query.Updates(map[string]any{
		"attempts": gorm.Expr("attempts + ?", 1), "delivered_at": common.GetTimestamp(),
		"next_attempt_at": 0, "last_error_category": "",
	}).Error; err != nil {
		return errors.New("audit delivery confirmation unavailable")
	}
	return nil
}

// DeliverPendingAuditIntents performs a bounded pass; a cancelled or cleaned
// scheduler run cannot remove the evidence or prevent a later run from retrying.
func DeliverPendingAuditIntents(ctx context.Context, limit int) (int, error) {
	var intents []AuditIntent
	if err := DB.WithContext(ctx).Select("event_id").Where("delivered_at = ? AND next_attempt_at <= ?", 0, common.GetTimestamp()).
		Order("id ASC").Limit(min(max(limit, 1), 100)).Find(&intents).Error; err != nil {
		return 0, errors.New("audit intent scan unavailable")
	}
	delivered := 0
	var deliveryErr error
	for _, intent := range intents {
		if err := ctx.Err(); err != nil {
			return delivered, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := DeliverAuditIntent(attemptCtx, intent.EventID)
		cancel()
		if err != nil {
			deliveryErr = err
			continue
		}
		delivered++
	}
	return delivered, deliveryErr
}
