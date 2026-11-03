package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchChannelsTagModeKeepsATagOnOnePage(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMaster, previousCache, previousRedis, previousSQLite := common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath = previousMaster, previousCache, previousRedis, previousSQLite
	})
	t.Setenv("SQL_DSN", os.Getenv("TEST_CHANNEL_SQL_DSN"))
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = true, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-search-tag.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database

	alpha := "alpha-tag"
	beta := "beta-tag"
	high := int64(10)
	low := int64(1)
	require.NoError(t, database.Create(&model.Channel{Name: "alpha-one", Type: 1, Key: "k1", Status: common.ChannelStatusEnabled, Group: "default", Models: "m", Tag: &alpha, Priority: &high}).Error)
	require.NoError(t, database.Create(&model.Channel{Name: "alpha-two", Type: 1, Key: "k2", Status: common.ChannelStatusEnabled, Group: "default", Models: "m", Tag: &alpha, Priority: &high}).Error)
	require.NoError(t, database.Create(&model.Channel{Name: "beta-one", Type: 1, Key: "k3", Status: common.ChannelStatusEnabled, Group: "default", Models: "m", Tag: &beta, Priority: &low}).Error)

	search := func(page string) (items []struct {
		Name string  `json:"name"`
		Tag  *string `json:"tag"`
	}, total int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/search?tag_mode=true&page_size=1&p="+page, nil)
		SearchChannels(c)
		require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())
		var body struct {
			Data struct {
				Items []struct {
					Name string  `json:"name"`
					Tag  *string `json:"tag"`
				} `json:"items"`
				Total int `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		return body.Data.Items, body.Data.Total
	}

	first, total := search("1")
	assert.Equal(t, 2, total)
	require.Len(t, first, 2)
	require.NotNil(t, first[0].Tag)
	require.NotNil(t, first[1].Tag)
	assert.Equal(t, alpha, *first[0].Tag)
	assert.Equal(t, alpha, *first[1].Tag)

	second, _ := search("2")
	require.Len(t, second, 1)
	require.NotNil(t, second[0].Tag)
	assert.Equal(t, beta, *second[0].Tag)
}
