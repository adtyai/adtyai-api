package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnableChannelRestoresKeysWhenNoneAreEnabled(t *testing.T) {
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
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-enable-keys.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.AdtyGroup{}, &model.User{}, &model.Log{}, &model.AuditLog{}))

	enable := func(t *testing.T, id int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
		c.Set("id", 1)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/"+strconv.Itoa(id)+"/status", bytes.NewReader([]byte(`{"status":1}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		UpdateChannelStatus(c)
		require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), `"data":true`, recorder.Body.String())
	}

	exhausted := &model.Channel{
		Name: "all-keys-disabled", Type: 1, Key: "key-one\nkey-two", Status: common.ChannelStatusAutoDisabled,
		Models: "test-model", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{0: 3, 1: 3}},
	}
	require.NoError(t, exhausted.Insert())
	_, _, keyErr := exhausted.GetNextEnabledKey()
	require.Error(t, keyErr)

	enable(t, exhausted.Id)
	loaded, err := model.GetChannelById(exhausted.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, loaded.Status)
	assert.Empty(t, loaded.ChannelInfo.MultiKeyStatusList)
	key, _, keyErr := loaded.GetNextEnabledKey()
	if keyErr != nil {
		t.Fatalf("status=%d info=%+v key=%q err=%v", loaded.Status, loaded.ChannelInfo, key, keyErr)
	}
	assert.NotEmpty(t, key)

	mixed := &model.Channel{
		Name: "one-key-still-enabled", Type: 1, Key: "keep-me\nleave-disabled", Status: common.ChannelStatusManuallyDisabled,
		Models: "test-model", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled}},
	}
	require.NoError(t, mixed.Insert())
	enable(t, mixed.Id)
	loaded, err = model.GetChannelById(mixed.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, loaded.Status)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, loaded.ChannelInfo.MultiKeyStatusList[1])
	key, _, keyErr = loaded.GetNextEnabledKey()
	if keyErr != nil {
		t.Fatalf("mixed channel key err=%v info=%+v", keyErr, loaded.ChannelInfo)
	}
	assert.Equal(t, "keep-me", key)
}

func TestEnableTagRestoresKeysWhenNoneAreEnabled(t *testing.T) {
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
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-tag-enable-keys.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.AdtyGroup{}, &model.User{}, &model.Log{}, &model.AuditLog{}))

	tag := "restore-tag"
	exhausted := &model.Channel{
		Name: "tag-all-keys-disabled", Type: 1, Key: "tag-key-one\ntag-key-two", Status: common.ChannelStatusAutoDisabled,
		Models: "test-model", Group: "default", Tag: &tag,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{0: 3, 1: 3}},
	}
	require.NoError(t, exhausted.Insert())
	mixed := &model.Channel{
		Name: "tag-one-key-still-enabled", Type: 1, Key: "tag-keep\ntag-leave", Status: common.ChannelStatusManuallyDisabled,
		Models: "test-model", Group: "default", Tag: &tag,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled}},
	}
	require.NoError(t, mixed.Insert())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/tag/enabled", bytes.NewReader([]byte(`{"tag":"restore-tag"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	EnableTagChannels(c)
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())

	loaded, err := model.GetChannelById(exhausted.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, loaded.Status)
	assert.Empty(t, loaded.ChannelInfo.MultiKeyStatusList)
	key, _, keyErr := loaded.GetNextEnabledKey()
	if keyErr != nil {
		t.Fatalf("tag status=%d info=%+v err=%v", loaded.Status, loaded.ChannelInfo, keyErr)
	}
	assert.NotEmpty(t, key)

	loaded, err = model.GetChannelById(mixed.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, loaded.Status)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, loaded.ChannelInfo.MultiKeyStatusList[1])
	key, _, keyErr = loaded.GetNextEnabledKey()
	if keyErr != nil {
		t.Fatalf("mixed tag key err=%v info=%+v", keyErr, loaded.ChannelInfo)
	}
	assert.Equal(t, "tag-keep", key)
}
