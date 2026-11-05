package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogCleanupRejectsUnboundedTimestamp(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.Log{}))
	require.NoError(t, db.Create(&model.Log{
		UserId: 1, CreatedAt: time.Now().Unix(), Type: model.LogTypeTopup, Content: "paid-order",
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/system-task/log-cleanup?target_timestamp=9999999999", nil)
	CreateLogCleanupSystemTask(c)

	require.NotEqual(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "90")
	var remaining int64
	require.NoError(t, db.Model(&model.Log{}).Where("content = ?", "paid-order").Count(&remaining).Error)
	assert.EqualValues(t, 1, remaining)
}

func TestSystemTaskListFiltersAndPaginationResponse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}))
	require.NoError(t, db.Create(&[]model.SystemTask{
		{TaskID: "older", Type: model.SystemTaskTypeModelUpdate, Status: model.SystemTaskStatusFailed},
		{TaskID: "newer", Type: model.SystemTaskTypeModelUpdate, Status: model.SystemTaskStatusFailed},
		{TaskID: "other-status", Type: model.SystemTaskTypeModelUpdate, Status: model.SystemTaskStatusSucceeded},
		{TaskID: "other-type", Type: model.SystemTaskTypeChannelTest, Status: model.SystemTaskStatusFailed},
	}).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/system-task/list?scope=history&type=model_update&status=failed&offset=1&limit=1", nil)
	ListSystemTasks(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                       `json:"success"`
		Total   int64                      `json:"total"`
		Data    []model.SystemTaskResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.EqualValues(t, 2, response.Total)
	require.Len(t, response.Data, 1)
	assert.Equal(t, "older", response.Data[0].TaskID)
}

func TestModelUpdateTaskFailureIsNotRecordedAsSuccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	run := func(t *testing.T, taskID string, ctx context.Context) model.SystemTask {
		t.Helper()
		task := &model.SystemTask{
			TaskID: taskID, Type: model.SystemTaskTypeModelUpdate,
			Status: model.SystemTaskStatusRunning, LockedBy: "runner-test", Payload: `{"manual":true}`,
		}
		require.NoError(t, db.Create(task).Error)
		require.NoError(t, db.Create(&model.SystemTaskLock{
			Type: model.SystemTaskTypeModelUpdate, TaskID: taskID, LockedBy: "runner-test",
			LockedUntil: common.GetTimestamp() + 3600,
		}).Error)
		modelUpdateHandler{}.Run(ctx, task, "runner-test")
		var stored model.SystemTask
		require.NoError(t, db.Where("task_id = ?", taskID).First(&stored).Error)
		return stored
	}

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stored := run(t, "detect-cancelled", ctx)
		require.Equal(t, model.SystemTaskStatusFailed, stored.Status)
		require.Contains(t, stored.Error, "context canceled")
	})

	t.Run("query failed", func(t *testing.T) {
		stored := run(t, "detect-query-failed", context.Background())
		require.Equal(t, model.SystemTaskStatusFailed, stored.Status)
		require.NotEmpty(t, stored.Error)
		require.NotEqual(t, model.SystemTaskStatusSucceeded, stored.Status)
	})
}

func TestSystemTaskInvalidFiltersAreRejected(t *testing.T) {
	for _, tc := range []struct {
		query   string
		handler gin.HandlerFunc
	}{
		{"?scope=invalid", ListSystemTasks},
		{"?status=unknown", ListSystemTasks},
		{"?offset=-1", ListSystemTasks},
		{"?limit=invalid", ListSystemTasks},
		{"?status=unknown", DeleteSystemTaskHistory},
		{"?scope=invalid", DeleteSystemTaskHistory},
	} {
		t.Run(tc.query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/"+tc.query, nil)
			tc.handler(c)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.JSONEq(t, `{"success":false,"message":"invalid system task filters"}`, recorder.Body.String())
		})
	}
}
