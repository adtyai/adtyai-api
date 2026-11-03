package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelEditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	common.SQLitePath = filepath.Join(t.TempDir(), "channel-edit-models.db")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = database
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.AdtyGroup{}, &model.User{}, &model.Log{}, &model.AuditLog{}))
	return database
}

func putChannelModels(t *testing.T, id int, models string) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{"id":` + strconv.Itoa(id) + `,"name":"edited-channel","type":1,"models":"` + models + `","group":"default"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(c)
	return recorder
}

func TestUpdateChannelRejectsModelNameLongerThanAbilityColumn(t *testing.T) {
	database := setupChannelEditTestDB(t)
	channel := &model.Channel{
		Name: "model-length", Type: 1, Key: "sk-model-length", Status: common.ChannelStatusEnabled,
		Models: "short-model", Group: "default",
	}
	require.NoError(t, channel.Insert())

	longName := strings.Repeat("m", 256)
	recorder := putChannelModels(t, channel.Id, longName)
	assert.Contains(t, recorder.Body.String(), "Model name is too long")
	assert.NotContains(t, recorder.Body.String(), `"success":true`)

	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "short-model", loaded.Models)
	var abilities []model.Ability
	require.NoError(t, database.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.NotEmpty(t, abilities)
	for _, ability := range abilities {
		assert.Equal(t, "short-model", ability.Model)
		assert.NotEqual(t, longName, ability.Model)
	}
}

func TestUpdateChannelRollsBackModelsWhenAbilityWriteFails(t *testing.T) {
	database := setupChannelEditTestDB(t)
	channel := &model.Channel{
		Name: "model-rollback", Type: 1, Key: "sk-model-rollback", Status: common.ChannelStatusEnabled,
		Models: "old-model", Group: "default",
	}
	require.NoError(t, channel.Insert())

	require.NoError(t, database.Callback().Create().Before("gorm:create").Register("test:fail_ability_create", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("ability write failed"))
		}
	}))
	t.Cleanup(func() {
		database.Callback().Create().Remove("test:fail_ability_create")
	})

	recorder := putChannelModels(t, channel.Id, "new-model")
	assert.Contains(t, recorder.Body.String(), "ability write failed")

	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "old-model", loaded.Models)
	var abilities []model.Ability
	require.NoError(t, database.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.NotEmpty(t, abilities)
	for _, ability := range abilities {
		assert.Equal(t, "old-model", ability.Model)
	}
}

func TestEditTagRollsBackModelsWhenAbilityWriteFails(t *testing.T) {
	database := setupChannelEditTestDB(t)
	tag := "ability-fail-tag"
	channel := &model.Channel{
		Name: "tag-models", Type: 1, Key: "sk-tag-models", Status: common.ChannelStatusEnabled,
		Models: "old-model", Group: "default", Tag: &tag,
	}
	require.NoError(t, channel.Insert())

	require.NoError(t, database.Callback().Create().Before("gorm:create").Register("test:fail_tag_ability_create", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("ability write failed"))
		}
	}))
	t.Cleanup(func() {
		database.Callback().Create().Remove("test:fail_tag_ability_create")
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/tag", strings.NewReader(`{"tag":"ability-fail-tag","models":"new-model"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	EditTagChannels(ctx)

	require.Contains(t, recorder.Body.String(), "ability write failed")
	require.NotContains(t, recorder.Body.String(), `"success":true`)

	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, "old-model", loaded.Models)
	var abilities []model.Ability
	require.NoError(t, database.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.NotEmpty(t, abilities)
	for _, ability := range abilities {
		require.Equal(t, "old-model", ability.Model)
	}
}

func TestDeleteChannelKeepsRowWhenAbilityDeleteFails(t *testing.T) {
	database := setupChannelEditTestDB(t)
	channel := &model.Channel{
		Name: "delete-rollback", Type: 1, Key: "sk-delete", Status: common.ChannelStatusEnabled,
		Models: "kept-model", Group: "default",
	}
	require.NoError(t, channel.Insert())

	require.NoError(t, database.Callback().Delete().Before("gorm:delete").Register("test:fail_ability_delete", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("ability delete failed"))
		}
	}))
	t.Cleanup(func() {
		database.Callback().Delete().Remove("test:fail_ability_delete")
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/"+strconv.Itoa(channel.Id), nil)
	DeleteChannel(ctx)

	require.Contains(t, recorder.Body.String(), "ability delete failed")
	require.NotContains(t, recorder.Body.String(), `"success":true`)

	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, "delete-rollback", loaded.Name)
	var abilities []model.Ability
	require.NoError(t, database.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.NotEmpty(t, abilities)
	for _, ability := range abilities {
		require.Equal(t, "kept-model", ability.Model)
	}
}
