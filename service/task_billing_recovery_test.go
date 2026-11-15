package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common/logsinktest"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归 ISS-R4-0095：adaptor 缺失的两支 `continue` 既不推进也不留痕，任务永远
// 停在 billing_pending。修好以后：一轮恢复就要留一条 SysError、把复核原因写到行上、
// 并且让这行离开 pending 集（第二轮不能再捞到它，也不能再记同样的错误）。
func TestRecoverBillingOperationsTerminatesTasksWithoutAdaptor(t *testing.T) {
	for _, tt := range []struct {
		name    string
		factory func(constant.TaskPlatform) TaskPollingAdaptor
	}{
		{name: "adaptor factory is not registered"},
		{name: "adaptor is not registered for the platform", factory: func(constant.TaskPlatform) TaskPollingAdaptor { return nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t)
			const userID, tokenID, channelID = 9701, 9701, 9701
			seedUser(t, userID, 990_000)
			seedToken(t, tokenID, userID, "sk-adty-retired-platform", 90_000)
			seedChannel(t, channelID)
			task := &model.Task{
				TaskID:         "task_retired_platform",
				UserId:         userID,
				ChannelId:      channelID,
				Group:          "default",
				Quota:          10_000,
				Platform:       constant.TaskPlatform("retired-platform"),
				Status:         model.TaskStatusSuccess,
				BillingPending: true,
				Properties:     model.Properties{OriginModelName: "test-model"},
				PrivateData:    model.TaskPrivateData{TokenId: tokenID},
			}
			require.NoError(t, model.DB.Create(task).Error)

			previous := GetTaskAdaptorFunc
			GetTaskAdaptorFunc = tt.factory
			t.Cleanup(func() { GetTaskAdaptorFunc = previous })
			logs := logsinktest.Capture(t)

			require.NoError(t, RecoverBillingOperations(context.Background(), 100))

			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.False(t, stored.BillingPending, "the task must leave the pending set instead of being skipped every 15s")
			assert.Contains(t, stored.PrivateData.BillingReviewReason, "retired-platform",
				"the row must carry why it stopped so operations can find it")
			assert.Contains(t, logs.String(), "ERROR", "the skipped branch must be auditable")
			assert.Contains(t, logs.String(), "task_retired_platform")
			assert.Equal(t, 990_000, getUserQuota(t, userID), "the reserved charge must not be guessed at")

			before := logs.String()
			require.NoError(t, RecoverBillingOperations(context.Background(), 100))
			assert.Equal(t, before, logs.String(), "a flagged task must not be picked up again")
		})
	}
}
