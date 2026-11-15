package service

import (
	"context"
	"fmt"
	"testing"
	"time"

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

// 回归 ISS-R4-0094：永久失败的 op 让队列堵死。失败的结算整笔回滚，updated_at
// 不前进，于是同一条 op 每轮都排在队头；积够 limit 条以后，排在后面的正常 op
// 永远轮不到。修好以后：失败落到 attempts/next_retry_at 上，队列按 next_retry_at
// 排序，正常 op 在下一轮就能被结算。
func TestRecoverBillingOperationsBacksOffPoisonedOperations(t *testing.T) {
	t.Run("a healthy operation is reached behind a full batch of poisoned operations", func(t *testing.T) {
		truncate(t)
		const userID, tokenID, channelID = 9801, 9801, 9801
		seedUser(t, userID, 1_000_000)
		seedToken(t, tokenID, userID, "sk-adty-poison-queue", 100_000)
		seedChannel(t, channelID)

		// 100 条永远结算不了的 op：user_id 指向不存在的用户，ApplyBillingSettlement
		// 每次都在查用户那一步失败、整笔回滚。updated_at 停在 1，排在正常 op 前面。
		for i := 0; i < 100; i++ {
			require.NoError(t, model.DB.Create(&model.BillingOperation{
				OperationKey: fmt.Sprintf("poison:%d", i), UserId: 90_000 + i, Kind: "request",
				State: model.BillingSettlePending, ReservedQuota: 100, ActualQuota: 100, UpdatedAt: 1,
			}).Error)
		}

		// 一条真的能结算的 op，但它的 updated_at 最新，所以排在 100 条之后。
		c := adtyWalletRelayContext()
		info := adtyWalletRelayInfo(userID, tokenID, "sk-adty-poison-queue")
		require.Nil(t, PreConsumeBilling(c, 1_000, info))
		key := billingSessionKey(t, info)
		require.NoError(t, model.QueueBillingSettlement(key, 1_000, false))

		// 这一轮仍然有结算不了的 op，所以恢复链如实返回第一个错误。
		require.Error(t, RecoverBillingOperations(context.Background(), 100))
		healthy, err := model.GetBillingOperation(key)
		require.NoError(t, err)
		assert.Equal(t, model.BillingSettlePending, healthy.State, "round 1 still starts with the poisoned head of the queue")

		// 本条唯一的验收：排在 100 条永久失败 op 之后的正常 op 必须在下一轮被结算。
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		healthy, err = model.GetBillingOperation(key)
		require.NoError(t, err)
		assert.Equal(t, model.BillingSettled, healthy.State, "the healthy operation must be reached behind the poisoned ones")

		poisoned, err := model.GetBillingOperation("poison:0")
		require.NoError(t, err)
		assert.Equal(t, 1, poisoned.Attempts, "a failed attempt must be recorded on the row")
		assert.NotEmpty(t, poisoned.LastError)
		assert.Greater(t, poisoned.NextRetryAt, time.Now().Unix(), "a failed operation must back off instead of taking the head again")

		// 第三轮还没到 next_retry_at：这一轮没有失败可报，也不能再记一次失败。
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		poisoned, err = model.GetBillingOperation("poison:0")
		require.NoError(t, err)
		assert.Equal(t, 1, poisoned.Attempts, "a backed-off operation must not be retried before its next_retry_at")
	})

	t.Run("an operation that used up its budget is dead-lettered", func(t *testing.T) {
		truncate(t)
		logs := logsinktest.Capture(t)
		require.NoError(t, model.DB.Create(&model.BillingOperation{
			OperationKey: "poison:exhausted", UserId: 90_999, Kind: "request",
			State: model.BillingSettlePending, ReservedQuota: 100, ActualQuota: 100,
			Attempts: model.BillingOperationMaxAttempts - 1, UpdatedAt: 1,
		}).Error)

		require.Error(t, RecoverBillingOperations(context.Background(), 100))

		var dead model.BillingOperation
		require.NoError(t, model.DB.First(&dead, "operation_key = ?", "poison:exhausted").Error)
		assert.Equal(t, model.BillingDead, dead.State)
		assert.Equal(t, model.BillingOperationMaxAttempts, dead.Attempts)
		assert.Contains(t, logs.String(), "dead letter", "the dead letter must be visible to operations")

		// 死信不再参与扫描：状态已经变了，下一轮不会再来一次。
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		var after model.BillingOperation
		require.NoError(t, model.DB.First(&after, "operation_key = ?", "poison:exhausted").Error)
		assert.Equal(t, model.BillingOperationMaxAttempts, after.Attempts)
	})
}
