package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// errAdtyPostgresAcceptanceDSNUnset 表示官方测试 DSN 未设置。
// 验收必须跳过并标明 BLK-ENV-PG，不能记为通过。
var errAdtyPostgresAcceptanceDSNUnset = errors.New("BLK-ENV-PG: ADTY_TEST_PG_DSN / TEST_POSTGRES_DSN is unset")

var adtyThrowawayPostgresNamePattern = regexp.MustCompile(`^adty_m2302_[0-9a-f]{16}$`)

// adtyOfficialPostgresAcceptanceDSN 只接受本方案的官方 PostgreSQL 测试连接串。
// SQLite / MySQL 连接串不能关闭 ADTY-23-02。
func adtyOfficialPostgresAcceptanceDSN() (string, error) {
	dsn := strings.TrimSpace(os.Getenv("ADTY_TEST_PG_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	}
	if dsn == "" {
		return "", errAdtyPostgresAcceptanceDSNUnset
	}
	if !IsPostgresDSN(dsn) {
		return "", errors.New("BLK-ENV-PG: official test DSN is not PostgreSQL; SQLite/MySQL cannot close ADTY-23-02")
	}
	return dsn, nil
}

func adtyReservedPostgresDatabase(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "postgres", "template0", "template1":
		return true
	default:
		return false
	}
}

func openAdtyAcceptancePostgres(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgresMigrationDialector{postgres.Dialector{Config: &postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}}}, newGormConfig(false))
}

func requireAdtyOfficialPostgresAcceptanceDSN(t *testing.T) string {
	t.Helper()
	dsn, err := adtyOfficialPostgresAcceptanceDSN()
	if errors.Is(err, errAdtyPostgresAcceptanceDSNUnset) {
		t.Skip("BLK-ENV-PG: ADTY_TEST_PG_DSN / TEST_POSTGRES_DSN is unset; this acceptance is not recorded as passed")
	}
	require.NoError(t, err)
	return dsn
}

func connectAdtyPostgresAcceptanceInstance(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	admin, err := openAdtyAcceptancePostgres(dsn)
	if err != nil {
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	sqlDB, err := admin.DB()
	if err != nil {
		_ = closeAdtyGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		_ = closeAdtyGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	var current string
	if err := admin.Raw("SELECT current_database()").Scan(&current).Error; err != nil {
		_ = closeAdtyGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	require.NotEmpty(t, current)
	require.False(t, adtyThrowawayPostgresNamePattern.MatchString(current), "admin DSN must not already point at a throwaway database")

	var versionNum, version string
	require.NoError(t, admin.Raw("SHOW server_version_num").Scan(&versionNum).Error)
	require.NoError(t, admin.Raw("SHOW server_version").Scan(&version).Error)
	require.NotEmpty(t, versionNum, "live server_version_num must be recorded as evidence")
	t.Logf("ADTY-23-02 evidence only (not a pass/fail condition): server_version_num=%s server_version=%s driver=%s", versionNum, version, admin.Dialector.Name())
	return admin
}

func newAdtyThrowawayPostgresName() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	name := "adty_m2302_" + hex.EncodeToString(buf)
	if !adtyThrowawayPostgresNamePattern.MatchString(name) || adtyReservedPostgresDatabase(name) {
		return "", fmt.Errorf("invalid throwaway database name %q", name)
	}
	return name, nil
}

// openAdtyThrowawayMigrateDatabase 在已运行实例上建一个用完即删的空库，
// 换上包级 DB 后走官方 migrateDB。绝不 DROP 共享的 postgres 库，也不 initdb。
func openAdtyThrowawayMigrateDatabase(t *testing.T, admin *gorm.DB, adminDSN string) *gorm.DB {
	t.Helper()
	name, err := newAdtyThrowawayPostgresName()
	require.NoError(t, err)
	require.False(t, adtyReservedPostgresDatabase(name))

	var adminDB string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&adminDB).Error)
	require.NotEqual(t, strings.ToLower(adminDB), strings.ToLower(name))
	require.False(t, adtyReservedPostgresDatabase(name))

	isolatedDSN, err := IsolateLocalPostgresDSN(adminDSN, name)
	require.NoError(t, err)

	adminSQL, err := admin.DB()
	require.NoError(t, err)
	_, err = adminSQL.Exec("CREATE DATABASE " + name)
	require.NoError(t, err, "create throwaway database %s", name)

	db, err := openAdtyAcceptancePostgres(isolatedDSN)
	if err != nil {
		_ = dropAdtyThrowawayPostgresDatabase(admin, name)
		require.NoError(t, err, "open throwaway database %s", name)
	}

	oldDB, oldLogDB := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	initCol()

	dropped := false
	drop := func() {
		if dropped {
			return
		}
		dropped = true
		_ = closeAdtyGorm(db)
		DB, LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		initCol()
		if dropErr := dropAdtyThrowawayPostgresDatabase(admin, name); dropErr != nil {
			t.Errorf("drop throwaway database %s: %v", name, dropErr)
		}
	}
	t.Cleanup(drop)
	t.Logf("throwaway database: %s", name)
	return db
}

func dropAdtyThrowawayPostgresDatabase(admin *gorm.DB, name string) error {
	if !adtyThrowawayPostgresNamePattern.MatchString(name) || adtyReservedPostgresDatabase(name) {
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

func assertOfficialAdtyMigrateIndexes(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.True(t, db.Migrator().HasTable(&AdtyVoucher{}))
	require.True(t, db.Migrator().HasTable(&AdtyTeamMember{}))
	require.True(t, db.Migrator().HasTable(&AdtySitePrice{}))
	require.True(t, db.Migrator().HasTable(&AdtySiteEarningDaily{}))

	require.True(t, db.Migrator().HasIndex(AdtyVoucher{}.TableName(), adtyVoucherTradeNoUniqueIndex))
	current, err := adtyVoucherTradeNoUniqueIndexIsCurrent(db)
	require.NoError(t, err)
	require.True(t, current, "empty-db migrate must install ux_adty_vouchers_trade_no")

	require.True(t, db.Migrator().HasIndex(AdtyTeamMember{}.TableName(), activeTeamMembershipUniqueIndex))
	current, err = activeTeamMembershipUniqueIndexIsCurrent(db)
	require.NoError(t, err)
	require.True(t, current, "empty-db migrate must install ux_adty_team_members_one_active_user")

	require.True(t, db.Migrator().HasIndex(&AdtySitePrice{}, adtySitePriceUniqueIndex))
	current, err = adtySitePriceUniqueIndexIsCurrent(db)
	require.NoError(t, err)
	require.True(t, current, "empty-db migrate must install idx_site_price_model")
	require.False(t, db.Migrator().HasIndex(&AdtySitePrice{}, adtySitePriceLegacyIndex))

	require.True(t, db.Migrator().HasIndex(&AdtySiteEarningDaily{}, "idx_site_earning_owner"))
	require.False(t, db.Migrator().HasIndex(&AdtySiteEarningDaily{}, "idx_site_earning"))

	for _, entity := range []any{&AdtySiteEarningDaily{}, &AdtySiteEarningEvent{}, &BillingOperation{}, &AdtyTopupOrder{}} {
		require.True(t, db.Migrator().HasColumn(entity, "site_owner_user_id"), "financial ownership column must exist after official migrate")
	}
	require.True(t, db.Migrator().HasColumn(&AdtySite{}, "ownership_version"))
}

type adtyMigrateSchemaSnapshot struct {
	tables  []string
	indexes []string
}

func snapshotAdtyMigrateSchema(t *testing.T, db *gorm.DB) adtyMigrateSchemaSnapshot {
	t.Helper()
	var tables []string
	require.NoError(t, db.Raw("SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY 1").Scan(&tables).Error)
	var indexes []string
	require.NoError(t, db.Raw("SELECT indexname || E'\\n' || indexdef FROM pg_indexes WHERE schemaname = current_schema() ORDER BY indexname, indexdef").Scan(&indexes).Error)
	require.NotEmpty(t, tables)
	require.NotEmpty(t, indexes)
	return adtyMigrateSchemaSnapshot{tables: tables, indexes: indexes}
}

func assertAdtyMigrateSchemaUnchanged(t *testing.T, db *gorm.DB, before adtyMigrateSchemaSnapshot) {
	t.Helper()
	after := snapshotAdtyMigrateSchema(t, db)
	assert.Equal(t, before.tables, after.tables, "idempotent remigrate must not add or drop tables")
	assert.Equal(t, before.indexes, after.indexes, "idempotent remigrate must not rewrite indexes")
}

func installSiteOwnershipLegacyFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&siteOwnershipLegacyDaily{}))
	require.NoError(t, db.Create(&siteOwnershipLegacyDaily{
		SiteId:    11,
		UserId:    22,
		Day:       "2026-11-01",
		Kind:      "usage",
		Quota:     500000,
		Revenue:   decimal.NewFromInt(1),
		Platform:  decimal.RequireFromString("0.4"),
		Earning:   decimal.RequireFromString("0.6"),
		Status:    "available",
		SettledAt: 1234,
	}).Error)
	require.True(t, db.Migrator().HasIndex(&siteOwnershipLegacyDaily{}, "idx_site_earning"))
}

func assertSiteOwnershipLegacyHistoryPreserved(t *testing.T, db *gorm.DB) {
	t.Helper()
	var history AdtySiteEarningDaily
	require.NoError(t, db.Where("id = ?", 1).First(&history).Error)
	assert.Equal(t, "0.6", history.Earning.String())
	assert.Equal(t, "1", history.Revenue.String())
	assert.Equal(t, "0.4", history.Platform.String())
	assert.EqualValues(t, 500000, history.Quota)
	assert.EqualValues(t, 1234, history.SettledAt)
	assert.Equal(t, "available", history.Status)
	assert.Equal(t, 11, history.SiteId)
	assert.Equal(t, 22, history.UserId)
	assert.Equal(t, "2026-11-01", history.Day)
	assert.Equal(t, "usage", history.Kind)
}

func TestAdtyOfficialPostgresAcceptanceDSNRejectsNonPostgres(t *testing.T) {
	t.Setenv("ADTY_TEST_PG_DSN", "root:pass@tcp(127.0.0.1:3306)/new-api")
	t.Setenv("TEST_POSTGRES_DSN", "")
	_, err := adtyOfficialPostgresAcceptanceDSN()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not PostgreSQL")

	t.Setenv("ADTY_TEST_PG_DSN", "")
	t.Setenv("TEST_POSTGRES_DSN", "")
	_, err = adtyOfficialPostgresAcceptanceDSN()
	require.ErrorIs(t, err, errAdtyPostgresAcceptanceDSNUnset)

	t.Setenv("TEST_POSTGRES_DSN", "host=127.0.0.1 port=5432 user=box dbname=postgres sslmode=disable")
	dsn, err := adtyOfficialPostgresAcceptanceDSN()
	require.NoError(t, err)
	assert.True(t, IsPostgresDSN(dsn))

	t.Setenv("ADTY_TEST_PG_DSN", "postgresql://box@127.0.0.1:55432/adty?sslmode=disable")
	preferred, err := adtyOfficialPostgresAcceptanceDSN()
	require.NoError(t, err)
	assert.Equal(t, "postgresql://box@127.0.0.1:55432/adty?sslmode=disable", preferred)
}

func TestAdtyPostgreSQLConnectInitAndMigrateAcceptance(t *testing.T) {
	dsn := requireAdtyOfficialPostgresAcceptanceDSN(t)
	admin := connectAdtyPostgresAcceptanceInstance(t, dsn)
	t.Cleanup(func() {
		require.NoError(t, closeAdtyGorm(admin))
	})

	t.Run("empty_database_init", func(t *testing.T) {
		db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
		require.NoError(t, migrateDB())
		assertOfficialAdtyMigrateIndexes(t, db)
		for _, ownerID := range []int{31, 32} {
			require.NoError(t, db.Model(&AdtySiteEarningDaily{}).Create(map[string]any{
				"site_id": 11, "site_owner_user_id": ownerID, "user_id": 22,
				"day": "2026-11-01", "kind": "usage", "quota": 100, "revenue": 1, "platform": 0.4, "earning": 0.6, "status": "pending",
			}).Error)
		}
		assert.Error(t, db.Model(&AdtySiteEarningDaily{}).Create(map[string]any{
			"site_id": 11, "site_owner_user_id": 31, "user_id": 22,
			"day": "2026-11-01", "kind": "usage",
		}).Error, "the same owner's daily slot must remain unique")
		before := snapshotAdtyMigrateSchema(t, db)
		require.NoError(t, migrateDB(), "official migrate must be idempotent on an already-initialized database")
		assertOfficialAdtyMigrateIndexes(t, db)
		assertAdtyMigrateSchemaUnchanged(t, db, before)
	})

	t.Run("representative_upgrade", func(t *testing.T) {
		db := openAdtyThrowawayMigrateDatabase(t, admin, dsn)
		installSiteOwnershipLegacyFixture(t, db)
		require.NoError(t, migrateDB())
		assertSiteOwnershipLegacyHistoryPreserved(t, db)
		assertOfficialAdtyMigrateIndexes(t, db)
		before := snapshotAdtyMigrateSchema(t, db)
		require.NoError(t, migrateDB(), "official migrate must be idempotent after a representative upgrade")
		assertSiteOwnershipLegacyHistoryPreserved(t, db)
		assertOfficialAdtyMigrateIndexes(t, db)
		assertAdtyMigrateSchemaUnchanged(t, db, before)
	})
}
