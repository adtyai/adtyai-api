package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestAddChannelRefreshesRouteCache(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
	model.InitChannelCache()

	before, err := model.GetRandomSatisfiedChannel("default", "cache-new-model", 0, nil)
	require.NoError(t, err)
	require.Nil(t, before)

	recorder := postAddChannel(t, 1, common.RoleRootUser, `{"mode":"single","channel":{"type":1,"name":"cache-new","key":"sk-cache","models":"cache-new-model","group":"default","status":1}}`)
	require.Contains(t, recorder.Body.String(), `"success":true`)

	selected, err := model.GetRandomSatisfiedChannel("default", "cache-new-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, "cache-new", selected.Name)
}

func TestChannelCacheRefreshAfterPartialBatchFailure(t *testing.T) {
	for _, operation := range []string{"plugin cascade", "upstream models"} {
		t.Run(operation, func(t *testing.T) {
			setupTaskPluginBindChannelTest(t)
			previousCache := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = true
			t.Cleanup(func() {
				model.InitChannelCache()
				common.MemoryCacheEnabled = previousCache
			})
			channel := &model.Channel{Name: "changed", Type: 1, Key: "sk-test", Models: "kept,revoked", Group: "default", Status: common.ChannelStatusEnabled}
			if operation == "plugin cascade" {
				require.NoError(t, model.DB.AutoMigrate(&model.Task{}))
				channel.Type = constant.ChannelTypeNewAPI
				channel.Setting = common.GetPointer(`{"task_extend_plugin_keys":["test-plugin"]}`)
			} else {
				channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamModelUpdateCheckEnabled: true, UpstreamModelUpdateLastRemovedModels: []string{"revoked"}})
			}
			require.NoError(t, channel.Insert())
			if operation == "plugin cascade" {
				other := *channel
				other.Id = 0
				require.NoError(t, other.Insert())
			} else {
				// Fill exactly one production page so the following page is queried.
				fillers := make([]model.Channel, channelUpstreamModelUpdateTaskBatchSize-1)
				for i := range fillers {
					fillers[i] = model.Channel{Name: "unchanged", Type: 1, Status: common.ChannelStatusEnabled}
				}
				require.NoError(t, model.DB.Create(&fillers).Error)
			}
			require.NoError(t, model.InitChannelCache())
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			if operation == "plugin cascade" {
				writes := 0
				require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("cache-fail-second-unbind", func(tx *gorm.DB) {
					if tx.Statement.Table == "channels" {
						writes++
						if writes == 2 {
							_ = tx.AddError(errors.New("second unbind unavailable"))
						}
					}
				}))
				t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove("cache-fail-second-unbind")) })
				ctx.Params = gin.Params{{Key: "key", Value: "test-plugin"}}
				ctx.Request = httptest.NewRequest(http.MethodPost, "/api/plugin/task/test-plugin/status?cascade=true", strings.NewReader(`{"enabled":false}`))
				ctx.Request.Header.Set("Content-Type", "application/json")
				SetTaskPluginStatus(ctx)
				stored, err := model.GetChannelById(channel.Id, true)
				require.NoError(t, err)
				require.False(t, stored.GetSetting().BindsTaskPlugin("test-plugin"), "the first write committed")
				cached, err := model.CacheGetChannel(channel.Id)
				require.NoError(t, err)
				assert.False(t, cached.GetSetting().BindsTaskPlugin("test-plugin"))
			} else {
				pages := 0
				require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("cache-fail-second-page", func(tx *gorm.DB) {
					if limit, ok := tx.Statement.Clauses["LIMIT"].Expression.(clause.Limit); ok && limit.Limit != nil && *limit.Limit == channelUpstreamModelUpdateTaskBatchSize {
						pages++
						if pages == 2 {
							_ = tx.AddError(errors.New("second page unavailable"))
						}
					}
				}))
				t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove("cache-fail-second-page")) })
				ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/upstream_models/apply_all", nil)
				ApplyAllChannelUpstreamModelUpdates(ctx)
				stored, err := model.GetChannelById(channel.Id, true)
				require.NoError(t, err)
				require.Equal(t, "kept", stored.Models, "the first page committed")
				cached, err := model.CacheGetChannel(channel.Id)
				require.NoError(t, err)
				assert.Equal(t, "kept", cached.Models)
			}
			require.Contains(t, recorder.Body.String(), `"success":false`)
		})
	}
}

func TestDeleteChannelDoesNotRouteToDeletedChannelWhenCacheRefreshFails(t *testing.T) {
	for _, failedQuery := range []string{"channels", "abilities"} {
		for _, operation := range []string{"delete", "batch delete", "disable tag", "change models"} {
			t.Run(failedQuery+"/"+operation, func(t *testing.T) {
				setupTaskPluginBindChannelTest(t)
				previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
				common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
				model.InitColumnNamesForTests()
				t.Cleanup(func() {
					common.SetDatabaseTypes(previousMain, previousLog)
					model.InitColumnNamesForTests()
				})
				previousCache := common.MemoryCacheEnabled
				common.MemoryCacheEnabled = true
				t.Cleanup(func() {
					model.InitChannelCache()
					common.MemoryCacheEnabled = previousCache
				})
				channel := &model.Channel{
					Name: "changed-cached-route", Type: 1, Key: "sk-changed", Models: "cache-old-model",
					Group: "default", Status: common.ChannelStatusEnabled, Tag: common.GetPointer("changed-tag"),
				}
				healthy := &model.Channel{
					Name: "healthy-route", Type: 1, Key: "sk-healthy", Models: "cache-healthy-model",
					Group: "default", Status: common.ChannelStatusEnabled,
				}
				require.NoError(t, channel.Insert())
				require.NoError(t, healthy.Insert())
				model.InitChannelCache()
				selected, err := model.GetRandomSatisfiedChannel("default", "cache-old-model", 0, nil)
				require.NoError(t, err)
				require.NotNil(t, selected)
				refreshFails := true
				require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("fail-channel-cache-reload", func(tx *gorm.DB) {
					_, hasWhere := tx.Statement.Clauses["WHERE"]
					if refreshFails && tx.Statement.Table == failedQuery && !hasWhere {
						_ = tx.AddError(errors.New("injected channel cache reload failure"))
					}
				}))
				t.Cleanup(func() { _ = model.DB.Callback().Query().Remove("fail-channel-cache-reload") })
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Set("id", 1)
				ctx.Set("role", common.RoleRootUser)
				ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(channel.Id)}}
				switch operation {
				case "delete":
					ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/"+fmt.Sprint(channel.Id), nil)
					DeleteChannel(ctx)
				case "batch delete":
					ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/batch", strings.NewReader(fmt.Sprintf(`{"ids":[%d]}`, channel.Id)))
					DeleteChannelBatch(ctx)
				case "disable tag":
					ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/tag/disable", strings.NewReader(`{"tag":"changed-tag"}`))
					DisableTagChannels(ctx)
				case "change models":
					ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/tag", strings.NewReader(`{"tag":"changed-tag","models":"cache-new-model"}`))
					EditTagChannels(ctx)
				}
				require.Contains(t, recorder.Body.String(), `"success":true`)
				selected, err = model.GetRandomSatisfiedChannel("default", "cache-old-model", 0, nil)
				require.NoError(t, err)
				assert.Nil(t, selected, "committed changes must remove the old route")
				assert.False(t, model.IsChannelEnabledForGroupModel("default", "cache-old-model", channel.Id))
				if operation == "delete" || operation == "batch delete" {
					cached, err := model.CacheGetChannel(channel.Id)
					assert.Error(t, err)
					assert.Nil(t, cached)
					info, err := model.CacheGetChannelInfo(channel.Id)
					assert.Error(t, err)
					assert.Nil(t, info)
				}
				selected, err = model.GetRandomSatisfiedChannel("default", "cache-healthy-model", 0, nil)
				require.NoError(t, err)
				require.NotNil(t, selected, "unaffected channels remain available through the database")
				assert.Equal(t, healthy.Id, selected.Id)
				if operation == "change models" {
					selected, err = model.GetRandomSatisfiedChannel("default", "cache-new-model", 0, nil)
					require.NoError(t, err)
					require.NotNil(t, selected)
					assert.Equal(t, channel.Id, selected.Id)
				}
				refreshFails = false
				model.InitChannelCache()
				selected, err = model.GetRandomSatisfiedChannel("default", "cache-old-model", 0, nil)
				require.NoError(t, err)
				assert.Nil(t, selected)
			})
		}
	}
}

func TestBatchAddKeyPrefixDoesNotStackOntoLaterNames(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	body := "{\"mode\":\"batch\",\"batch_add_set_key_prefix_2_name\":true,\"channel\":{\"type\":1,\"name\":\"batch-base\",\"key\":\"aaaaaaaa-1\\nbbbbbbbb-2\",\"models\":\"gpt\",\"group\":\"default\",\"status\":1}}"
	recorder := postAddChannel(t, 1, common.RoleRootUser, body)
	require.Contains(t, recorder.Body.String(), `"success":true`)

	var rows []model.Channel
	require.NoError(t, model.DB.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, "batch-base aaaaaaaa", rows[0].Name)
	require.Equal(t, "batch-base bbbbbbbb", rows[1].Name)
	require.Equal(t, "aaaaaaaa-1", rows[0].Key)
	require.Equal(t, "bbbbbbbb-2", rows[1].Key)
}

func TestEditTagRejectsModelMappingThatIsNotAStringObject(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	tag := "map-tag"
	channel := &model.Channel{
		Name: "mapped", Type: 1, Key: "sk", Models: "gpt", Group: "default", Status: 1, Tag: &tag,
		ModelMapping: common.GetPointer(`{"gpt":"upstream"}`),
	}
	require.NoError(t, model.DB.Create(channel).Error)

	reject := func(t *testing.T, mapping string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", 1)
		ctx.Set("role", common.RoleRootUser)
		payload, err := common.Marshal(map[string]string{"tag": "map-tag", "model_mapping": mapping})
		require.NoError(t, err)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/tag", strings.NewReader(string(payload)))
		ctx.Request.Header.Set("Content-Type", "application/json")
		EditTagChannels(ctx)
		require.Contains(t, recorder.Body.String(), `"success":false`, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), "Model mapping must be a JSON object of strings")
	}
	reject(t, `[1]`)
	reject(t, `{"gpt":1}`)
	reject(t, `not-json`)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	require.NotNil(t, stored.ModelMapping)
	require.Equal(t, `{"gpt":"upstream"}`, *stored.ModelMapping)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/tag", strings.NewReader(`{"tag":"map-tag","model_mapping":"{\"gpt\":\"real-upstream\"}"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	EditTagChannels(ctx)
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	require.Equal(t, `{"gpt":"real-upstream"}`, *stored.ModelMapping)
}
