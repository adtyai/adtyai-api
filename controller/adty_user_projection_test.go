package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 客户接口不露渠道、倍率和上游信息（任务 7.1）。

func adtyTaskFixture() *model.Task {
	return &model.Task{
		TaskID:    "task_public",
		ChannelId: 42,
		Properties: model.Properties{
			Input: "a cat", UpstreamModelName: "vendor-real-model", OriginModelName: "video-1",
		},
	}
}

func TestUserTasksHideChannelAndUpstreamModel(t *testing.T) {
	task := adtyTaskFixture()

	userView := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.Zero(t, userView.ChannelId)
	props, ok := userView.Properties.(model.Properties)
	require.True(t, ok)
	assert.Empty(t, props.UpstreamModelName)
	assert.Equal(t, "video-1", props.OriginModelName)

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]
	assert.Equal(t, 42, adminView.ChannelId)
	assert.Equal(t, "vendor-real-model", adminView.Properties.(model.Properties).UpstreamModelName)
}

func TestVideoFetchHidesChannel(t *testing.T) {
	// The video fetch API returns TaskModel2Dto straight to API tokens.
	view := relay.TaskModel2Dto(adtyTaskFixture())
	assert.Zero(t, view.ChannelId)
}

func TestUserMidjourneyHideChannelId(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}))
	require.NoError(t, db.Create(&model.Midjourney{UserId: 7, MjId: "mj-1", ChannelId: 42, Action: "IMAGINE"}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/mj/self?p=1&page_size=10", nil)
	c.Set("id", 7)
	GetUserMidjourney(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"mj_id":"mj-1"`)
	assert.Contains(t, recorder.Body.String(), `"channel_id":0`)
	assert.NotContains(t, recorder.Body.String(), `"channel_id":42`)
}

func TestSelfStatIgnoresChannelForUsers(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = oldLogDB })
	now := common.GetTimestamp()
	require.NoError(t, db.Create([]*model.Log{
		{UserId: 7, Username: "stat-user", Type: model.LogTypeConsume, Quota: 100, ChannelId: 1, CreatedAt: now},
		{UserId: 7, Username: "stat-user", Type: model.LogTypeConsume, Quota: 50, ChannelId: 2, CreatedAt: now},
	}).Error)

	stat := func(role int) string {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/stat?type=2&channel=1", nil)
		c.Set("id", 7)
		c.Set("username", "stat-user")
		c.Set("role", role)
		GetLogsSelfStat(c)
		return recorder.Body.String()
	}
	assert.Contains(t, stat(common.RoleCommonUser), `"quota":150`, "the channel filter is ignored for users")
	assert.Contains(t, stat(common.RoleAdminUser), `"quota":100`)
}

func TestRelayErrorMasksUpstreamURL(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://secret-upstream.example.com")

	apiErr := types.NewOpenAIError(errors.New("Post \"https://secret-upstream.example.com/v1/chat/completions\": dial tcp: timeout"),
		types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	message := apiErr.ToOpenAIError(common.UpstreamAddresses(c)...).Message

	assert.NotContains(t, message, "secret-upstream.example.com")
	assert.True(t, strings.Contains(message, "timeout"), message)
}
