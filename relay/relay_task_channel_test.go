package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoFetchDoesNotLeakChannelID(t *testing.T) {
	previousDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Task{}))
	model.DB = database
	t.Cleanup(func() { model.DB = previousDB })

	task := &model.Task{TaskID: "task-hidden-channel", UserId: 7, ChannelId: 42, Platform: "missing-platform", Status: "SUCCESS"}
	require.NoError(t, database.Create(task).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-hidden-channel", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	ctx.Set("id", 7)
	_, taskErr := videoFetchByIDRespBodyBuilder(ctx)
	require.NotNil(t, taskErr)
	require.NotContains(t, taskErr.Message, "42")
	require.NotContains(t, common.LogText("%v", taskErr.Error), "42")
}
