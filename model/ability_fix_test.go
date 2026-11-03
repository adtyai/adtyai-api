package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFixAbilityKeepsRowsWhenRebuildFails(t *testing.T) {
	truncateTables(t)
	channel := &Channel{
		Name: "fix-ability", Type: 1, Key: "sk-fix", Status: common.ChannelStatusEnabled,
		Group: "default", Models: "alpha",
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "alpha", ChannelId: channel.Id, Enabled: true,
	}).Error)

	var before int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND model = ?", channel.Id, "alpha").Count(&before).Error)
	require.EqualValues(t, 1, before)

	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("test:fail_fix_ability", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			_ = tx.AddError(errors.New("ability rebuild failed"))
		}
	}))
	t.Cleanup(func() {
		DB.Callback().Create().Remove("test:fail_fix_ability")
	})

	_, fails, err := FixAbility()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, fails, 1)

	var after int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND model = ?", channel.Id, "alpha").Count(&after).Error)
	assert.EqualValues(t, 1, after)
}

func TestInitChannelCacheAllowsGroupMissingFromAbilities(t *testing.T) {
	truncateTables(t)
	previous := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previous
		channelSyncLock.Lock()
		channelsIDM = nil
		group2model2channels = nil
		channelSyncLock.Unlock()
	})

	channel := &Channel{
		Name: "missing-group", Type: 1, Key: "sk-missing", Status: common.ChannelStatusEnabled,
		Group: "vip", Models: "solo-model",
	}
	require.NoError(t, DB.Create(channel).Error)

	require.NotPanics(t, func() { InitChannelCache() })

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	require.Contains(t, channelsIDM, channel.Id)
	assert.Equal(t, []int{channel.Id}, group2model2channels["vip"]["solo-model"])
}

func TestSyncChannelCacheOnceKeepsRoutesAfterPanic(t *testing.T) {
	previousDB := DB
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	kept := &Channel{Id: 7, Name: "kept"}
	channelSyncLock.Lock()
	channelsIDM = map[int]*Channel{kept.Id: kept}
	group2model2channels = map[string]map[string][]int{"default": {"kept-model": {kept.Id}}}
	channelSyncLock.Unlock()
	DB = nil
	t.Cleanup(func() {
		DB = previousDB
		common.MemoryCacheEnabled = previousCache
		channelSyncLock.Lock()
		channelsIDM = nil
		group2model2channels = nil
		channelSyncLock.Unlock()
	})

	require.NotPanics(t, syncChannelCacheOnce)

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	require.Contains(t, channelsIDM, kept.Id)
	assert.Equal(t, []int{kept.Id}, group2model2channels["default"]["kept-model"])
}
