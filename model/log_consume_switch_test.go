package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConsumeLogSwitchStillKeepsQuotaEvidence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB, previousLogDB := DB, LOG_DB
	previousLog, previousExport := common.LogConsumeEnabled, common.DataExportEnabled
	DB, LOG_DB = db, db
	common.LogConsumeEnabled = false
	common.DataExportEnabled = true
	CacheQuotaDataLock.Lock()
	previousCache := CacheQuotaData
	CacheQuotaData = map[string]*QuotaData{}
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.LogConsumeEnabled, common.DataExportEnabled = previousLog, previousExport
		CacheQuotaDataLock.Lock()
		CacheQuotaData = previousCache
		CacheQuotaDataLock.Unlock()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Log{}, &QuotaData{}))

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("username", "payer")
	RecordConsumeLog(ctx, 7, RecordConsumeLogParams{
		ModelName: "m", Quota: 500000, PromptTokens: 10, CompletionTokens: 5, TokenName: "key",
	})

	var logs int64
	require.NoError(t, db.Model(&Log{}).Where("user_id = ?", 7).Count(&logs).Error)
	assert.Zero(t, logs)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	var quota int
	for _, row := range CacheQuotaData {
		if row.UserID == 7 {
			quota += row.Quota
		}
	}
	assert.Equal(t, 500000, quota)
}
