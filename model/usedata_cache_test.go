package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSaveQuotaDataCacheKeepsRowsWhenWriteFails(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&QuotaData{}))
	resetQuotaDataCache(t)

	failWrites := true
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("f083-fail-create", func(tx *gorm.DB) {
		if failWrites && tx.Statement.Table == "quota_data" {
			_ = tx.AddError(gorm.ErrInvalidTransaction)
		}
	}))
	t.Cleanup(func() {
		db.Callback().Create().Remove("f083-fail-create")
	})

	LogQuotaData(QuotaDataLogParams{
		UserID: 41, Username: "f083-keep", ModelName: "gpt", CreatedAt: 7200,
		Quota: 15, TokenUsed: 3, UseGroup: "default", NodeName: "node-a",
	})
	SaveQuotaDataCache()
	require.NotEmpty(t, CacheQuotaData)
	var rows int64
	require.NoError(t, db.Model(&QuotaData{}).Where("username = ?", "f083-keep").Count(&rows).Error)
	require.Zero(t, rows)

	failWrites = false
	SaveQuotaDataCache()
	require.Empty(t, CacheQuotaData)
	var stored QuotaData
	require.NoError(t, db.Where("username = ?", "f083-keep").First(&stored).Error)
	require.Equal(t, 15, stored.Quota)
	require.Equal(t, 1, stored.Count)
	require.Equal(t, 3, stored.TokenUsed)
}

func TestSaveQuotaDataCacheDoesNotMultiplyDuplicateRows(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&QuotaData{}))
	resetQuotaDataCache(t)
	dropQuotaDataUniqueIndexes(t, db)

	bucket := QuotaData{
		UserID: 42, Username: "f083-dup", ModelName: "gpt", CreatedAt: 7200,
		UseGroup: "default", NodeName: "node-b",
	}
	first := bucket
	first.Quota = 100
	first.Count = 1
	second := bucket
	second.Quota = 40
	second.Count = 2
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)

	LogQuotaData(QuotaDataLogParams{
		UserID: bucket.UserID, Username: bucket.Username, ModelName: bucket.ModelName, CreatedAt: bucket.CreatedAt,
		Quota: 10, TokenUsed: 4, UseGroup: bucket.UseGroup, NodeName: bucket.NodeName,
	})
	SaveQuotaDataCache()

	var sum int64
	require.NoError(t, db.Model(&QuotaData{}).Where("username = ?", bucket.Username).Select("COALESCE(SUM(quota), 0)").Scan(&sum).Error)
	require.Equal(t, int64(150), sum)

	require.NoError(t, mergeQuotaDataDuplicates(db))
	var left []QuotaData
	require.NoError(t, db.Where("username = ?", bucket.Username).Find(&left).Error)
	require.Len(t, left, 1)
	require.Equal(t, 150, left[0].Quota)
	require.Equal(t, 4, left[0].Count)
	require.Equal(t, 4, left[0].TokenUsed)
}

func resetQuotaDataCache(t *testing.T) {
	t.Helper()
	CacheQuotaDataLock.Lock()
	CacheQuotaData = map[string]*QuotaData{}
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		CacheQuotaDataLock.Lock()
		CacheQuotaData = map[string]*QuotaData{}
		CacheQuotaDataLock.Unlock()
	})
}

func dropQuotaDataUniqueIndexes(t *testing.T, db *gorm.DB) {
	t.Helper()
	var names []string
	require.NoError(t, db.Raw(`
SELECT i.relname
FROM pg_index x
JOIN pg_class i ON i.oid = x.indexrelid
JOIN pg_class t ON t.oid = x.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = current_schema() AND t.relname = 'quota_data' AND x.indisunique AND NOT x.indisprimary`).Scan(&names).Error)
	for _, name := range names {
		require.NoError(t, db.Exec(`DROP INDEX IF EXISTS `+quoteIdent(name)).Error)
	}
}

func quoteIdent(name string) string {
	return `"` + name + `"`
}
