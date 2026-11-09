package model

import (
	"fmt"

	"gorm.io/gorm"
)

// ensureConditionalUniqueIndex enforces uniqueness only for matching rows.
// MySQL 5.7 has no partial indexes, but supports indexed generated columns.
// Use STORED: 5.7.8-rc can omit existing values from virtual-column indexes.
// NULL outside the predicate permits any number of non-matching rows.
// Names and predicates come only from the migration's fixed schema definitions.
func ensureConditionalUniqueIndex(db *gorm.DB, table, index, column, predicate string) error {
	statement := &gorm.Statement{DB: db}
	quotedTable := statement.Quote(table)
	quotedIndex := statement.Quote(index)
	quotedColumn := statement.Quote(column)
	if db.Dialector.Name() != "mysql" {
		return db.Exec(fmt.Sprintf(
			"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s) WHERE %s",
			quotedIndex, quotedTable, quotedColumn, predicate,
		)).Error
	}

	valueColumn := "adty_" + index + "_value"
	quotedValue := statement.Quote(valueColumn)
	type columnDefinition struct {
		ColumnName       string
		ColumnType       string
		CharacterSetName string
		CollationName    string
	}
	var columns []columnDefinition
	if err := db.Raw(`
			SELECT COLUMN_NAME AS column_name, COLUMN_TYPE AS column_type, CHARACTER_SET_NAME AS character_set_name, COLLATION_NAME AS collation_name
			FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name IN ?`,
		table, []string{column, valueColumn}).Scan(&columns).Error; err != nil {
		return fmt.Errorf("inspect conditional index source %s.%s: %w", table, column, err)
	}
	var source, generated columnDefinition
	for _, definition := range columns {
		if definition.ColumnName == column {
			source = definition
		} else {
			generated = definition
		}
	}
	if source.ColumnType == "" {
		return fmt.Errorf("conditional index source %s.%s is missing", table, column)
	}
	if generated.ColumnType != source.ColumnType || generated.CharacterSetName != source.CharacterSetName || generated.CollationName != source.CollationName {
		// Preserve the original column's comparison and length semantics. A
		// fixed-width digest or index prefix could reject distinct identities.
		dataType := source.ColumnType
		if source.CharacterSetName != "" {
			dataType += " CHARACTER SET " + statement.Quote(source.CharacterSetName)
		}
		if source.CollationName != "" {
			dataType += " COLLATE " + statement.Quote(source.CollationName)
		}
		operation := "MODIFY"
		if generated.ColumnName == "" {
			operation = "ADD"
		}
		if err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s %s COLUMN %s %s GENERATED ALWAYS AS (CASE WHEN %s THEN %s ELSE NULL END) STORED",
			quotedTable, operation, quotedValue, dataType, predicate, quotedColumn,
		)).Error; err != nil {
			return fmt.Errorf("add conditional index value %s: %w", index, err)
		}
	}
	if db.Migrator().HasIndex(table, index) {
		return nil
	}
	return db.Exec(fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s)", quotedIndex, quotedTable, quotedValue)).Error
}
