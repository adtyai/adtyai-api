package model

import (
	"net"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTokenQuotaBatchDeltaPersistsOnPostgres(t *testing.T) {
	if os.Getenv("ADTY_TEST_PG") != "1" {
		t.Skip("set ADTY_TEST_PG=1 and TEST_POSTGRES_DSN to run the postgres path")
	}
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	require.NotEmpty(t, dsn)
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	ip := net.ParseIP(parsed.Hostname())
	require.NotNil(t, ip)
	require.True(t, ip.IsLoopback(), "database tests only permit loopback instances")

	name := "adty_tokenq_" + strings.ToLower(common.GetRandomString(12))
	require.True(t, strings.HasPrefix(name, "adty_tokenq_"))
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	require.NoError(t, adminSQL.Close())
	t.Cleanup(func() { dropIsolatedTokenQuotaDatabase(t, dsn, name) })

	parsed.Path = "/" + name
	isolated, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := isolated.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	origDB, origLog := DB, LOG_DB
	origMain, origLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		DB = origDB
		LOG_DB = origLog
		common.SetDatabaseTypes(origMain, origLogType)
		initCol()
	})
	DB = isolated
	LOG_DB = isolated
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	initCol()
	require.NoError(t, DB.AutoMigrate(&User{}, &UserBonusWallet{}, &Token{}, &AdtyWalletConsumeHourly{}))

	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 9)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 9, getTokenFromDB(t, token.Id).RemainQuota, "batch queue must not pretend the deduction is already durable")

	batchUpdate()
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 2, reloaded.RemainQuota)
	assert.Equal(t, 7, reloaded.UsedQuota)

	require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 3))
	batchUpdate()
	refunded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 5, refunded.RemainQuota)
	assert.Equal(t, 4, refunded.UsedQuota)
}

func dropIsolatedTokenQuotaDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()
	if !strings.HasPrefix(name, "adty_tokenq_") || name == "postgres" || strings.ContainsAny(name, "\"'; ") {
		t.Errorf("refusing to drop database %s", name)
		return
	}
	admin, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		t.Errorf("reopen admin database: %v", err)
		return
	}
	sqlDB, err := admin.DB()
	if err != nil {
		t.Errorf("admin sql database: %v", err)
		return
	}
	defer sqlDB.Close()
	if err := admin.Exec("DROP DATABASE "+name).Error; err != nil {
		t.Errorf("drop isolated database %s: %v", name, err)
	}
}
