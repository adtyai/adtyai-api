package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateVideoTasksMissingChannelRefundsAndKeepsError(t *testing.T) {
	truncate(t)
	prevCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = prevCache })

	const userID, channelID = 771, 999001
	const charged = 4000
	seedUser(t, userID, 6000)
	task := makeTask(userID, channelID, charged, 0, BillingSourceWallet, 0)
	task.PrivateData.UpstreamTaskID = "upstream-missing-channel"
	require.NoError(t, model.DB.Create(task).Error)
	upstreamID := task.GetUpstreamTaskID()

	err := updateVideoTasks(context.Background(), constant.TaskPlatformSuno, channelID, []string{upstreamID}, map[string]*model.Task{upstreamID: task})
	require.Error(t, err)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, persisted.Status)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, 10000, getUserQuota(t, userID))
	assert.Contains(t, persisted.FailReason, "channel information")
}

func TestUpdateVideoTasksCacheMissDoesNotRefundExistingChannel(t *testing.T) {
	truncate(t)
	prevCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = prevCache })

	const userID, channelID = 772, 999002
	const charged = 4000
	seedUser(t, userID, 6000)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	require.NoError(t, model.InitChannelCache())
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return nil }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	channel := &model.Channel{Id: channelID, Name: "still-there", Key: "sk-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(channel).Error)
	task := makeTask(userID, channelID, charged, 0, BillingSourceWallet, 0)
	task.PrivateData.UpstreamTaskID = "upstream-cache-miss"
	require.NoError(t, model.DB.Create(task).Error)
	upstreamID := task.GetUpstreamTaskID()

	err := updateVideoTasks(context.Background(), constant.TaskPlatformSuno, channelID, []string{upstreamID}, map[string]*model.Task{upstreamID: task})
	require.Error(t, err)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusInProgress, persisted.Status)
	assert.Equal(t, charged, persisted.Quota)
	assert.Equal(t, 6000, getUserQuota(t, userID))
	assert.NotContains(t, persisted.FailReason, "channel information")
}

func TestClaimAndRefundRevertsWhenFundingRefundFails(t *testing.T) {
	truncate(t)
	task := makeTask(404404, 1, 100, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatusInProgress
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	assert.False(t, claimAndRefundUnfinishedTask(context.Background(), task, "Task is missing an upstream id"))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusInProgress, persisted.Status)
	assert.Equal(t, "20%", persisted.Progress)
	assert.Equal(t, 100, persisted.Quota)
	assert.Empty(t, persisted.FailReason)
}

func TestMissingMidjourneyChannelRefundsQuota(t *testing.T) {
	truncate(t)
	const userID, channelID = 772, 8801
	const charged = 900
	seedUser(t, userID, 1000)
	task := &model.Midjourney{
		UserId:           userID,
		ChannelId:        channelID,
		MjId:             "mj-missing-channel",
		Status:           "SUBMITTED",
		Progress:         "0%",
		Quota:            charged,
		AdtyWalletPaid:   charged,
		BillingChannelId: channelID,
	}
	require.NoError(t, task.Insert())

	kept := FailUnfinishedMidjourneyTasks(context.Background(), []*model.Midjourney{task}, missingChannelFailReason)
	assert.Equal(t, 1, kept)

	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, "FAILURE", persisted.Status)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, 1900, getUserQuota(t, userID))
}
