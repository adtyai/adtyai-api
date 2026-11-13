package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withMemoryBillingHoldDB(t *testing.T, migrate bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	if migrate {
		require.NoError(t, db.AutoMigrate(&AdtyBillingHold{}))
	} else {
		require.False(t, db.Migrator().HasTable(&AdtyBillingHold{}))
	}
	prev := DB
	DB = db
	t.Cleanup(func() { DB = prev })
	return db
}

func TestRecordAdtyBillingHoldMissingTableErrors(t *testing.T) {
	withMemoryBillingHoldDB(t, false)
	err := RecordAdtyBillingHold(51, "hold-missing", 12)
	require.ErrorIs(t, err, ErrAdtyBillingHoldTableMissing)
}

func TestAdtyUserPaidPausedMissingTableErrors(t *testing.T) {
	withMemoryBillingHoldDB(t, false)
	paused, err := AdtyUserPaidPaused(51)
	require.ErrorIs(t, err, ErrAdtyBillingHoldTableMissing)
	require.False(t, paused)
}

func TestAdtyUserPaidPausedNilDBErrors(t *testing.T) {
	prev := DB
	DB = nil
	t.Cleanup(func() { DB = prev })
	paused, err := AdtyUserPaidPaused(51)
	require.ErrorIs(t, err, ErrAdtyBillingHoldTableMissing)
	require.False(t, paused)
}

func TestRecordAdtyBillingHoldPausesUserWhenTableExists(t *testing.T) {
	withMemoryBillingHoldDB(t, true)
	require.NoError(t, RecordAdtyBillingHold(52, "hold-1", 15))
	paused, err := AdtyUserPaidPaused(52)
	require.NoError(t, err)
	require.True(t, paused)

	paused, err = AdtyUserPaidPaused(53)
	require.NoError(t, err)
	require.False(t, paused)

	require.NoError(t, RecordAdtyBillingHold(52, "hold-1", 15))
	var count int64
	require.NoError(t, DB.Model(&AdtyBillingHold{}).Where("user_id = ?", 52).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
