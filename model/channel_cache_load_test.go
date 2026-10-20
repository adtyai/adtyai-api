package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInitChannelCacheKeepsPreviousMapWhenLoadFails(t *testing.T) {
	prevCache := common.MemoryCacheEnabled
	prevDB := DB
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = prevCache
		DB = prevDB
	})

	good, err := gorm.Open(sqlite.Open("file:channel-cache-good?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, good.AutoMigrate(&Channel{}, &Ability{}))
	require.NoError(t, good.Create(&Channel{Id: 42, Name: "kept", Key: "sk-test", Status: common.ChannelStatusEnabled}).Error)
	DB = good
	require.NoError(t, InitChannelCache())

	bad, err := gorm.Open(sqlite.Open("file:channel-cache-bad?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := bad.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB = bad
	require.Error(t, InitChannelCache())

	DB = good
	ch, err := CacheGetChannel(42)
	require.NoError(t, err)
	require.Equal(t, "kept", ch.Name)
}

func TestLookupChannelConfirmsCacheMissBeforeNotFound(t *testing.T) {
	prevCache := common.MemoryCacheEnabled
	prevDB := DB
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = prevCache
		DB = prevDB
	})

	db, err := gorm.Open(sqlite.Open("file:channel-cache-lookup?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	require.NoError(t, db.Create(&Channel{Id: 7, Name: "present", Key: "sk-test", Status: common.ChannelStatusEnabled}).Error)
	DB = db
	require.NoError(t, InitChannelCache())
	channelSyncLock.Lock()
	channelsIDM = map[int]*Channel{}
	channelSyncLock.Unlock()

	ch, err := LookupChannel(7)
	require.NoError(t, err)
	require.Equal(t, "present", ch.Name)

	require.NoError(t, db.Delete(&Channel{}, 7).Error)
	channelSyncLock.Lock()
	channelsIDM = map[int]*Channel{}
	channelSyncLock.Unlock()
	_, err = LookupChannel(7)
	require.ErrorIs(t, err, ErrChannelNotFound)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = LookupChannel(7)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrChannelNotFound)
}
