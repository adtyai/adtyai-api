package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const siteEmailUniqueIndex = "ux_users_site_normalized_email"

// SiteEmailCodeKey scopes a verification or password-reset code to one site so
// the same address on two sites cannot share or consume each other's codes.
func SiteEmailCodeKey(email string, siteID int) string {
	return strconv.Itoa(siteID) + ":" + NormalizeEmail(email)
}

func siteEmailLockKey(email string, siteID int) string {
	return SiteEmailCodeKey(email, siteID)
}

func siteIDForUser(tx *gorm.DB, userID int) (int, error) {
	if tx == nil {
		tx = DB
	}
	var user User
	if err := tx.Select("site_id").First(&user, userID).Error; err != nil {
		return 0, err
	}
	return user.SiteId, nil
}

func createUserWithEmailConflict(tx *gorm.DB, user *User) error {
	if err := tx.Create(user).Error; err != nil {
		if isSiteEmailUniqueViolation(err) {
			return ErrEmailAlreadyTaken
		}
		return err
	}
	return nil
}

func isSiteEmailUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return strings.Contains(pgErr.ConstraintName, siteEmailUniqueIndex)
	}
	text := err.Error()
	return strings.Contains(text, siteEmailUniqueIndex)
}

// EnsureSiteEmailUniqueIndex makes a non-empty email unique within a site.
// Empty emails may still repeat. Same-site duplicates must be resolved first.
func EnsureSiteEmailUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("site email unique index: database is nil")
	}
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}
	var duplicates int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT site_id, LOWER(email) AS normalized_email
			FROM users
			WHERE email <> ''
			GROUP BY site_id, LOWER(email)
			HAVING COUNT(*) > 1
		) AS duplicated
	`).Scan(&duplicates).Error; err != nil {
		return fmt.Errorf("count duplicate site emails: %w", err)
	}
	if duplicates > 0 {
		return fmt.Errorf("duplicate site emails must be resolved before the unique index")
	}
	if db.Dialector.Name() == "mysql" {
		return ensureMySQLSiteEmailUniqueIndex(db)
	}
	statement := &gorm.Statement{DB: db}
	return db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (site_id, LOWER(email)) WHERE email <> ''",
		statement.Quote(siteEmailUniqueIndex),
		statement.Quote("users"),
	)).Error
}

func ensureMySQLSiteEmailUniqueIndex(db *gorm.DB) error {
	statement := &gorm.Statement{DB: db}
	valueColumn := "adty_" + siteEmailUniqueIndex + "_value"
	quotedTable := statement.Quote("users")
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
		"users", []string{"email", valueColumn}).Scan(&columns).Error; err != nil {
		return fmt.Errorf("inspect site email index source: %w", err)
	}
	var source, generated columnDefinition
	for _, definition := range columns {
		if definition.ColumnName == "email" {
			source = definition
		} else {
			generated = definition
		}
	}
	if source.ColumnType == "" {
		return fmt.Errorf("site email index source users.email is missing")
	}
	// site_id (up to 11 digits) + ':' + the email value.
	dataType := "varchar(320)"
	if source.CharacterSetName != "" {
		dataType += " CHARACTER SET " + statement.Quote(source.CharacterSetName)
	}
	if source.CollationName != "" {
		dataType += " COLLATE " + statement.Quote(source.CollationName)
	}
	if generated.ColumnType == "" || !strings.HasPrefix(strings.ToLower(generated.ColumnType), "varchar") {
		operation := "MODIFY"
		if generated.ColumnName == "" {
			operation = "ADD"
		}
		if err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s %s COLUMN %s %s GENERATED ALWAYS AS (CASE WHEN email <> '' THEN CONCAT(site_id, ':', LOWER(email)) ELSE NULL END) STORED",
			quotedTable, operation, quotedValue, dataType,
		)).Error; err != nil {
			return fmt.Errorf("add site email index value: %w", err)
		}
	}
	if db.Migrator().HasIndex("users", siteEmailUniqueIndex) {
		return nil
	}
	return db.Exec(fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s)", statement.Quote(siteEmailUniqueIndex), quotedTable, quotedValue)).Error
}
