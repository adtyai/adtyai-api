package model

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	optionPrimaryKeyTmpTable = "options_pk_tmp"
	optionPrimaryKeyLockName = "new_api_options_pk"
	optionPrimaryKeyLockID   = 75820193
	optionLegacyTablePrefix  = "options_legacy_"
	optionQuotaPerUnitKey    = "QuotaPerUnit"
)

func migrateOptionPrimaryKey(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("migrate options primary key: database is nil")
	}
	if !db.Migrator().HasTable(&Option{}) {
		return nil
	}
	unique, err := optionsKeyIsUnique(db)
	if err != nil {
		return err
	}
	if unique {
		return nil
	}
	return withOptionPrimaryKeyLock(db, func(locked *gorm.DB) error {
		unique, err := optionsKeyIsUnique(locked)
		if err != nil {
			return err
		}
		if unique {
			return nil
		}
		return repairOptionPrimaryKey(locked)
	})
}

func optionsKeyIsUnique(db *gorm.DB) (bool, error) {
	indexes, err := db.Migrator().GetIndexes(&Option{})
	if err != nil {
		return false, fmt.Errorf("inspect options indexes: %w", err)
	}
	for _, index := range indexes {
		if len(index.Columns()) != 1 || index.Columns()[0] != "key" {
			continue
		}
		if unique, ok := index.Unique(); ok && unique {
			return true, nil
		}
		if primary, ok := index.PrimaryKey(); ok && primary {
			return true, nil
		}
	}
	if db.Dialector.Name() != "postgres" {
		return false, nil
	}
	// PostgreSQL GetIndexes omits constraint-backed primary/unique keys.
	var count int64
	if err := db.Raw(`
SELECT count(*)
FROM pg_catalog.pg_constraint AS constraint_meta
JOIN pg_catalog.pg_attribute AS attribute_meta
  ON attribute_meta.attrelid = constraint_meta.conrelid
 AND attribute_meta.attnum = constraint_meta.conkey[1]
WHERE constraint_meta.conrelid = to_regclass('options')
  AND constraint_meta.contype IN ('p', 'u')
  AND cardinality(constraint_meta.conkey) = 1
  AND attribute_meta.attname = 'key'`).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("inspect options constraints: %w", err)
	}
	return count > 0, nil
}

func withOptionPrimaryKeyLock(db *gorm.DB, fn func(*gorm.DB) error) error {
	switch db.Dialector.Name() {
	case "mysql":
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("lock options table: %w", err)
		}
		ctx := context.Background()
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return fmt.Errorf("lock options table: %w", err)
		}
		defer conn.Close()
		var acquired int
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", optionPrimaryKeyLockName).Scan(&acquired); err != nil {
			return fmt.Errorf("lock options table: %w", err)
		}
		if acquired != 1 {
			return fmt.Errorf("lock options table: timeout")
		}
		defer conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", optionPrimaryKeyLockName)
		return fn(db)
	case "postgres":
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", optionPrimaryKeyLockID).Error; err != nil {
				return fmt.Errorf("lock options table: %w", err)
			}
			return fn(tx)
		})
	default:
		return db.Transaction(func(tx *gorm.DB) error {
			return fn(tx)
		})
	}
}

func repairOptionPrimaryKey(db *gorm.DB) error {
	var rows []Option
	// 显式排序：「保留最后一行」必须是一个能写在文档里的规则。不加 ORDER BY 时
	// PostgreSQL 返回物理顺序，同一份数据换环境、或 VACUUM FULL 之后可能选出
	// 不同的值（ISS-R4-0113）。
	if err := db.Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "value"}}).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("read options rows: %w", err)
	}
	deduped, skippedEmpty, conflicting := dedupeOptionRows(rows)
	if db.Migrator().HasTable(optionPrimaryKeyTmpTable) {
		if err := db.Migrator().DropTable(optionPrimaryKeyTmpTable); err != nil {
			return fmt.Errorf("drop leftover %s: %w", optionPrimaryKeyTmpTable, err)
		}
	}
	if err := db.Table(optionPrimaryKeyTmpTable).Migrator().CreateTable(&Option{}); err != nil {
		return fmt.Errorf("create %s: %w", optionPrimaryKeyTmpTable, err)
	}
	tmpCreated := true
	defer func() {
		if tmpCreated && db.Migrator().HasTable(optionPrimaryKeyTmpTable) {
			_ = db.Migrator().DropTable(optionPrimaryKeyTmpTable)
		}
	}()
	if len(deduped) > 0 {
		if err := db.Table(optionPrimaryKeyTmpTable).CreateInBatches(deduped, 100).Error; err != nil {
			return fmt.Errorf("insert rebuilt options: %w", err)
		}
	}
	var written int64
	if err := db.Table(optionPrimaryKeyTmpTable).Count(&written).Error; err != nil {
		return fmt.Errorf("count rebuilt options: %w", err)
	}
	if int(written) != len(deduped) {
		return fmt.Errorf("options rebuild wrote %d rows, want %d", written, len(deduped))
	}
	backup := fmt.Sprintf("%s%d", optionLegacyTablePrefix, time.Now().UnixNano())
	if err := swapOptionTables(db, optionPrimaryKeyTmpTable, backup); err != nil {
		return err
	}
	tmpCreated = false
	unique, err := optionsKeyIsUnique(db)
	if err != nil {
		return err
	}
	if !unique {
		return fmt.Errorf("options table still has no unique key after rebuild")
	}
	common.SysLog(common.LogText("rebuilt options table with primary key from %d rows into %d keys; previous rows kept in %s", len(rows)-skippedEmpty, len(deduped), backup))
	if len(conflicting) > 0 {
		// 一条汇总，便于事后按这份键名清单补录；逐键的取值在上面各条里。
		common.SysError(common.LogText("options rebuild kept one value for %d conflicting keys: %s", len(conflicting), strings.Join(conflicting, ", ")))
	}
	return nil
}

// dedupeOptionRows 按 Key 去重：冲突时保留排序后的最后一行，并返回发生过值冲突
// 的键（已排序、去重）供调用方汇总。
func dedupeOptionRows(rows []Option) ([]Option, int, []string) {
	seen := make(map[string]int)
	deduped := make([]Option, 0, len(rows))
	skippedEmpty := 0
	conflicting := make([]string, 0)
	for _, row := range rows {
		if row.Key == "" {
			skippedEmpty++
			continue
		}
		value := optionRebuildValue(row.Key, row.Value)
		i, ok := seen[row.Key]
		if !ok {
			seen[row.Key] = len(deduped)
			deduped = append(deduped, Option{Key: row.Key, Value: value})
			continue
		}
		if kept := deduped[i].Value; kept != value {
			conflicting = append(conflicting, row.Key)
			// 值也必须进日志：只留键名时，事后没人能回答「最后选中的是哪个值」，
			// 而这正是本次重建唯一不可逆的决定。
			common.SysError(common.LogText("options key %q had conflicting values: kept=%q dropped=%q", row.Key, value, kept))
		}
		deduped[i].Value = value
	}
	if skippedEmpty > 0 {
		common.SysError(common.LogText("skipped %d options rows with empty keys while rebuilding the primary key", skippedEmpty))
	}
	slices.Sort(conflicting)
	return deduped, skippedEmpty, slices.Compact(conflicting)
}

// optionRebuildValue 返回重建时应当落库的值。
//
// 约定 C 把 QuotaPerUnit 的唯一合法值定死在 500000，已存的其他值加载时一律不
// 生效（model/option.go 的加载分支直接拒绝）。所以这个键根本不需要「按行序猜」：
// 重建直接取合法值，连「不同环境选出不同比例」的余地都没有。其余键保持
// 「保留最后一行」。换掉的值写进错误日志，因为重建是不可逆的。
func optionRebuildValue(key, value string) string {
	legal := strconv.Itoa(fixedQuotaPerUnit)
	if key != optionQuotaPerUnitKey || strings.TrimSpace(value) == legal {
		return value
	}
	common.SysError(common.LogText("options key %q rebuilt with the only legal value: dropped=%q kept=%q", key, value, legal))
	return legal
}

func swapOptionTables(db *gorm.DB, tmp, backup string) error {
	if !optionSafeIdent(tmp) || !optionSafeIdent(backup) {
		return fmt.Errorf("unsafe options table name")
	}
	if db.Dialector.Name() == "mysql" {
		sql := "RENAME TABLE `options` TO `" + backup + "`, `" + tmp + "` TO `options`"
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("rename options tables: %w", err)
		}
		return nil
	}
	if err := db.Migrator().RenameTable("options", backup); err != nil {
		return fmt.Errorf("rename options to %s: %w", backup, err)
	}
	if err := db.Migrator().RenameTable(tmp, "options"); err != nil {
		return fmt.Errorf("rename %s to options: %w", tmp, err)
	}
	return nil
}

func optionSafeIdent(name string) bool {
	if name == "" {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool {
		return r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z')
	})
}
