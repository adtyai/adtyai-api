package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const activeTeamMembershipUniqueIndex = "ux_adty_team_members_one_active_user"

// EnsureActiveTeamMembershipUniqueIndex installs a partial unique index that
// allows at most one active membership per user. Historical collisions are
// left in place and are not merged, deleted, or rewritten; the index is
// skipped in that case so startup can continue, and new second-team writes
// must still be rejected at the application layer. Memberships marked left
// are outside the index and do not block a later active row on another team.
// idx_adty_team_member on (team_id, user_id) is left unchanged.
func EnsureActiveTeamMembershipUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("active team membership unique index: database is nil")
	}
	if !db.Migrator().HasTable(&AdtyTeamMember{}) {
		return nil
	}
	duplicates, err := countActiveTeamMembershipCollisions(db)
	if err != nil {
		return err
	}
	if duplicates > 0 {
		return fmt.Errorf("%w: %d user(s) already have more than one active membership; existing rows were left unchanged", ErrActiveTeamMembershipCollision, duplicates)
	}
	if db.Dialector.Name() == "mysql" {
		return ensureMySQLActiveTeamMembershipUniqueIndex(db)
	}
	current, err := activeTeamMembershipUniqueIndexIsCurrent(db)
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	if err := dropActiveTeamMembershipUniqueIndex(db); err != nil {
		return err
	}
	statement := &gorm.Statement{DB: db}
	if err := db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s) WHERE %s = '%s'",
		statement.Quote(activeTeamMembershipUniqueIndex),
		statement.Quote(AdtyTeamMember{}.TableName()),
		statement.Quote("user_id"),
		statement.Quote("status"),
		AdtyTeamMemberActive,
	)).Error; err != nil {
		if isActiveTeamMembershipUniqueViolation(err) || isAdtyUniqueViolation(err) {
			return fmt.Errorf("%w: creating the index failed because colliding rows already exist; existing rows were left unchanged", ErrActiveTeamMembershipCollision)
		}
		return fmt.Errorf("create active team membership unique index: %w", err)
	}
	return nil
}

func countActiveTeamMembershipCollisions(db *gorm.DB) (int64, error) {
	var duplicates int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT user_id
			FROM adty_team_members
			WHERE status = ?
			GROUP BY user_id
			HAVING COUNT(*) > 1
		) AS duplicated
	`, AdtyTeamMemberActive).Scan(&duplicates).Error; err != nil {
		return 0, fmt.Errorf("count duplicate active team memberships: %w", err)
	}
	return duplicates, nil
}

func dropActiveTeamMembershipUniqueIndex(db *gorm.DB) error {
	statement := &gorm.Statement{DB: db}
	quotedName := statement.Quote(activeTeamMembershipUniqueIndex)
	var err error
	switch db.Dialector.Name() {
	case "mysql":
		err = db.Exec(fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", statement.Quote(AdtyTeamMember{}.TableName()), quotedName)).Error
	default:
		err = db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", quotedName)).Error
	}
	if err != nil {
		if db.Dialector.Name() == "mysql" && strings.Contains(strings.ToLower(err.Error()), "check that it exists") {
			return nil
		}
		return fmt.Errorf("drop active team membership unique index: %w", err)
	}
	return nil
}

func activeTeamMembershipUniqueIndexIsCurrent(db *gorm.DB) (bool, error) {
	table := AdtyTeamMember{}.TableName()
	if !db.Migrator().HasIndex(table, activeTeamMembershipUniqueIndex) {
		return false, nil
	}
	var definition string
	var err error
	switch db.Dialector.Name() {
	case "postgres":
		err = db.Raw(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?",
			activeTeamMembershipUniqueIndex,
		).Scan(&definition).Error
	case "sqlite":
		err = db.Raw(
			"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?",
			activeTeamMembershipUniqueIndex,
		).Scan(&definition).Error
	default:
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect active team membership unique index: %w", err)
	}
	normalized := strings.ToLower(definition)
	// PostgreSQL may quote identifiers, so "status" = 'active' will not
	// contain the substring status = 'active'.
	hasActivePredicate := strings.Contains(normalized, "status") && strings.Contains(normalized, "'active'")
	return strings.Contains(normalized, "unique") &&
		strings.Contains(normalized, "user_id") &&
		hasActivePredicate, nil
}

func ensureMySQLActiveTeamMembershipUniqueIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex(AdtyTeamMember{}.TableName(), activeTeamMembershipUniqueIndex) {
		return nil
	}
	if err := ensureConditionalUniqueIndex(db, AdtyTeamMember{}.TableName(), activeTeamMembershipUniqueIndex, "user_id", "status = 'active'"); err != nil {
		if isActiveTeamMembershipUniqueViolation(err) || isAdtyUniqueViolation(err) {
			return fmt.Errorf("%w: creating the index failed because colliding rows already exist; existing rows were left unchanged", ErrActiveTeamMembershipCollision)
		}
		return fmt.Errorf("create active team membership unique index: %w", err)
	}
	return nil
}

// IsActiveTeamMembershipUniqueViolation reports a unique-index conflict on
// the one-active-membership index, or on idx_adty_team_member.
func IsActiveTeamMembershipUniqueViolation(err error) bool {
	return isActiveTeamMembershipUniqueViolation(err)
}

func isActiveTeamMembershipUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		name := strings.ToLower(pgErr.ConstraintName)
		if strings.Contains(name, activeTeamMembershipUniqueIndex) || strings.Contains(name, "idx_adty_team_member") {
			return true
		}
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, strings.ToLower(activeTeamMembershipUniqueIndex)) ||
		strings.Contains(text, "idx_adty_team_member")
}
