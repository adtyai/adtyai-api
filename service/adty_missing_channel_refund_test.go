package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestFailTasksWithoutChannelRefundsPrecharge(t *testing.T) {
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}, &model.Log{}, &model.Midjourney{}))

	const userID, tokenID, channelID = 71, 71, 71
	const startQuota, charged, tokenRemain = 10000, 3000, 5000
	seedUser(t, userID, startQuota)
	seedToken(t, tokenID, userID, "sk-missing-channel", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, charged, 1)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("quota", startQuota-charged).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{
		"remain_quota": tokenRemain - charged,
		"used_quota":   charged,
	}).Error)

	task := makeTask(userID, channelID, charged, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatusInProgress
	require.NoError(t, model.DB.Create(task).Error)

	FailTasksWithoutChannel(context.Background(), []*model.Task{task}, "")

	require.Equal(t, startQuota, getUserQuota(t, userID))
	require.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	stored := getTaskQuota(t, task.ID)
	require.Zero(t, stored)
	var row model.Task
	require.NoError(t, model.DB.First(&row, task.ID).Error)
	require.Equal(t, string(model.TaskStatusFailure), string(row.Status))
	require.Equal(t, "100%", row.Progress)

	mjUser := &model.User{Username: "mjmiss", Password: "x", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: startQuota - charged}
	require.NoError(t, model.DB.Create(mjUser).Error)
	mj := &model.Midjourney{UserId: mjUser.Id, MjId: "", Action: "IMAGINE", Status: "SUBMITTED", Progress: "0%", ChannelId: channelID, Quota: charged, TokenId: tokenID}
	require.NoError(t, model.DB.Create(mj).Error)
	FailMidjourneyTasksWithoutChannel(context.Background(), []*model.Midjourney{mj}, "mj_id is empty")
	require.Equal(t, startQuota, getUserQuota(t, mjUser.Id))
	var mjRow model.Midjourney
	require.NoError(t, model.DB.First(&mjRow, mj.Id).Error)
	require.Equal(t, "FAILURE", mjRow.Status)
	require.Zero(t, mjRow.Quota)
}
