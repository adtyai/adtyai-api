package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const adtyVoucherTradeNoUniqueIndex = "ux_adty_vouchers_trade_no"

// EnsureAdtyVoucherTradeNoUniqueIndex installs a partial unique index on
// adty_vouchers.trade_no for non-empty values. Empty trade numbers on
// unredeemed codes stay outside the index so many pending rows can coexist.
// Historical collisions are left in place and are not merged, deleted, or
// rewritten; the index is skipped in that case so startup can continue, and
// new duplicate trade numbers must still be rejected at the application
// layer. The ordinary trade_no index created by AutoMigrate is left in place.
func EnsureAdtyVoucherTradeNoUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("voucher trade_no unique index: database is nil")
	}
	if !db.Migrator().HasTable(&AdtyVoucher{}) {
		return nil
	}
	duplicates, err := countAdtyVoucherTradeNoCollisions(db)
	if err != nil {
		return err
	}
	if duplicates > 0 {
		return fmt.Errorf("%w: %d non-empty trade_no value(s) already have colliding rows; existing rows were left unchanged", ErrAdtyVoucherTradeNoCollision, duplicates)
	}
	if db.Dialector.Name() == "mysql" {
		return ensureMySQLAdtyVoucherTradeNoUniqueIndex(db)
	}
	current, err := adtyVoucherTradeNoUniqueIndexIsCurrent(db)
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	if err := dropAdtyVoucherTradeNoUniqueIndex(db); err != nil {
		return err
	}
	statement := &gorm.Statement{DB: db}
	quotedColumn := statement.Quote("trade_no")
	if err := db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s) WHERE %s IS NOT NULL AND %s <> ''",
		statement.Quote(adtyVoucherTradeNoUniqueIndex),
		statement.Quote(AdtyVoucher{}.TableName()),
		quotedColumn,
		quotedColumn,
		quotedColumn,
	)).Error; err != nil {
		if isAdtyVoucherTradeNoUniqueViolation(err) || isAdtyUniqueViolation(err) {
			return fmt.Errorf("%w: creating the index failed because colliding rows already exist; existing rows were left unchanged", ErrAdtyVoucherTradeNoCollision)
		}
		return fmt.Errorf("create voucher trade_no unique index: %w", err)
	}
	return nil
}

func countAdtyVoucherTradeNoCollisions(db *gorm.DB) (int64, error) {
	var duplicates int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT trade_no
			FROM adty_vouchers
			WHERE trade_no IS NOT NULL AND trade_no <> ''
			GROUP BY trade_no
			HAVING COUNT(*) > 1
		) AS duplicated
	`).Scan(&duplicates).Error; err != nil {
		return 0, fmt.Errorf("count duplicate voucher trade_no values: %w", err)
	}
	return duplicates, nil
}

func dropAdtyVoucherTradeNoUniqueIndex(db *gorm.DB) error {
	statement := &gorm.Statement{DB: db}
	quotedName := statement.Quote(adtyVoucherTradeNoUniqueIndex)
	var err error
	switch db.Dialector.Name() {
	case "mysql":
		err = db.Exec(fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", statement.Quote(AdtyVoucher{}.TableName()), quotedName)).Error
	default:
		err = db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", quotedName)).Error
	}
	if err != nil {
		if db.Dialector.Name() == "mysql" && strings.Contains(strings.ToLower(err.Error()), "check that it exists") {
			return nil
		}
		return fmt.Errorf("drop voucher trade_no unique index: %w", err)
	}
	return nil
}

func adtyVoucherTradeNoUniqueIndexIsCurrent(db *gorm.DB) (bool, error) {
	table := AdtyVoucher{}.TableName()
	if !db.Migrator().HasIndex(table, adtyVoucherTradeNoUniqueIndex) {
		return false, nil
	}
	var definition string
	var err error
	switch db.Dialector.Name() {
	case "postgres":
		err = db.Raw(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?",
			adtyVoucherTradeNoUniqueIndex,
		).Scan(&definition).Error
	case "sqlite":
		err = db.Raw(
			"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?",
			adtyVoucherTradeNoUniqueIndex,
		).Scan(&definition).Error
	default:
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect voucher trade_no unique index: %w", err)
	}
	normalized := strings.ToLower(definition)
	hasNonEmptyPredicate := strings.Contains(normalized, "trade_no") &&
		(strings.Contains(normalized, "<> ''") ||
			strings.Contains(normalized, "<> ''::text") ||
			strings.Contains(normalized, "<> ''::character") ||
			strings.Contains(normalized, "!= ''") ||
			strings.Contains(normalized, "<> ''::varchar"))
	return strings.Contains(normalized, "unique") && hasNonEmptyPredicate, nil
}

func ensureMySQLAdtyVoucherTradeNoUniqueIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex(AdtyVoucher{}.TableName(), adtyVoucherTradeNoUniqueIndex) {
		return nil
	}
	if err := ensureConditionalUniqueIndex(db, AdtyVoucher{}.TableName(), adtyVoucherTradeNoUniqueIndex, "trade_no", "trade_no <> ''"); err != nil {
		if isAdtyVoucherTradeNoUniqueViolation(err) || isAdtyUniqueViolation(err) {
			return fmt.Errorf("%w: creating the index failed because colliding rows already exist; existing rows were left unchanged", ErrAdtyVoucherTradeNoCollision)
		}
		return fmt.Errorf("create voucher trade_no unique index: %w", err)
	}
	return nil
}

// AdtyVoucherTradeNoTaken reports whether a non-empty redeem order number is
// already stored on a voucher, a compatibility TopUp row, or an official
// AdtyTopupOrder. Empty values are never treated as taken.
func AdtyVoucherTradeNoTaken(tx *gorm.DB, tradeNo string) (bool, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return false, nil
	}
	if tx == nil {
		tx = DB
	}
	var vouchers int64
	if err := tx.Model(&AdtyVoucher{}).Where("trade_no = ?", tradeNo).Limit(1).Count(&vouchers).Error; err != nil {
		return false, err
	}
	if vouchers > 0 {
		return true, nil
	}
	var topUps int64
	if err := tx.Model(&TopUp{}).Where("trade_no = ?", tradeNo).Limit(1).Count(&topUps).Error; err != nil {
		return false, err
	}
	if topUps > 0 {
		return true, nil
	}
	var orders int64
	if err := tx.Model(&AdtyTopupOrder{}).Where("trade_no = ?", tradeNo).Limit(1).Count(&orders).Error; err != nil {
		return false, err
	}
	return orders > 0, nil
}

// IsAdtyVoucherTradeNoUniqueViolation reports a unique-index conflict on the
// non-empty voucher trade_no index, or on the official/compatibility order
// trade_no unique constraints that share the same redeem order number.
func IsAdtyVoucherTradeNoUniqueViolation(err error) bool {
	return isAdtyVoucherTradeNoUniqueViolation(err)
}

func isAdtyVoucherTradeNoUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		name := strings.ToLower(pgErr.ConstraintName)
		if voucherOrOrderTradeNoConstraint(name) {
			return true
		}
	}
	text := strings.ToLower(err.Error())
	if !strings.Contains(text, "trade_no") && !strings.Contains(text, "tradeno") {
		return strings.Contains(text, strings.ToLower(adtyVoucherTradeNoUniqueIndex))
	}
	return voucherOrOrderTradeNoConstraint(text) ||
		strings.Contains(text, strings.ToLower(adtyVoucherTradeNoUniqueIndex))
}

func voucherOrOrderTradeNoConstraint(name string) bool {
	name = strings.ToLower(name)
	if strings.Contains(name, adtyVoucherTradeNoUniqueIndex) {
		return true
	}
	hasTradeNo := strings.Contains(name, "trade_no") || strings.Contains(name, "tradeno")
	if !hasTradeNo {
		return false
	}
	return strings.Contains(name, "adty_vouchers") ||
		strings.Contains(name, "top_ups") ||
		strings.Contains(name, "topups") ||
		strings.Contains(name, "adty_topup_orders")
}
