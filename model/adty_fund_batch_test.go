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

func remainingBySource(t *testing.T, db *gorm.DB, userID int) map[string]int64 {
	t.Helper()
	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", userID).Order("id").Find(&rows).Error)
	got := map[string]int64{}
	for _, row := range rows {
		got[row.Source+":"+row.Kind] = row.Remaining
	}
	return got
}

func TestConsumeFundBatchesFullyCoveredDecreasesRemaining(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: 16, Kind: "principal", Source: "p", Original: 30, Remaining: 30},
		{UserId: 16, Kind: "bonus", Source: "b", Original: 20, Remaining: 20},
	}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 16, 35, 50)
	}))
	got := remainingBySource(t, db, 16)
	require.EqualValues(t, 0, got["p:principal"])
	require.EqualValues(t, 15, got["b:bonus"])
}

func TestConsumeFundBatchesShortfallDoesNotDropRemainder(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Create(&User{Id: 17, Username: "short-17", Password: "password1", Quota: 50}).Error)
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: 17, Kind: "principal", Source: "thin", Original: 20, Remaining: 20,
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", 17).Update("quota", 0).Error; err != nil {
			return err
		}
		return ConsumeAdtyFundBatchesTx(tx, 17, 50, 50)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall)
	require.Contains(t, err.Error(), "uncovered 30")

	var user User
	require.NoError(t, db.First(&user, 17).Error)
	require.EqualValues(t, 50, user.Quota)
	got := remainingBySource(t, db, 17)
	require.EqualValues(t, 20, got["thin:principal"])
}

func TestConsumeFundBatchesShortfallWithNoLotsErrors(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 18, 40, 0)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall)
	var count int64
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("user_id = ?", 18).Count(&count).Error)
	require.Zero(t, count)
}

func TestConsumeFundBatchesLegacyLotStillShortErrors(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 19, 40, 25)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall)
	var count int64
	require.NoError(t, db.Model(&AdtyFundBatch{}).Where("user_id = ?", 19).Count(&count).Error)
	require.Zero(t, count)
}

func TestConsumeFundBatchesQuietTxShortfallErrors(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: 20, Kind: "principal", Source: "q", Original: 10, Remaining: 10,
	}).Error)
	err := db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesQuietTx(tx, 20, 15, 10)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall)
	require.EqualValues(t, 10, remainingBySource(t, db, 20)["q:principal"])
}

func TestApplyWalletDebitToBatchesShortfallAfterReclaimErrors(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: 21, Kind: "principal", Source: "order-a", Original: 50, Remaining: 50},
		{UserId: 21, Kind: "bonus", Source: "order-a", Original: 10, Remaining: 10},
		{UserId: 21, Kind: "principal", Source: "order-b", Original: 20, Remaining: 20},
	}).Error)
	err := db.Transaction(func(tx *gorm.DB) error {
		return applyAdtyWalletDebitToBatchesTx(tx, 21, "order-a", 50, 10, 100, 80)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall)
	got := remainingBySource(t, db, 21)
	require.EqualValues(t, 50, got["order-a:principal"])
	require.EqualValues(t, 10, got["order-a:bonus"])
	require.EqualValues(t, 20, got["order-b:principal"])
}

func TestApplyWalletDebitToBatchesReclaimThenFIFOCovers(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: 23, Kind: "principal", Source: "order-a", Original: 50, Remaining: 50},
		{UserId: 23, Kind: "bonus", Source: "order-a", Original: 10, Remaining: 10},
		{UserId: 23, Kind: "principal", Source: "order-b", Original: 20, Remaining: 20},
	}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return applyAdtyWalletDebitToBatchesTx(tx, 23, "order-a", 50, 10, 70, 80)
	}))
	got := remainingBySource(t, db, 23)
	require.EqualValues(t, 0, got["order-a:principal"])
	require.EqualValues(t, 0, got["order-a:bonus"])
	require.EqualValues(t, 10, got["order-b:principal"])
}

func openMemoryDBWithoutFundBatchTable(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.False(t, db.Migrator().HasTable(&AdtyFundBatch{}))
	return db
}

func TestCreateAdtyFundBatchesTxMissingTableErrors(t *testing.T) {
	db := openMemoryDBWithoutFundBatchTable(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, 41, "order-missing", 80, 20)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchTableMissing)
}

func TestConsumeFundBatchesMissingTableErrors(t *testing.T) {
	db := openMemoryDBWithoutFundBatchTable(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 42, 40, 100)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchTableMissing)
}

func TestConsumeFundBatchesQuietTxMissingTableErrors(t *testing.T) {
	db := openMemoryDBWithoutFundBatchTable(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesQuietTx(tx, 43, 15, 50)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchTableMissing)
}

func TestReclaimFundBatchesMissingTableErrors(t *testing.T) {
	db := openMemoryDBWithoutFundBatchTable(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := ReclaimAdtyFundBatchesBySourceTx(tx, 44, "order-a", 10, 5)
		return err
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchTableMissing)
}

func TestApplyWalletDebitToBatchesMissingTableErrors(t *testing.T) {
	db := openMemoryDBWithoutFundBatchTable(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return applyAdtyWalletDebitToBatchesTx(tx, 45, "order-a", 50, 10, 70, 80)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchTableMissing)
}

func TestConsumeFundBatchesShortfallDoesNotFireHook(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	require.NoError(t, db.Create(&AdtyFundBatch{
		UserId: 46, Kind: "principal", Source: "thin", Original: 20, Remaining: 20,
	}).Error)
	fired := 0
	prev := AdtyAfterPrincipalSpentTx
	AdtyAfterPrincipalSpentTx = func(tx *gorm.DB, userID int, principalQuota int64) error {
		fired++
		return nil
	}
	t.Cleanup(func() { AdtyAfterPrincipalSpentTx = prev })

	err := db.Transaction(func(tx *gorm.DB) error {
		return ConsumeAdtyFundBatchesTx(tx, 46, 50, 50)
	})
	require.ErrorIs(t, err, ErrAdtyFundBatchShortfall)
	require.Zero(t, fired)
	require.EqualValues(t, 20, remainingBySource(t, db, 46)["thin:principal"])
}

func TestSumFundBatchRemainingByKind(t *testing.T) {
	db := openMemoryFundBatchDB(t)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: 11, Kind: "principal", Source: "a", Original: 80, Remaining: 50},
		{UserId: 11, Kind: "principal", Source: "b", Original: 20, Remaining: 20},
		{UserId: 11, Kind: "bonus", Source: "a", Original: 40, Remaining: 15},
		{UserId: 12, Kind: "bonus", Source: "c", Original: 9, Remaining: 9},
	}).Error)
	sum, err := SumAdtyFundBatchRemainingByKind(11)
	require.NoError(t, err)
	require.True(t, sum.HasLots)
	require.EqualValues(t, 70, sum.Principal)
	require.EqualValues(t, 15, sum.Bonus)
	other, err := SumAdtyFundBatchRemainingByKind(12)
	require.NoError(t, err)
	require.True(t, other.HasLots)
	require.EqualValues(t, 0, other.Principal)
	require.EqualValues(t, 9, other.Bonus)
	empty, err := SumAdtyFundBatchRemainingByKind(13)
	require.NoError(t, err)
	require.False(t, empty.HasLots)
	require.EqualValues(t, 0, empty.Principal)
	require.EqualValues(t, 0, empty.Bonus)
}
