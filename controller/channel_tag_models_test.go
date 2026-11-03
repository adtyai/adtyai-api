package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTagModelsReturnsUnion(t *testing.T) {
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
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-tag-models.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.AdtyGroup{}, &model.User{}, &model.Log{}, &model.AuditLog{}))

	tag := "union-tag"
	first := &model.Channel{
		Name: "tag-models-a", Type: 1, Key: "sk-a", Status: common.ChannelStatusEnabled,
		Models: "alpha,beta", Group: "default", Tag: &tag,
	}
	second := &model.Channel{
		Name: "tag-models-b", Type: 1, Key: "sk-b", Status: common.ChannelStatusEnabled,
		Models: "beta, gamma", Group: "default", Tag: &tag,
	}
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/tag/models?tag="+tag, nil)
	GetTagModels(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var result struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success, recorder.Body.String())
	got := map[string]struct{}{}
	for _, name := range strings.Split(result.Data, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			got[name] = struct{}{}
		}
	}
	assert.Contains(t, got, "alpha")
	assert.Contains(t, got, "beta")
	assert.Contains(t, got, "gamma")
	assert.Len(t, got, 3)

	body, err := common.Marshal(map[string]string{"tag": tag, "new_tag": "union-tag-renamed", "models": result.Data})
	require.NoError(t, err)
	edit := httptest.NewRecorder()
	editCtx, _ := gin.CreateTestContext(edit)
	editCtx.Set("id", 1)
	editCtx.Set("role", common.RoleRootUser)
	editCtx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/tag", bytes.NewReader(body))
	editCtx.Request.Header.Set("Content-Type", "application/json")
	EditTagChannels(editCtx)
	require.Contains(t, edit.Body.String(), `"success":true`, edit.Body.String())

	for _, id := range []int{first.Id, second.Id} {
		loaded, err := model.GetChannelById(id, true)
		require.NoError(t, err)
		require.NotNil(t, loaded.Tag)
		assert.Equal(t, "union-tag-renamed", *loaded.Tag)
		names := map[string]struct{}{}
		for _, name := range strings.Split(loaded.Models, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				names[name] = struct{}{}
			}
		}
		assert.Contains(t, names, "alpha")
		assert.Contains(t, names, "beta")
		assert.Contains(t, names, "gamma")
		var abilities []model.Ability
		require.NoError(t, database.Where("channel_id = ?", id).Find(&abilities).Error)
		abilityModels := map[string]struct{}{}
		for _, ability := range abilities {
			abilityModels[ability.Model] = struct{}{}
		}
		assert.Contains(t, abilityModels, "alpha")
		assert.Contains(t, abilityModels, "gamma")
	}
}
