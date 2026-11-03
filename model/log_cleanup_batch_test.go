package model

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

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
