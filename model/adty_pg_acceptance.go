package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ErrPostgresAcceptanceDSNUnset 表示官方测试 DSN 未设置。
// 依赖 PostgreSQL 的验收必须跳过并标明 BLK-ENV-PG，不能记为通过。
var ErrPostgresAcceptanceDSNUnset = errors.New("BLK-ENV-PG: ADTY_TEST_PG_DSN / TEST_POSTGRES_DSN is unset")

// OfficialPostgresThrowawayPrefix2310 是 ADTY-23-10 验收用的一次性库名前缀。
const OfficialPostgresThrowawayPrefix2310 = "adty_m2310_"

// OfficialPostgresThrowawayPrefix1617 是 A.16.7 关站迁移矩阵验收用的一次性库名前缀。
const OfficialPostgresThrowawayPrefix1617 = "adty_m1617_"

var officialThrowawayPostgresNamePattern = regexp.MustCompile(`^adty_m[0-9]{4}_[0-9a-f]{16}$`)
var officialThrowawayPostgresPrefixPattern = regexp.MustCompile(`^adty_m[0-9]{4}_$`)

// OfficialPostgresAcceptanceDSN 只接受本方案的官方 PostgreSQL 测试连接串。
// SQLite / MySQL 连接串不能关闭需要真实 PostgreSQL 的验收项。
func OfficialPostgresAcceptanceDSN() (string, error) {
	dsn := strings.TrimSpace(os.Getenv("ADTY_TEST_PG_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	}
	if dsn == "" {
		return "", ErrPostgresAcceptanceDSNUnset
	}
	if !IsPostgresDSN(dsn) {
		return "", errors.New("BLK-ENV-PG: official test DSN is not PostgreSQL; SQLite/MySQL cannot close this PostgreSQL acceptance")
	}
	return dsn, nil
}

// ReservedPostgresDatabase reports whether name is a shared cluster database
// that acceptance tests must never drop.
func ReservedPostgresDatabase(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "postgres", "template0", "template1":
		return true
	default:
		return false
	}
}

// OfficialThrowawayPostgresName reports whether name is a disposable
// acceptance database created on the already-running local instance.
func OfficialThrowawayPostgresName(name string) bool {
	return officialThrowawayPostgresNamePattern.MatchString(name)
}

// OpenOfficialPostgresAcceptance opens a PostgreSQL connection with the same
// official migrator dialector used by production migrateDB.
func OpenOfficialPostgresAcceptance(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgresMigrationDialector{postgres.Dialector{Config: &postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}}}, newGormConfig(false))
}

// CloseOfficialGorm closes the underlying sql.DB.
func CloseOfficialGorm(db *gorm.DB) error {
	return closeAdtyGorm(db)
}

// OfficialPostgresServerVersion records the live server version as evidence only.
// Version is not a pass/fail condition.
func OfficialPostgresServerVersion(db *gorm.DB) (versionNum, version string, err error) {
	if err = db.Raw("SHOW server_version_num").Scan(&versionNum).Error; err != nil {
		return "", "", err
	}
	if err = db.Raw("SHOW server_version").Scan(&version).Error; err != nil {
		return "", "", err
	}
	return versionNum, version, nil
}

// NewOfficialThrowawayPostgresName returns a validated disposable database name.
func NewOfficialThrowawayPostgresName(prefix string) (string, error) {
	if !officialThrowawayPostgresPrefixPattern.MatchString(prefix) {
		return "", fmt.Errorf("invalid throwaway database prefix %q", prefix)
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	name := prefix + hex.EncodeToString(buf)
	if !OfficialThrowawayPostgresName(name) || ReservedPostgresDatabase(name) {
		return "", fmt.Errorf("invalid throwaway database name %q", name)
	}
	return name, nil
}

// CreateOfficialThrowawayPostgresDatabase creates an empty disposable database
// on the already-running instance and opens it. The caller must drop only this
// name afterwards. The shared postgres database is never dropped.
func CreateOfficialThrowawayPostgresDatabase(admin *gorm.DB, adminDSN, name string) (*gorm.DB, error) {
	if !OfficialThrowawayPostgresName(name) || ReservedPostgresDatabase(name) {
		return nil, fmt.Errorf("refusing to create unexpected database %q", name)
	}
	var adminDB string
	if err := admin.Raw("SELECT current_database()").Scan(&adminDB).Error; err != nil {
		return nil, err
	}
	if strings.EqualFold(adminDB, name) {
		return nil, fmt.Errorf("admin DSN must not already point at throwaway database %q", name)
	}
	isolatedDSN, err := IsolateLocalPostgresDSN(adminDSN, name)
	if err != nil {
		return nil, err
	}
	adminSQL, err := admin.DB()
	if err != nil {
		return nil, err
	}
	if _, err := adminSQL.Exec("CREATE DATABASE " + name); err != nil {
		return nil, err
	}
	db, err := OpenOfficialPostgresAcceptance(isolatedDSN)
	if err != nil {
		_ = DropOfficialThrowawayPostgresDatabase(admin, name)
		return nil, err
	}
	return db, nil
}

// DropOfficialThrowawayPostgresDatabase drops only a validated throwaway
// database. It refuses to drop postgres, template0, template1, or the
// currently connected database.
func DropOfficialThrowawayPostgresDatabase(admin *gorm.DB, name string) error {
	if !OfficialThrowawayPostgresName(name) || ReservedPostgresDatabase(name) {
		return fmt.Errorf("refusing to drop unexpected database %q", name)
	}
	var current string
	if err := admin.Raw("SELECT current_database()").Scan(&current).Error; err != nil {
		return err
	}
	if strings.EqualFold(current, name) {
		return fmt.Errorf("refusing to drop the connected database %q", name)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		return err
	}
	if _, err := adminSQL.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, name); err != nil {
		return err
	}
	_, err = adminSQL.Exec("DROP DATABASE " + name)
	return err
}

// SwapOfficialPackageDatabase points package DB/LOG_DB at db and records the
// database types for quoted identifiers. The returned restore function puts
// the previous package database back and reinitializes column quoting.
func SwapOfficialPackageDatabase(db *gorm.DB, mainType, logType common.DatabaseType) (restore func()) {
	oldDB, oldLogDB := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(mainType, logType)
	initCol()
	return func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		initCol()
	}
}

// RunOfficialMigrateDB runs the official migrateDB path, including MigrateAdtyTables.
func RunOfficialMigrateDB() error {
	return migrateDB()
}
