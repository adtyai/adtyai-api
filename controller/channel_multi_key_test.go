package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMultiKeyEnableRestoresOnlyExhaustedChannels(t *testing.T) {
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
	common.SQLitePath = filepath.Join(t.TempDir(), "channel.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Log{}, &model.AuditLog{}, &model.AdtyGroup{}))
	root := &model.User{Username: "multi-key-review-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, database.Create(root).Error)
	t.Cleanup(func() { require.NoError(t, database.Unscoped().Delete(root).Error) })
	versionQuery := "SELECT VERSION()"
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, database.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database=%s version=%s", common.MainDatabaseType(), version)

	for _, cacheEnabled := range []bool{false, true} {
		for _, action := range []string{"enable_key", "enable_all_keys"} {
			for _, tc := range []struct {
				name           string
				initialStatus  int
				manualOverride string
				wantStatus     int
			}{
				{name: "key exhaustion restores", initialStatus: common.ChannelStatusEnabled, wantStatus: common.ChannelStatusEnabled},
				{name: "manual disable is preserved", initialStatus: common.ChannelStatusManuallyDisabled, wantStatus: common.ChannelStatusManuallyDisabled},
				{name: "manual disable after exhaustion is preserved", initialStatus: common.ChannelStatusEnabled, manualOverride: "status", wantStatus: common.ChannelStatusManuallyDisabled},
				{name: "tag disable after exhaustion is preserved", initialStatus: common.ChannelStatusEnabled, manualOverride: "tag", wantStatus: common.ChannelStatusManuallyDisabled},
			} {
				t.Run(fmt.Sprintf("cache=%t/%s/%s", cacheEnabled, action, tc.name), func(t *testing.T) {
					common.MemoryCacheEnabled = cacheEnabled
					tag := t.Name()
					channel := &model.Channel{Name: t.Name(), Type: 1, Key: "key-one\nkey-two", Status: tc.initialStatus, Models: "test-model", Group: "default", Tag: &tag,
						ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled}},
					}
					require.NoError(t, channel.Insert())
					t.Cleanup(func() {
						require.NoError(t, channel.Delete())
						model.InitChannelCache()
					})
					for _, operation := range []string{"disable_key", action} {
						if operation == action {
							if tc.manualOverride == "status" {
								model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual operation")
							} else if tc.manualOverride == "tag" {
								require.NoError(t, model.DisableChannelByTag(tag))
							}
						}
						payload, err := common.Marshal(MultiKeyManageRequest{ChannelId: channel.Id, Action: operation, KeyIndex: common.GetPointer(0)})
						require.NoError(t, err)
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						c.Set("id", root.Id)
						c.Set("role", common.RoleRootUser)
						c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
						c.Request.Header.Set("Content-Type", "application/json")
						ManageMultiKeys(c)
						var result struct {
							Success bool `json:"success"`
						}
						require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
						require.True(t, result.Success, recorder.Body.String())
					}
					loaded, err := model.GetChannelById(channel.Id, true)
					require.NoError(t, err)
					assert.Equal(t, tc.wantStatus, loaded.Status)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyStatusList, 0)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyDisabledReason, 0)
					assert.NotContains(t, loaded.ChannelInfo.MultiKeyDisabledTime, 0)
					var ability model.Ability
					require.NoError(t, database.Where("channel_id = ?", channel.Id).First(&ability).Error)
					assert.Equal(t, tc.wantStatus == common.ChannelStatusEnabled, ability.Enabled)
				})
			}
		}
	}
}

func TestMultiKeyManageKeepsConcurrentQuotaAndKeyStatus(t *testing.T) {
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
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-quota.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Log{}, &model.AuditLog{}, &model.AdtyGroup{}))
	root := &model.User{Username: "multi-key-quota-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, database.Create(root).Error)

	channel := &model.Channel{
		Name: "quota-race", Type: 1, Key: "key-one\nkey-two", Status: common.ChannelStatusEnabled,
		Models: "test-model", Group: "default", UsedQuota: 10, Balance: 4,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{1: 3}},
	}
	require.NoError(t, channel.Insert())

	var once sync.Once
	require.NoError(t, database.Callback().Update().Before("gorm:update").Register("test:concurrent_channel_quota", func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "channels" {
			return
		}
		once.Do(func() {
			require.NoError(t, tx.Exec("UPDATE channels SET used_quota = ?, balance = ? WHERE id = ?", 99, 8.5, channel.Id).Error)
		})
	}))
	t.Cleanup(func() {
		database.Callback().Update().Remove("test:concurrent_channel_quota")
	})

	payload, err := common.Marshal(MultiKeyManageRequest{ChannelId: channel.Id, Action: "disable_key", KeyIndex: common.GetPointer(0)})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", root.Id)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	ManageMultiKeys(c)
	var result struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success, recorder.Body.String())

	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.EqualValues(t, 99, loaded.UsedQuota)
	assert.Equal(t, 8.5, loaded.Balance)
	assert.Equal(t, 2, loaded.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, 3, loaded.ChannelInfo.MultiKeyStatusList[1])
}

func TestChannelEditKeepsKeyStatusCommittedUnderPollingLock(t *testing.T) {
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
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-edit.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Log{}, &model.AuditLog{}, &model.AdtyGroup{}))
	root := &model.User{Username: "channel-edit-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, database.Create(root).Error)

	channel := &model.Channel{
		Name: "before-edit", Type: 1, Key: "key-one\nkey-two", Status: common.ChannelStatusEnabled,
		Models: "test-model", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyPollingIndex: 1},
	}
	require.NoError(t, channel.Insert())

	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	finished := make(chan string, 1)
	go func() {
		body := fmt.Sprintf(`{"id":%d,"name":"renamed-channel","type":1,"key":"key-one\nkey-two","models":"test-model","group":"default"}`, channel.Id)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", root.Id)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader([]byte(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		UpdateChannel(c)
		finished <- recorder.Body.String()
	}()
	select {
	case body := <-finished:
		lock.Unlock()
		t.Fatalf("channel edit did not wait for the polling lock: %s", body)
	case <-time.After(200 * time.Millisecond):
	}
	disabled := model.ChannelInfo{
		IsMultiKey: true, MultiKeySize: 2, MultiKeyPollingIndex: 7,
		MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
	}
	require.NoError(t, database.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("channel_info", disabled).Error)
	lock.Unlock()
	var response string
	select {
	case response = <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("channel edit did not finish")
	}
	require.Contains(t, response, `"success":true`, response)

	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "renamed-channel", loaded.Name)
	assert.Equal(t, 7, loaded.ChannelInfo.MultiKeyPollingIndex)
	assert.Equal(t, common.ChannelStatusAutoDisabled, loaded.ChannelInfo.MultiKeyStatusList[0])
}

func TestDeleteDisabledKeysRejectsRemovingEveryKey(t *testing.T) {
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
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-delete-disabled.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Log{}, &model.AuditLog{}, &model.AdtyGroup{}))
	root := &model.User{Username: "delete-disabled-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, database.Create(root).Error)

	call := func(channelID int) string {
		t.Helper()
		payload, err := common.Marshal(MultiKeyManageRequest{ChannelId: channelID, Action: "delete_disabled_keys"})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", root.Id)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		ManageMultiKeys(c)
		return recorder.Body.String()
	}

	blocked := &model.Channel{
		Name: "all-auto-disabled", Type: 1, Key: "key-one\nkey-two", Status: common.ChannelStatusEnabled,
		Models: "test-model", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{0: 3, 1: 3}},
	}
	require.NoError(t, blocked.Insert())
	assert.Contains(t, call(blocked.Id), "Cannot delete the last key")
	loaded, err := model.GetChannelById(blocked.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "key-one\nkey-two", loaded.Key)
	assert.Equal(t, 3, loaded.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, 3, loaded.ChannelInfo.MultiKeyStatusList[1])

	partial := &model.Channel{
		Name: "one-auto-disabled", Type: 1, Key: "keep-me\ndrop-me", Status: common.ChannelStatusEnabled,
		Models: "test-model", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{1: 3}},
	}
	require.NoError(t, partial.Insert())
	assert.Contains(t, call(partial.Id), `"success":true`)
	loaded, err = model.GetChannelById(partial.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "keep-me", loaded.Key)
	assert.Empty(t, loaded.ChannelInfo.MultiKeyStatusList)
	assert.Equal(t, 1, loaded.ChannelInfo.MultiKeySize)
}
