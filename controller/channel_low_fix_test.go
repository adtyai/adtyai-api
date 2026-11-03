package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMultiKeyAuditWaitsForSuccess(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.AdtyGroup{}, &model.AuditLog{}, &model.User{}))
	previousLog, previousRedis, previousCache := model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = previousLog
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousCache
	})
	channel := &model.Channel{
		Name: "audit-keys", Type: 1, Key: "key-one\nkey-two", Status: 1, Models: "m", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2},
	}
	require.NoError(t, channel.Insert())

	call := func(index int) string {
		t.Helper()
		payload, err := common.Marshal(MultiKeyManageRequest{ChannelId: channel.Id, Action: "disable_key", KeyIndex: common.GetPointer(index)})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", 1)
		ctx.Set("role", common.RoleRootUser)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", bytes.NewReader(payload))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ManageMultiKeys(ctx)
		return recorder.Body.String()
	}

	require.Contains(t, call(9), `"success":false`)
	var audits int64
	require.NoError(t, db.Model(&model.AuditLog{}).Where("action = ?", "channel.multi_key_manage").Count(&audits).Error)
	require.Zero(t, audits)

	require.Contains(t, call(0), `"success":true`)
	require.NoError(t, db.Model(&model.AuditLog{}).Where("action = ?", "channel.multi_key_manage").Count(&audits).Error)
	require.Equal(t, int64(1), audits)
}

func TestChannelSearchDoesNotMatchFullKey(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	require.NoError(t, db.Create(&model.Channel{
		Name: "named-channel", Key: "secret-key-value", Type: 1, Models: "gpt", Group: "default", Status: 1,
	}).Error)

	search := func(keyword string) string {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/search?keyword="+keyword, nil)
		SearchChannels(ctx)
		return recorder.Body.String()
	}
	require.Contains(t, search("secret-key-value"), `"total":0`)
	require.Contains(t, search("named-channel"), `"total":1`)
	require.NotContains(t, search("named-channel"), "secret-key-value")
}

func TestChannelListNegativePageSizeReturnsOnePage(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	for i := 0; i < 15; i++ {
		require.NoError(t, db.Create(&model.Channel{
			Name: "page-channel", Key: "k", Type: 1, Models: "gpt", Group: "default", Status: 1,
		}).Error)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/?page_size=-1", nil)
	GetAllChannels(ctx)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Contains(t, recorder.Body.String(), `"total":15`)
	require.Contains(t, recorder.Body.String(), `"page_size":10`)
	require.Equal(t, 10, bytes.Count(recorder.Body.Bytes(), []byte(`"name":"page-channel"`)))
}

func TestBatchAddTrimsKeysAndRejectsBlank(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	recorder := postAddChannel(t, 1, common.RoleRootUser, `{"mode":"batch","channel":{"type":1,"name":"batch-trim","key":"  sk-left  \n\n sk-right\r","models":"gpt","group":"default","status":1}}`)
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())

	var rows []model.Channel
	require.NoError(t, model.DB.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, "sk-left", rows[0].Key)
	require.Equal(t, "sk-right", rows[1].Key)

	blank := postAddChannel(t, 1, common.RoleRootUser, `{"mode":"batch","channel":{"type":1,"name":"blank","key":"\n  \n","models":"gpt","group":"default","status":1}}`)
	require.Contains(t, blank.Body.String(), `"success":false`, blank.Body.String())
	require.NoError(t, model.DB.Find(&rows).Error)
	require.Len(t, rows, 2)
}
