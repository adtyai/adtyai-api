package model

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
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

func TestChannelCacheFailureDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := isolatedMigrationTestDatabase(t, dialect)
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			previousCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
			common.MemoryCacheEnabled, common.RedisEnabled = true, false
			t.Cleanup(func() {
				common.MemoryCacheEnabled, common.RedisEnabled = previousCache, previousRedis
				channelSyncLock.Lock()
				channelsIDM, group2model2channels = nil, nil
				channel2advancedCustomConfig = nil
				channelCacheInvalid = false
				channelSyncLock.Unlock()
			})
			for _, scenario := range []string{"background failure", "stale reload", "status ability failure", "single key status", "constraint failure", "poll during reload", "polling recovery"} {
				t.Run(scenario, func(t *testing.T) {
					channel := &Channel{
						Name: scenario, Type: constant.ChannelTypeOpenAI, Key: "sk-first\nsk-second", Models: "cache-model",
						Group: "default", Status: common.ChannelStatusEnabled,
						ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeySize: 2},
					}
					require.NoError(t, channel.Insert())
					t.Cleanup(func() {
						require.NoError(t, channel.Delete())
						require.NoError(t, InitChannelCache())
					})
					require.NoError(t, InitChannelCache())
					switch scenario {
					case "background failure":
						require.NoError(t, db.Callback().Query().Before("gorm:query").Register("cache-background-failure", func(tx *gorm.DB) {
							_ = tx.AddError(errors.New("database unavailable"))
						}))
						t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("cache-background-failure")) })
						require.Error(t, InitChannelCache())
						selected, err := GetRandomSatisfiedChannel("default", "cache-model", 0, nil)
						require.NoError(t, err)
						require.NotNil(t, selected, "a background outage must not erase an unchanged valid route")
						assert.Equal(t, channel.Id, selected.Id)
					case "stale reload":
						loaded, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
						var paused atomic.Bool
						require.NoError(t, db.Callback().Query().After("gorm:query").Register("cache-pause-old-snapshot", func(tx *gorm.DB) {
							if _, isChannels := tx.Statement.Dest.(*[]*Channel); isChannels {
								if paused.CompareAndSwap(false, true) {
									close(loaded)
									<-release
								}
							}
						}))
						t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("cache-pause-old-snapshot")) })
						go func() { finished <- InitChannelCache() }()
						select {
						case <-loaded:
						case <-time.After(5 * time.Second):
							t.Fatal("background reload did not reach its snapshot barrier")
						}
						require.NoError(t, channel.Delete())
						require.NoError(t, db.Callback().Query().Before("gorm:query").Register("cache-fail-new-snapshot", func(tx *gorm.DB) {
							if _, isChannels := tx.Statement.Dest.(*[]*Channel); isChannels {
								_ = tx.AddError(errors.New("reload after deletion unavailable"))
							}
						}))
						err := RefreshChannelCache()
						close(release)
						require.Error(t, err)
						select {
						case <-finished:
						case <-time.After(5 * time.Second):
							t.Fatal("old reload did not finish")
						}
						require.NoError(t, db.Callback().Query().Remove("cache-fail-new-snapshot"))
						selected, err := GetRandomSatisfiedChannel("default", "cache-model", 0, nil)
						require.NoError(t, err)
						assert.Nil(t, selected, "an older in-flight snapshot cannot resurrect a deleted channel")
						_, err = CacheGetChannel(channel.Id)
						assert.Error(t, err)
					case "status ability failure", "single key status":
						previousSkip := db.Config.SkipDefaultTransaction
						db.Config.SkipDefaultTransaction = true
						t.Cleanup(func() { db.Config.SkipDefaultTransaction = previousSkip })
						require.NoError(t, db.Callback().Update().Before("gorm:update").Register("cache-fail-ability-status", func(tx *gorm.DB) {
							if tx.Statement.Table == "abilities" {
								_ = tx.AddError(errors.New("ability update unavailable"))
								return
							}
							if tx.Statement.Table == "channels" {
								_ = InitChannelCache() // Capture the old DB after the early cache mutation.
							}
						}))
						t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("cache-fail-ability-status")) })
						if scenario == "single key status" {
							require.True(t, UpdateChannelStatus(channel.Id, "sk-first", common.ChannelStatusAutoDisabled, "upstream rejected key"))
							selected, err := CacheGetChannel(channel.Id)
							require.NoError(t, err)
							actual, _, apiErr := selected.GetNextEnabledKey()
							require.Nil(t, apiErr)
							assert.Equal(t, "sk-second", actual, "an old reload cannot revive a disabled individual key")
							break
						}
						require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual operation"))
						stored, err := GetChannelById(channel.Id, true)
						require.NoError(t, err)
						require.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
						selected, err := GetRandomSatisfiedChannel("default", "cache-model", 0, nil)
						require.NoError(t, err)
						assert.Nil(t, selected)
						assert.False(t, IsChannelEnabledForGroupModel("default", "cache-model", channel.Id))
					case "constraint failure":
						common.MemoryCacheEnabled = false
						t.Cleanup(func() { common.MemoryCacheEnabled = true })
						require.NoError(t, db.Callback().Query().Before("gorm:query").Register("cache-fail-constraint-read", func(tx *gorm.DB) {
							if _, isChannels := tx.Statement.Dest.(*[]*Channel); isChannels {
								_ = tx.AddError(errors.New("constraint lookup unavailable"))
							}
						}))
						t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("cache-fail-constraint-read")) })
						selected, _ := GetRandomSatisfiedChannel("default", "cache-model", 0, []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/responses"}})
						assert.Nil(t, selected, "a failed constraint lookup cannot allow a route")
					case "poll during reload":
						var polled atomic.Bool
						require.NoError(t, db.Callback().Query().After("gorm:query").Register("cache-poll-during-reload", func(tx *gorm.DB) {
							if _, isChannels := tx.Statement.Dest.(*[]*Channel); isChannels && polled.CompareAndSwap(false, true) {
								selected, err := CacheGetChannel(channel.Id)
								require.NoError(t, err)
								actual, _, apiErr := selected.GetNextEnabledKey()
								require.Nil(t, apiErr)
								assert.Equal(t, "sk-first", actual)
							}
						}))
						t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("cache-poll-during-reload")) })
						require.NoError(t, InitChannelCache(), "ordinary polling cannot prevent configuration reload")
						selected, err := CacheGetChannel(channel.Id)
						require.NoError(t, err)
						actual, _, apiErr := selected.GetNextEnabledKey()
						require.Nil(t, apiErr)
						assert.Equal(t, "sk-second", actual)
					case "polling recovery":
						failReload := true
						require.NoError(t, db.Callback().Query().Before("gorm:query").Register("cache-fail-polling-reload", func(tx *gorm.DB) {
							if _, hasWhere := tx.Statement.Clauses["WHERE"]; failReload && !hasWhere && tx.Statement.Table == "channels" {
								_ = tx.AddError(errors.New("cache unavailable"))
							}
						}))
						require.Error(t, RefreshChannelCache())
						for i, key := range []string{"sk-first", "sk-second", "sk-first"} {
							selected, err := GetRandomSatisfiedChannel("default", "cache-model", 0, nil)
							require.NoError(t, err)
							require.NotNil(t, selected)
							if i == 1 {
								var recovered atomic.Bool
								require.NoError(t, db.Callback().Query().After("gorm:query").Register("cache-recover-during-poll", func(tx *gorm.DB) {
									if _, isChannel := tx.Statement.Dest.(*Channel); isChannel && recovered.CompareAndSwap(false, true) {
										failReload = false
										require.NoError(t, InitChannelCache())
									}
								}))
								t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("cache-recover-during-poll")) })
							}
							actual, _, apiErr := selected.GetNextEnabledKey()
							require.Nil(t, apiErr)
							assert.Equal(t, key, actual)
						}
						require.NoError(t, db.Callback().Query().Remove("cache-fail-polling-reload"))
						require.NoError(t, InitChannelCache())
						selected, err := CacheGetChannel(channel.Id)
						require.NoError(t, err)
						actual, _, apiErr := selected.GetNextEnabledKey()
						require.Nil(t, apiErr)
						assert.Equal(t, "sk-second", actual, "recovery retains the cursor advanced in the database")
					}
				})
			}
		})
	}
}
