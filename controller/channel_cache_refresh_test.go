package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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
