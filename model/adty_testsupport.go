package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/QuantumNous/new-api/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ErrAdtyTestDSNUnset 表示当前进程没有测试库连接串。调用方应跳过，检查脚本不能靠这个跳过。
var ErrAdtyTestDSNUnset = errors.New("ADTY_TEST_PG_DSN is unset")

// InitColumnNamesForTests sets the quoted key/group column names for the
// database type already selected by the test. Service tests that read tokens
// by key need this because their sqlite TestMain does not open the main DB.
func InitColumnNamesForTests() { initCol() }

var adtyTestSchemaPattern = regexp.MustCompile(`^adty_t_[0-9a-f]{16}$`)

// AdtyTestDatabase 是一个只给测试用的独立 schema。Close 会换回原来的连接并删掉 schema。
type AdtyTestDatabase struct {
	DB         *gorm.DB
	Schema     string
	admin      *gorm.DB
	oldDB      *gorm.DB
	oldLogDB   *gorm.DB
	oldMain    common.DatabaseType
	oldLogType common.DatabaseType
	closed     bool
}

// OpenAdtyTestDatabase 打开测试库，把搜索路径固定到新建 schema，并换上包级 DB、LOG_DB。
func OpenAdtyTestDatabase() (*AdtyTestDatabase, error) {
	dsn := os.Getenv("ADTY_TEST_PG_DSN")
	if dsn == "" {
		return nil, ErrAdtyTestDSNUnset
	}
	schema, err := adtyRandomSchema()
	if err != nil {
		return nil, err
	}
	quoted, err := adtyQuoteSchema(schema)
	if err != nil {
		return nil, err
	}
	admin, err := openAdtyPostgres(dsn, "")
	if err != nil {
		return nil, err
	}
	if err := admin.Exec("CREATE SCHEMA " + quoted).Error; err != nil {
		closeAdtyGorm(admin)
		return nil, err
	}
	db, err := openAdtyPostgres(dsn, schema)
	if err != nil {
		_ = admin.Exec("DROP SCHEMA IF EXISTS " + quoted + " CASCADE").Error
		closeAdtyGorm(admin)
		return nil, err
	}
	opened := &AdtyTestDatabase{
		DB:         db,
		Schema:     schema,
		admin:      admin,
		oldDB:      DB,
		oldLogDB:   LOG_DB,
		oldMain:    common.MainDatabaseType(),
		oldLogType: common.LogDatabaseType(),
	}
	DB = db
	LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	initCol()
	var got string
	if err := db.Raw("SELECT current_schema()").Scan(&got).Error; err != nil {
		_ = opened.Close()
		return nil, err
	}
	if got != schema {
		_ = opened.Close()
		return nil, fmt.Errorf("search_path did not apply: got %q want %q", got, schema)
	}
	return opened, nil
}

// Close 先关掉测试连接，再恢复全局库类型，最后删掉 schema。
func (opened *AdtyTestDatabase) Close() error {
	if opened == nil || opened.closed {
		return nil
	}
	opened.closed = true
	var first error
	remember := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	if opened.DB != nil {
		remember(closeAdtyGorm(opened.DB))
	}
	DB = opened.oldDB
	LOG_DB = opened.oldLogDB
	common.SetDatabaseTypes(opened.oldMain, opened.oldLogType)
	initCol()
	if opened.admin != nil && opened.Schema != "" {
		quoted, err := adtyQuoteSchema(opened.Schema)
		if err != nil {
			remember(err)
		} else {
			remember(opened.admin.Exec("DROP SCHEMA IF EXISTS " + quoted + " CASCADE").Error)
		}
		remember(closeAdtyGorm(opened.admin))
	}
	return first
}

func openAdtyPostgres(dsn, searchPath string) (*gorm.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	if searchPath != "" {
		if _, exists := cfg.RuntimeParams["options"]; exists {
			return nil, errors.New("ADTY_TEST_PG_DSN already sets options")
		}
		cfg.RuntimeParams["options"] = "-c search_path=" + searchPath
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	sqlDB := stdlib.OpenDB(*cfg)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	return gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
}

func adtyRandomSchema() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "adty_t_" + hex.EncodeToString(buf), nil
}

func adtyQuoteSchema(schema string) (string, error) {
	if !adtyTestSchemaPattern.MatchString(schema) {
		return "", fmt.Errorf("invalid adty test schema %q", schema)
	}
	return `"` + schema + `"`, nil
}

func closeAdtyGorm(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
