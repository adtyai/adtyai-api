package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Subscriptions, redemption codes and upstream invite rewards were deleted in
// tasks 1.2-1.5 (docs/adty/deleted-features.md). A new database must not get
// their tables and columns back from an upstream merge. The full migration
// runs on SQLite because the deleted subscription_plans DDL only ran there.
func TestAdtyDeletedSchemaAbsent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = oldDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, migrateDB())

	require.True(t, db.Migrator().HasTable("users"))
	for _, table := range []string{
		"subscription_plans",
		"subscription_orders",
		"user_subscriptions",
		"subscription_pre_consume_records",
		"redemptions",
	} {
		assert.Falsef(t, db.Migrator().HasTable(table), "deleted table %s is created again", table)
	}
	for _, column := range []string{"aff_code", "aff_count", "aff_quota", "aff_history", "inviter_id"} {
		assert.Falsef(t, db.Migrator().HasColumn("users", column), "deleted column users.%s is created again", column)
	}
}

func TestAdtyDeletedOptionKeysAbsent(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMapRWMutex.Unlock()
	// InitOptionMap ends with loadOptionsFromDatabase, which also rebuilds the
	// request policy snapshot from the new map.
	oldRequestPolicy := requestPolicySnapshot.Load()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		requestPolicySnapshot.Store(oldRequestPolicy)
	})

	InitOptionMap()

	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	require.NotEmpty(t, common.OptionMap)
	for _, key := range []string{"QuotaForInviter", "QuotaForInvitee", "TopUpLink", "WaffoSubscriptionReturnUrl"} {
		assert.NotContainsf(t, common.OptionMap, key, "deleted option %s is published again", key)
	}
}
