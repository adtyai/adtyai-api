package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	adtySitePriceUniqueIndex = "idx_site_price_model"
	adtySitePriceLegacyIndex = "idx_site_price"
)

// EnsureAdtySitePriceUniqueIndex installs a unique index on
// (site_id, user_id, group, model) so a site cannot store two rows for the
// same customer, group, and model. Existing rows have an implicit empty
// model (the site/user/group default for all models). The new index is
// created before the legacy (site_id, user_id, group) index is dropped, so
// uniqueness is never loosened. Historical rows are not rewritten except
// to replace a NULL model with an empty string, which matches the implicit
// default. idx_site_price_model is the live uniqueness constraint.
func EnsureAdtySitePriceUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("site price unique index: database is nil")
	}
	if !db.Migrator().HasTable(&AdtySitePrice{}) {
		return nil
	}
	if db.Migrator().HasColumn(&AdtySitePrice{}, "model") {
		if err := db.Model(&AdtySitePrice{}).Where("model IS NULL").Update("model", "").Error; err != nil {
			return fmt.Errorf("backfill empty site price model: %w", err)
		}
	}
	if err := ensureAdtySitePriceModelUniqueIndex(db); err != nil {
		return err
	}
	if db.Migrator().HasIndex(&AdtySitePrice{}, adtySitePriceLegacyIndex) {
		if err := db.Migrator().DropIndex(&AdtySitePrice{}, adtySitePriceLegacyIndex); err != nil {
			return fmt.Errorf("drop legacy site price unique index: %w", err)
		}
	}
	return nil
}

func ensureAdtySitePriceModelUniqueIndex(db *gorm.DB) error {
	current, err := adtySitePriceUniqueIndexIsCurrent(db)
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	statement := &gorm.Statement{DB: db}
	table := statement.Quote(AdtySitePrice{}.TableName())
	index := statement.Quote(adtySitePriceUniqueIndex)
	columns := strings.Join([]string{
		statement.Quote("site_id"),
		statement.Quote("user_id"),
		statement.Quote("group"),
		statement.Quote("model"),
	}, ", ")
	var sql string
	switch db.Dialector.Name() {
	case "mysql":
		sql = fmt.Sprintf("ALTER TABLE %s ADD UNIQUE INDEX %s (%s)", table, index, columns)
	default:
		sql = fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)", index, table, columns)
	}
	if err := db.Exec(sql).Error; err != nil {
		if db.Migrator().HasIndex(&AdtySitePrice{}, adtySitePriceUniqueIndex) {
			return nil
		}
		return fmt.Errorf("create site price unique index: %w", err)
	}
	return nil
}

func adtySitePriceUniqueIndexIsCurrent(db *gorm.DB) (bool, error) {
	if !db.Migrator().HasIndex(&AdtySitePrice{}, adtySitePriceUniqueIndex) {
		return false, nil
	}
	var definition string
	var err error
	switch db.Dialector.Name() {
	case "postgres":
		err = db.Raw(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?",
			adtySitePriceUniqueIndex,
		).Scan(&definition).Error
	case "sqlite":
		err = db.Raw(
			"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?",
			adtySitePriceUniqueIndex,
		).Scan(&definition).Error
	default:
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect site price unique index: %w", err)
	}
	normalized := strings.ToLower(definition)
	return strings.Contains(normalized, "unique") &&
		strings.Contains(normalized, "site_id") &&
		strings.Contains(normalized, "user_id") &&
		strings.Contains(normalized, "group") &&
		strings.Contains(normalized, "model"), nil
}
