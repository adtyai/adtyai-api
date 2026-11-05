package model

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeleteOldLogBatchKeepsPaymentAndAdjustRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB, previousLogDB := DB, LOG_DB
	DB, LOG_DB = db, db
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Log{}))
	for _, logType := range []int{LogTypeTopup, LogTypeRefund, LogTypeManage, LogTypeConsume} {
		require.NoError(t, db.Create(&Log{UserId: 1, CreatedAt: 10, Type: logType, Content: "old"}).Error)
	}

	deleted, err := DeleteOldLogBatch(context.Background(), 9_999_999_999, 100)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var kept []Log
	require.NoError(t, db.Order("type").Find(&kept).Error)
	require.Len(t, kept, 3)
	assert.Equal(t, []int{LogTypeTopup, LogTypeManage, LogTypeRefund}, []int{kept[0].Type, kept[1].Type, kept[2].Type})
}

func TestDeleteOldLogBatchDeletesOnlyTheRequestedPostgreSQLBatch(t *testing.T) {
	opened, err := OpenAdtyTestDatabase()
	if errors.Is(err, ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	require.NoError(t, opened.DB.AutoMigrate(&Log{}))

	for i := 0; i < 5; i++ {
		require.NoError(t, LOG_DB.Create(&Log{UserId: 1, CreatedAt: 10, Type: LogTypeConsume, Content: "old"}).Error)
	}
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, CreatedAt: 50, Type: LogTypeConsume, Content: "new"}).Error)

	deleted, err := DeleteOldLogBatch(context.Background(), 20, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)

	var remaining int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("created_at < ?", 20).Count(&remaining).Error)
	require.Equal(t, int64(3), remaining)
	require.NoError(t, LOG_DB.Model(&Log{}).Where("created_at >= ?", 20).Count(&remaining).Error)
	require.Equal(t, int64(1), remaining)
}
