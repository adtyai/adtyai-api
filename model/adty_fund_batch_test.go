package model

import (
	"errors"
	"sync"
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

func openMemoryFundBatchDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))
	return db
}

func requireFundLots(t *testing.T, db *gorm.DB, userID int, source string, principal, bonus int64, lotCount int) {
	t.Helper()
	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND source = ?", userID, source).Order("kind").Find(&rows).Error)
	require.Len(t, rows, lotCount)
	got := map[string]int64{}
	for _, row := range rows {
		got[row.Kind] += row.Remaining
	}
	require.EqualValues(t, principal, got["principal"])
	require.EqualValues(t, bonus, got["bonus"])
}

func TestCreateAdtyFundBatchesTxReplayDoesNotDuplicate(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, 11, "order-1", 80, 20)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, 11, "order-1", 80, 20)
	}))
	requireFundLots(t, db, 11, "order-1", 80, 20, 2)
}

func TestCreateAdtyFundBatchesTxAllowsPrincipalAndBonusForSameSource(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, 12, "order-2", 70, 15)
	}))
	requireFundLots(t, db, 12, "order-2", 70, 15, 2)
}

func TestCreateAdtyFundBatchesTxDifferentSourceStillInserts(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := CreateAdtyFundBatchesTx(tx, 13, "order-a", 50, 10); err != nil {
			return err
		}
		return CreateAdtyFundBatchesTx(tx, 13, "order-b", 30, 0)
	}))
	requireFundLots(t, db, 13, "order-a", 50, 10, 2)
	requireFundLots(t, db, 13, "order-b", 30, 0, 1)
}

func TestAdtyFundBatchUniqueOnUserSourceKind(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Create(&AdtyFundBatch{UserId: 14, Kind: "principal", Source: "order-u", Original: 10, Remaining: 10}).Error)
	err := db.Create(&AdtyFundBatch{UserId: 14, Kind: "principal", Source: "order-u", Original: 10, Remaining: 10}).Error
	require.Error(t, err)
	require.True(t, isAdtyUniqueViolation(err), err)
	require.NoError(t, db.Create(&AdtyFundBatch{UserId: 14, Kind: "bonus", Source: "order-u", Original: 4, Remaining: 4}).Error)
	require.NoError(t, db.Create(&AdtyFundBatch{UserId: 15, Kind: "principal", Source: "order-u", Original: 10, Remaining: 10}).Error)
}

func TestCreateAdtyFundBatchesTxConcurrentReplayIsIdempotent(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))

	const userID = 22
	const source = "concurrent-source"
	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = db.Transaction(func(tx *gorm.DB) error {
				if err := CreateAdtyFundBatchesTx(tx, userID, source, 100, 25); err != nil {
					return err
				}
				// The caller's transaction must still be usable after a unique conflict.
				var count int64
				if err := tx.Model(&AdtyFundBatch{}).Where("user_id = ? AND source = ?", userID, source).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return errors.New("expected lots after idempotent create")
				}
				return nil
			})
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "worker %d", i)
	}
	requireFundLots(t, db, userID, source, 100, 25, 2)
}
