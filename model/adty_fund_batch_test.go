package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConsumeFundBatchesSpendsPrincipalBeforeBonus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: 7, Kind: "bonus", Source: "b", Original: 40, Remaining: 40},
		{UserId: 7, Kind: "principal", Source: "late", Original: 30, Remaining: 30},
		{UserId: 7, Kind: "principal", Source: "early", Original: 20, Remaining: 20},
	}).Error)
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("source = ?", "early").Update("created_at", 1).Error)
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("source = ?", "late").Update("created_at", 2).Error)
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("source = ?", "b").Update("created_at", 1).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 7, 45, 0)
	}))
	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 7).Order("id").Find(&rows).Error)
	remaining := map[string]int64{}
	for _, row := range rows {
		remaining[row.Source] = row.Remaining
	}
	require.EqualValues(t, 0, remaining["early"])
	require.EqualValues(t, 5, remaining["late"])
	require.EqualValues(t, 40, remaining["b"])
}

func TestLegacyBalanceBecomesPrincipalBatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 8, 40, 100)
	}))
	var row AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 8).First(&row).Error)
	require.Equal(t, "principal", row.Kind)
	require.Equal(t, "legacy", row.Source)
	require.EqualValues(t, 60, row.Remaining)
}

func TestReclaimFundBatchesBySourceCapsEachKind(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: 9, Kind: "principal", Source: "order-a", Original: 80, Remaining: 80},
		{UserId: 9, Kind: "bonus", Source: "order-a", Original: 20, Remaining: 20},
		{UserId: 9, Kind: "principal", Source: "order-b", Original: 50, Remaining: 50},
	}).Error)
	var taken int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		taken, err = ReclaimAdtyFundBatchesBySourceTx(tx, 9, "order-a", 100, 5)
		return err
	}))
	require.EqualValues(t, 85, taken)
	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 9).Order("id").Find(&rows).Error)
	got := map[string]int64{}
	for _, row := range rows {
		got[row.Source+":"+row.Kind] = row.Remaining
	}
	require.EqualValues(t, 0, got["order-a:principal"])
	require.EqualValues(t, 15, got["order-a:bonus"])
	require.EqualValues(t, 50, got["order-b:principal"])
}
