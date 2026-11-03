package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInitChannelCacheKeepsRoutesWhenQueryFails(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	channel := &Channel{
		Name: "cached-route", Type: 1, Key: "sk-cache", Status: common.ChannelStatusEnabled,
		Group: "default", Models: "gpt-test",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group: "default", Model: "gpt-test", ChannelId: channel.Id, Enabled: true,
	}).Error)

	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousCache
		channelSyncLock.Lock()
		channelsIDM = nil
		group2model2channels = nil
		channelSyncLock.Unlock()
	})

	InitChannelCache()
	channelSyncLock.RLock()
	_, present := channelsIDM[channel.Id]
	channelSyncLock.RUnlock()
	require.True(t, present)

	broken := db.Session(&gorm.Session{NewDB: true})
	require.NoError(t, broken.Callback().Query().Before("gorm:query").Register("adty-fail-query", func(tx *gorm.DB) {
		_ = tx.AddError(errors.New("database unavailable"))
	}))
	previousDB := DB
	DB = broken
	t.Cleanup(func() { DB = previousDB })

	InitChannelCache()

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	require.Contains(t, channelsIDM, channel.Id)
	require.Equal(t, []int{channel.Id}, group2model2channels["default"]["gpt-test"])
}
