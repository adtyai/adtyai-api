package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DataRetentionCategoryUserAccountProfile  = "user_account_profile"
	DataRetentionCategoryLoginSecurity       = "login_security"
	DataRetentionCategoryAPIUsageLogs        = "api_usage_logs"
	DataRetentionCategoryOrdersPayments      = "orders_payments"
	DataRetentionCategoryFinancialLedgers    = "financial_ledgers"
	DataRetentionCategorySupportTickets      = "support_tickets"
	DataRetentionCategoryNotificationHistory = "notification_history"
	DataRetentionCategoryOperationAudit      = "operation_audit"
	DataRetentionCategoryTaskArtifacts       = "task_artifacts"

	DataRetentionActionDelete     = "delete"
	DataRetentionActionDeidentify = "deidentify"
	DataRetentionActionExclude    = "exclude"
	DataRetentionTriggerScheduled = "scheduled"
	DataRetentionTriggerManual    = "manual"
	DataRetentionScopePlatform    = "platform"
)

// AdtyDataRetentionPolicy is the Root-configured retention rule for one data category.
// A nil RetentionDays value means the category is unconfigured and must not auto-delete.
type AdtyDataRetentionPolicy struct {
	Category      string `json:"category" gorm:"primaryKey;type:varchar(64)"`
	RetentionDays *int   `json:"retention_days"`
	Action        string `json:"action" gorm:"type:varchar(32);not null;default:''"`
	UpdatedBy     int    `json:"updated_by"`
	UpdatedAt     int64  `json:"updated_at" gorm:"not null"`
}

func (AdtyDataRetentionPolicy) TableName() string { return "adty_data_retention_policies" }

// AdtyDataRetentionRun is one classified retention-task execution.
type AdtyDataRetentionRun struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	StartedAt    int64  `json:"started_at" gorm:"not null;index"`
	FinishedAt   int64  `json:"finished_at" gorm:"not null"`
	Trigger      string `json:"trigger" gorm:"type:varchar(32);not null"`
	Scope        string `json:"scope" gorm:"type:varchar(64);not null"`
	SystemTaskID string `json:"system_task_id" gorm:"type:varchar(64)"`
}

func (AdtyDataRetentionRun) TableName() string { return "adty_data_retention_runs" }

// AdtyDataRetentionRunItem records per-category outcomes for one run.
type AdtyDataRetentionRunItem struct {
	Id              int64  `json:"id" gorm:"primaryKey"`
	RunId           int64  `json:"run_id" gorm:"index;not null"`
	Category        string `json:"category" gorm:"type:varchar(64);not null;index"`
	Rule            string `json:"rule" gorm:"type:varchar(255);not null"`
	Scope           string `json:"scope" gorm:"type:varchar(64);not null"`
	DeleteCount     int64  `json:"delete_count" gorm:"not null;default:0"`
	DeidentifyCount int64  `json:"deidentify_count" gorm:"not null;default:0"`
	FailureCount    int64  `json:"failure_count" gorm:"not null;default:0"`
	SkipReason      string `json:"skip_reason" gorm:"type:text"`
	ExecutedAt      int64  `json:"executed_at" gorm:"not null"`
}

func (AdtyDataRetentionRunItem) TableName() string { return "adty_data_retention_run_items" }

func DataRetentionCategoryOrder() []string {
	return []string{
		DataRetentionCategoryUserAccountProfile,
		DataRetentionCategoryLoginSecurity,
		DataRetentionCategoryAPIUsageLogs,
		DataRetentionCategoryOrdersPayments,
		DataRetentionCategoryFinancialLedgers,
		DataRetentionCategorySupportTickets,
		DataRetentionCategoryNotificationHistory,
		DataRetentionCategoryOperationAudit,
		DataRetentionCategoryTaskArtifacts,
	}
}

func IsDataRetentionCategory(category string) bool {
	for _, item := range DataRetentionCategoryOrder() {
		if item == category {
			return true
		}
	}
	return false
}

func GetAdtyDataRetentionPolicies() (map[string]AdtyDataRetentionPolicy, error) {
	rows := []AdtyDataRetentionPolicy{}
	if DB == nil {
		return nil, errors.New("database unavailable")
	}
	if !DB.Migrator().HasTable(&AdtyDataRetentionPolicy{}) {
		return map[string]AdtyDataRetentionPolicy{}, nil
	}
	if err := DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]AdtyDataRetentionPolicy, len(rows))
	for _, row := range rows {
		out[row.Category] = row
	}
	return out, nil
}

func GetAdtyDataRetentionPolicy(category string) (AdtyDataRetentionPolicy, bool, error) {
	if DB == nil {
		return AdtyDataRetentionPolicy{}, false, errors.New("database unavailable")
	}
	if !DB.Migrator().HasTable(&AdtyDataRetentionPolicy{}) {
		return AdtyDataRetentionPolicy{}, false, nil
	}
	var row AdtyDataRetentionPolicy
	err := DB.Where("category = ?", category).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdtyDataRetentionPolicy{}, false, nil
	}
	if err != nil {
		return AdtyDataRetentionPolicy{}, false, err
	}
	return row, true, nil
}

func UpsertAdtyDataRetentionPolicy(row AdtyDataRetentionPolicy) error {
	if row.UpdatedAt == 0 {
		row.UpdatedAt = common.GetTimestamp()
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "category"}},
		DoUpdates: clause.AssignmentColumns([]string{"retention_days", "action", "updated_by", "updated_at"}),
	}).Create(&row).Error
}

func ClearAdtyDataRetentionPolicy(category string) error {
	if DB == nil {
		return errors.New("database unavailable")
	}
	if !DB.Migrator().HasTable(&AdtyDataRetentionPolicy{}) {
		return nil
	}
	result := DB.Where("category = ?", category).Delete(&AdtyDataRetentionPolicy{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func CreateAdtyDataRetentionRun(run *AdtyDataRetentionRun, items []AdtyDataRetentionRunItem) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].RunId = run.Id
			if err := tx.Create(&items[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ListAdtyDataRetentionRuns(limit int) ([]AdtyDataRetentionRun, map[int64][]AdtyDataRetentionRunItem, error) {
	if limit <= 0 {
		limit = 20
	}
	runs := []AdtyDataRetentionRun{}
	if err := DB.Order("started_at DESC, id DESC").Limit(limit).Find(&runs).Error; err != nil {
		return nil, nil, err
	}
	itemsByRun := map[int64][]AdtyDataRetentionRunItem{}
	if len(runs) == 0 {
		return runs, itemsByRun, nil
	}
	ids := make([]int64, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.Id)
	}
	items := []AdtyDataRetentionRunItem{}
	if err := DB.Where("run_id IN ?", ids).Order("id").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	for _, item := range items {
		itemsByRun[item.RunId] = append(itemsByRun[item.RunId], item)
	}
	return runs, itemsByRun, nil
}

// DeleteOldQuotaDataBatch removes usage-statistic rows older than targetTimestamp.
func DeleteOldQuotaDataBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if DB == nil {
		return 0, errors.New("database unavailable")
	}
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		result := DB.WithContext(ctx).Exec(
			"DELETE FROM quota_data WHERE id IN (SELECT id FROM (SELECT id FROM quota_data WHERE created_at < ? ORDER BY id LIMIT ?) AS old_quota)",
			targetTimestamp, limit,
		)
		if result.Error != nil {
			return 0, result.Error
		}
		return result.RowsAffected, nil
	}
	result := DB.WithContext(ctx).Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&QuotaData{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// DeleteOldLogsOfTypesBatch deletes log rows of the given types older than targetTimestamp.
func DeleteOldLogsOfTypesBatch(ctx context.Context, types []int, targetTimestamp int64, limit int) (int64, error) {
	if len(types) == 0 {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if LOG_DB == nil {
		return 0, errors.New("log database unavailable")
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		var total int64
		if err := LOG_DB.WithContext(ctx).Model(&Log{}).Where("created_at < ? AND type IN ?", targetTimestamp, types).Count(&total).Error; err != nil {
			return 0, err
		}
		if total == 0 {
			return 0, nil
		}
		args := make([]any, 0, 1+len(types))
		args = append(args, targetTimestamp)
		placeholders := ""
		for i, logType := range types {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
			args = append(args, logType)
		}
		if err := LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE logs DELETE WHERE created_at < ? AND type IN ("+placeholders+") SETTINGS mutations_sync = 1",
			args...,
		).Error; err != nil {
			return 0, err
		}
		return total, nil
	}
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		sub := LOG_DB.WithContext(ctx).Model(&Log{}).Select("id").Where("created_at < ? AND type IN ?", targetTimestamp, types).Order("id").Limit(limit)
		result := LOG_DB.WithContext(ctx).Where("id IN (?)", sub).Delete(&Log{})
		if result.Error != nil {
			return 0, result.Error
		}
		return result.RowsAffected, nil
	}
	result := LOG_DB.WithContext(ctx).Where("created_at < ? AND type IN ?", targetTimestamp, types).Limit(limit).Delete(&Log{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// DeleteOldAuditLogsByCategoriesBatch deletes audit rows in the given categories older than targetTimestamp.
func DeleteOldAuditLogsByCategoriesBatch(ctx context.Context, categories []string, targetTimestamp int64, limit int) (int64, error) {
	if len(categories) == 0 {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if LOG_DB == nil {
		return 0, errors.New("log database unavailable")
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		var total int64
		if err := LOG_DB.WithContext(ctx).Table("audit_logs").Where("created_at < ? AND category IN ?", targetTimestamp, categories).Count(&total).Error; err != nil {
			return 0, err
		}
		if total == 0 {
			return 0, nil
		}
		args := make([]any, 0, 1+len(categories))
		args = append(args, targetTimestamp)
		placeholders := ""
		for i, category := range categories {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
			args = append(args, category)
		}
		if err := LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE audit_logs DELETE WHERE created_at < ? AND category IN ("+placeholders+") SETTINGS mutations_sync = 1",
			args...,
		).Error; err != nil {
			return 0, err
		}
		return total, nil
	}
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		sub := LOG_DB.WithContext(ctx).Table("audit_logs").Select("id").Where("created_at < ? AND category IN ?", targetTimestamp, categories).Order("id").Limit(limit)
		result := LOG_DB.WithContext(ctx).Table("audit_logs").Where("id IN (?)", sub).Delete(&AuditLog{})
		if result.Error != nil {
			return 0, result.Error
		}
		return result.RowsAffected, nil
	}
	result := LOG_DB.WithContext(ctx).Table("audit_logs").Where("created_at < ? AND category IN ?", targetTimestamp, categories).Limit(limit).Delete(&AuditLog{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// DeleteInactiveUserSessionsForRetention removes expired or revoked sessions older than retentionDays.
// Active, unexpired sessions are left in place. The issuance window still applies as an operational safety bound.
func DeleteInactiveUserSessionsForRetention(now int64, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	if common.UserSessionIssuanceWindowSeconds <= 0 {
		return 0, ErrUserSessionInvalid
	}
	issuanceCutoff := now - common.UserSessionIssuanceWindowSeconds
	cutoff := now - int64(retentionDays)*24*60*60
	deleted := int64(0)
	for {
		var sids []string
		if err := DB.Model(&UserSession{}).
			Where(
				"created_at <= ? AND ((status = ? AND revoked_at > 0 AND revoked_at < ?) OR (expires_at < ? AND (status <> ? OR revoked_at <= 0 OR revoked_at < ?)))",
				issuanceCutoff,
				UserSessionStatusRevoked,
				cutoff,
				cutoff,
				UserSessionStatusRevoked,
				cutoff,
			).
			Order("id").Limit(userSessionCleanupScanLimit).Pluck("sid", &sids).Error; err != nil {
			return deleted, err
		}
		if len(sids) == 0 {
			return deleted, nil
		}
		for start := 0; start < len(sids); start += userSessionCleanupBatchSize {
			end := min(start+userSessionCleanupBatchSize, len(sids))
			result := DB.Where("sid IN ?", sids[start:end]).
				Where(
					"created_at <= ? AND ((status = ? AND revoked_at > 0 AND revoked_at < ?) OR (expires_at < ? AND (status <> ? OR revoked_at <= 0 OR revoked_at < ?)))",
					issuanceCutoff,
					UserSessionStatusRevoked,
					cutoff,
					cutoff,
					UserSessionStatusRevoked,
					cutoff,
				).
				Delete(&UserSession{})
			if result.Error != nil {
				return deleted, result.Error
			}
			deleted += result.RowsAffected
		}
	}
}

// DeleteExpiredUserAccessTokensForRetention deletes access tokens that expired before the retention cutoff.
func DeleteExpiredUserAccessTokensForRetention(now int64, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	before := now - int64(retentionDays)*24*60*60
	deleted := int64(0)
	for {
		var ids []int
		if err := DB.Model(&UserAccessToken{}).
			Where("expires_at > 0 AND expires_at < ?", before).
			Order("id").Limit(userAccessTokenCleanupBatchSize).Pluck("id", &ids).Error; err != nil {
			return deleted, err
		}
		if len(ids) == 0 {
			return deleted, nil
		}
		result := DB.Where("id IN ?", ids).Delete(&UserAccessToken{})
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if len(ids) < userAccessTokenCleanupBatchSize {
			return deleted, nil
		}
	}
}
