package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAdtyVoucherTradeNoIndexDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&AdtyVoucher{}, &TopUp{}, &AdtyTopupOrder{}))
	return db
}

func createAdtyVoucherFixture(t *testing.T, db *gorm.DB, hash, tail, status, tradeNo string) AdtyVoucher {
	t.Helper()
	row := AdtyVoucher{
		CodeHash:   hash,
		CodeTail:   tail,
		Purpose:    AdtyVoucherPurposeMainUser,
		Status:     status,
		AmountUSD:  decimal.RequireFromString("10.50"),
		Quota:      5_250_000,
		SourceKind: AdtyVoucherSourcePlatform,
		TradeNo:    tradeNo,
	}
	require.NoError(t, db.Create(&row).Error)
	return row
}

func TestAdtyVoucherTradeNoUniqueIndexRejectsDuplicateNonEmptyTradeNo(t *testing.T) {
	db := setupAdtyVoucherTradeNoIndexDB(t)
	require.NoError(t, EnsureAdtyVoucherTradeNoUniqueIndex(db))
	require.NoError(t, EnsureAdtyVoucherTradeNoUniqueIndex(db), "installing the index twice is a no-op")
	assert.True(t, db.Migrator().HasIndex(AdtyVoucher{}.TableName(), adtyVoucherTradeNoUniqueIndex))

	first := createAdtyVoucherFixture(t, db, "hash-a", "AAAA", AdtyVoucherStatusRedeemed, "VCHSHAREDTRADE001")
	err := db.Create(&AdtyVoucher{
		CodeHash:   "hash-b",
		CodeTail:   "BBBB",
		Purpose:    AdtyVoucherPurposeMainUser,
		Status:     AdtyVoucherStatusRedeemed,
		AmountUSD:  decimal.NewFromInt(1),
		Quota:      500000,
		SourceKind: AdtyVoucherSourcePlatform,
		TradeNo:    first.TradeNo,
	}).Error
	require.Error(t, err)
	assert.True(t, isAdtyVoucherTradeNoUniqueViolation(err) || isAdtyUniqueViolation(err), err)

	var count int64
	require.NoError(t, db.Model(&AdtyVoucher{}).Where("trade_no = ?", first.TradeNo).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAdtyVoucherTradeNoUniqueIndexAllowsEmptyPendingTradeNo(t *testing.T) {
	db := setupAdtyVoucherTradeNoIndexDB(t)
	require.NoError(t, EnsureAdtyVoucherTradeNoUniqueIndex(db))

	first := createAdtyVoucherFixture(t, db, "pending-a", "AAAA", AdtyVoucherStatusPending, "")
	second := createAdtyVoucherFixture(t, db, "pending-b", "BBBB", AdtyVoucherStatusPending, "")
	require.NotEqual(t, first.Id, second.Id)

	var empty int64
	require.NoError(t, db.Model(&AdtyVoucher{}).Where("trade_no = ? OR trade_no IS NULL", "").Count(&empty).Error)
	assert.Equal(t, int64(2), empty)

	require.NoError(t, db.Model(&AdtyVoucher{}).Where("id = ?", first.Id).Update("trade_no", "VCHPENDINGONE0001").Error)
	require.NoError(t, db.Model(&AdtyVoucher{}).Where("id = ?", second.Id).Update("trade_no", "VCHPENDINGTWO0002").Error)
}

func TestAdtyVoucherTradeNoUniqueIndexSkipsHistoricalCollisions(t *testing.T) {
	db := setupAdtyVoucherTradeNoIndexDB(t)

	first := createAdtyVoucherFixture(t, db, "collision-a", "AAAA", AdtyVoucherStatusRedeemed, "VCHHISTDUP0000001")
	second := createAdtyVoucherFixture(t, db, "collision-b", "BBBB", AdtyVoucherStatusRedeemed, "VCHHISTDUP0000001")
	pending := createAdtyVoucherFixture(t, db, "collision-c", "CCCC", AdtyVoucherStatusPending, "")

	err := EnsureAdtyVoucherTradeNoUniqueIndex(db)
	require.ErrorIs(t, err, ErrAdtyVoucherTradeNoCollision)
	assert.False(t, db.Migrator().HasIndex(AdtyVoucher{}.TableName(), adtyVoucherTradeNoUniqueIndex))

	var stored []AdtyVoucher
	require.NoError(t, db.Where("id IN ?", []int64{first.Id, second.Id, pending.Id}).Order("id ASC").Find(&stored).Error)
	require.Len(t, stored, 3)
	assert.Equal(t, first.Id, stored[0].Id)
	assert.Equal(t, first.TradeNo, stored[0].TradeNo)
	assert.Equal(t, AdtyVoucherStatusRedeemed, stored[0].Status)
	assert.Equal(t, second.Id, stored[1].Id)
	assert.Equal(t, second.TradeNo, stored[1].TradeNo)
	assert.Equal(t, AdtyVoucherStatusRedeemed, stored[1].Status)
	assert.Equal(t, pending.Id, stored[2].Id)
	assert.Empty(t, stored[2].TradeNo)
	assert.Equal(t, AdtyVoucherStatusPending, stored[2].Status)

	var colliding int64
	require.NoError(t, db.Model(&AdtyVoucher{}).Where("trade_no = ?", "VCHHISTDUP0000001").Count(&colliding).Error)
	assert.Equal(t, int64(2), colliding, "historical colliding trade numbers were not merged or deleted")

	taken, err := AdtyVoucherTradeNoTaken(db, "VCHHISTDUP0000001")
	require.NoError(t, err)
	assert.True(t, taken, "application layer must still reject a new write of the colliding trade number")
	taken, err = AdtyVoucherTradeNoTaken(db, "")
	require.NoError(t, err)
	assert.False(t, taken)
}

func TestAdtyVoucherTradeNoUniqueIndexIsInstalledByMigrateAdtyTables(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}))
	require.NoError(t, MigrateAdtyTables())
	assert.True(t, db.Migrator().HasIndex(AdtyVoucher{}.TableName(), adtyVoucherTradeNoUniqueIndex))
	current, err := adtyVoucherTradeNoUniqueIndexIsCurrent(db)
	require.NoError(t, err)
	if !current {
		var definition string
		require.NoError(t, db.Raw(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?",
			adtyVoucherTradeNoUniqueIndex,
		).Scan(&definition).Error)
		t.Fatalf("index exists but is not recognized as current: %s", definition)
	}
}
