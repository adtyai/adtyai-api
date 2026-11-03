package controller

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSlowUserCacheFailureDoesNotCrashChannelTest(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.User{}))
	previous := common.AutomaticDisableChannelEnabled
	previousRedis := common.RedisEnabled
	common.AutomaticDisableChannelEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = previous
		common.RedisEnabled = previousRedis
	})

	autoBan := 1
	channel := &model.Channel{Name: "slow-cache", Type: 1, Status: common.ChannelStatusEnabled, AutoBan: &autoBan, Key: "sk-test"}
	require.NoError(t, db.Create(channel).Error)

	var summary channelTestSummary
	require.NotPanics(t, func() {
		summary = testChannelForHealthCheck(context.Background(), channel, 0, true, -1)
	})
	require.Equal(t, 0, summary.Disabled)
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, stored.Status)
}
