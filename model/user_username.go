package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const usernameUniqueIndex = "ux_users_normalized_username"

// NormalizeUsername folds a login name for uniqueness and lookup.
// The stored display spelling is not rewritten by this helper.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func usernameLockKey(username string) string {
	return "adty-username:" + NormalizeUsername(username)
}

func lockNormalizedUsername(tx *gorm.DB, username string) error {
	username = NormalizeUsername(username)
	if username == "" || tx == nil {
		return nil
	}
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", usernameLockKey(username)).Error; err != nil {
			return err
		}
	}
	return nil
}

func usernameOccupiedQuery(tx *gorm.DB, username string) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	return tx.Unscoped().Model(&User{}).Where("LOWER(username) = ?", NormalizeUsername(username))
}

// EnsureUsernameAvailableWithTx rejects a username that collides with any
// existing row, including soft-deleted tombstones, under the case-insensitive
// platform-wide rule. excludeUserID is the row allowed to keep or change its
// own display spelling.
func EnsureUsernameAvailableWithTx(tx *gorm.DB, username string, excludeUserID int) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	if err := lockNormalizedUsername(tx, username); err != nil {
		return err
	}
	query := usernameOccupiedQuery(tx, username)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrUsernameAlreadyTaken
	}
	return nil
}

// findLiveUserByUsername locates a not-soft-deleted account. An exact spelling
// match wins so historical case-variant rows can still sign in with the name
// they registered. A unique case-insensitive match is used when the typed
// spelling differs. Multiple case-variant rows with no exact match are treated
// as not found so the wrong account is never chosen.
func findLiveUserByUsername(tx *gorm.DB, username string) (*User, error) {
	if tx == nil {
		tx = DB
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var exact User
	err := tx.Where("username = ?", username).First(&exact).Error
	if err == nil {
		return &exact, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var matches []User
	if err := tx.Where("LOWER(username) = ?", NormalizeUsername(username)).Limit(2).Find(&matches).Error; err != nil {
		return nil, err
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	return nil, gorm.ErrRecordNotFound
}

func isUsernameUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		name := strings.ToLower(pgErr.ConstraintName)
		if strings.Contains(name, usernameUniqueIndex) || strings.Contains(name, "username") {
			return true
		}
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, strings.ToLower(usernameUniqueIndex)) ||
		strings.Contains(text, "uni_users_username") ||
		strings.Contains(text, "users_username_key") ||
		strings.Contains(text, "unique constraint failed: users.username")
}

// EnsureUsernameUniqueIndex installs a case-insensitive unique index on
// users.username (LOWER(username) or equivalent). Existing case-variant
// collisions are left in place and are not merged or renamed; the index is
// skipped in that case so startup can continue, and new writes must still be
// rejected at the application layer.
func EnsureUsernameUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("username unique index: database is nil")
	}
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}
	duplicates, err := countNormalizedUsernameCollisions(db)
	if err != nil {
		return err
	}
	if duplicates > 0 {
		return fmt.Errorf("%w: %d normalized username(s) already have colliding rows; existing accounts were left unchanged", ErrUsernameCaseCollision, duplicates)
	}
	if db.Dialector.Name() == "mysql" {
		return ensureMySQLUsernameUniqueIndex(db)
	}
	current, err := usernameUniqueIndexIsCurrent(db)
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	if err := dropUsernameUniqueIndex(db); err != nil {
		return err
	}
	statement := &gorm.Statement{DB: db}
	if err := db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (LOWER(username))",
		statement.Quote(usernameUniqueIndex),
		statement.Quote("users"),
	)).Error; err != nil {
		if isUsernameUniqueViolation(err) || isAdtyUniqueViolation(err) {
			return fmt.Errorf("%w: creating the index failed because colliding rows already exist; existing accounts were left unchanged", ErrUsernameCaseCollision)
		}
		return fmt.Errorf("create username unique index: %w", err)
	}
	return nil
}

func countNormalizedUsernameCollisions(db *gorm.DB) (int64, error) {
	var duplicates int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT LOWER(username) AS normalized_username
			FROM users
			GROUP BY LOWER(username)
			HAVING COUNT(*) > 1
		) AS duplicated
	`).Scan(&duplicates).Error; err != nil {
		return 0, fmt.Errorf("count duplicate usernames: %w", err)
	}
	return duplicates, nil
}

func dropUsernameUniqueIndex(db *gorm.DB) error {
	statement := &gorm.Statement{DB: db}
	quotedName := statement.Quote(usernameUniqueIndex)
	var err error
	switch db.Dialector.Name() {
	case "mysql":
		err = db.Exec(fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", statement.Quote("users"), quotedName)).Error
	default:
		err = db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", quotedName)).Error
	}
	if err != nil {
		if db.Dialector.Name() == "mysql" && strings.Contains(strings.ToLower(err.Error()), "check that it exists") {
			return nil
		}
		return fmt.Errorf("drop username unique index: %w", err)
	}
	return nil
}

func usernameUniqueIndexIsCurrent(db *gorm.DB) (bool, error) {
	if !db.Migrator().HasIndex("users", usernameUniqueIndex) {
		return false, nil
	}
	var definition string
	var err error
	switch db.Dialector.Name() {
	case "postgres":
		err = db.Raw(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?",
			usernameUniqueIndex,
		).Scan(&definition).Error
	case "sqlite":
		err = db.Raw(
			"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?",
			usernameUniqueIndex,
		).Scan(&definition).Error
	default:
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect username unique index: %w", err)
	}
	return strings.Contains(strings.ToLower(definition), "lower(username)"), nil
}

func ensureMySQLUsernameUniqueIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex("users", usernameUniqueIndex) {
		return nil
	}
	statement := &gorm.Statement{DB: db}
	if err := db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX %s ON %s ((LOWER(username)))",
		statement.Quote(usernameUniqueIndex),
		statement.Quote("users"),
	)).Error; err != nil {
		if isUsernameUniqueViolation(err) || isAdtyUniqueViolation(err) {
			return fmt.Errorf("%w: creating the index failed because colliding rows already exist; existing accounts were left unchanged", ErrUsernameCaseCollision)
		}
		return fmt.Errorf("create username unique index: %w", err)
	}
	return nil
}
