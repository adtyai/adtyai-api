package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

// ErrChannelNotFound means the channel row is confirmed absent. A cache miss or
// a database error is not this error and must not be treated as deletion.
var ErrChannelNotFound = errors.New("channel no longer exists")

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
var abilityPriorityCache map[string]int64            // group|model|channel -> ability priority

func abilityPriorityKey(group, modelName string, channelID int) string {
	return group + "\x00" + modelName + "\x00" + strconv.Itoa(channelID)
}

// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*kitdto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

func InitChannelCache() error {
	if !common.MemoryCacheEnabled {
		InvalidatePricingCache()
		rebuildTaskAliasView()
		return nil
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*kitdto.AdvancedCustomConfig)
	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		return err
	}
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	if err := DB.Find(&abilities).Error; err != nil {
		return err
	}
	newAbilityPriority := make(map[string]int64, len(abilities))
	for _, ability := range abilities {
		if ability == nil || ability.Priority == nil {
			continue
		}
		newAbilityPriority[abilityPriorityKey(ability.Group, ability.Model, ability.ChannelId)] = *ability.Priority
	}
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.SplitSeq(channel.Group, ",")
		for group := range groups {
			models := channel.GetModels()
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	abilityPriorityCache = newAbilityPriority
	channelSyncLock.Unlock()
	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	rebuildTaskAliasView()
	common.SysLog(common.LogText("channels synced from database"))
	return nil
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog(common.LogText("syncing channels from database"))
		if err := InitChannelCache(); err != nil {
			common.SysError(common.LogText("failed to sync channels from database: %s", err.Error()))
		}
	}
}

func GetRandomSatisfiedChannel(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
) (*Channel, error) {
	ch, _, err := GetRandomSatisfiedChannelRouted(group, model, retry, filters, RouteCaps{})
	return ch, err
}

// GetRandomSatisfiedChannelRouted is the cache path with per-request caps and a decision record.
func GetRandomSatisfiedChannelRouted(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
	caps RouteCaps,
) (*Channel, RouteBandDecision, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelRouted(group, model, retry, filters, caps)
	}
	caps.Filters = filters
	ids, channels, missing, err := snapshotSatisfiedChannels(group, model, filters)
	if err != nil {
		return nil, RouteBandDecision{Group: group}, err
	}
	if len(ids) == 0 {
		// 审查 N-P1-2：全部熔断时回退到冷却到期的半开渠道，否则模型永久不可用
		// 审查 R-P1-1：不再要求 filters 为空，改为对回退渠道做 ChannelSatisfiesFilters 校验
		if retry == 0 {
			ch, rejected, fbErr := halfOpenFallback(model, group, caps, filters)
			if fbErr != nil || rejected {
				return nil, RouteBandDecision{Reason: "inflight_rejected", Group: group}, fbErr
			}
			if ch != nil {
				return ch, RouteBandDecision{Picked: ch.Id, HalfOpen: true, Reason: "halfopen", Group: group, BandIndex: BandIndexNotWalked}, nil
			}
		}
		return nil, RouteBandDecision{Reason: "empty", Group: group}, nil
	}
	if missing != 0 {
		return nil, RouteBandDecision{Group: group}, fmt.Errorf("database consistency error: channel #%d does not exist, please contact the administrator", missing)
	}

	if caps.PriorityOf == nil {
		caps.PriorityOf = func(channelID int) (int64, bool) {
			return cachedAbilityPriority(group, model, channelID)
		}
	}
	// 审查 N-P1-2：智能路由挂钩时单渠道也要走 ApplySmartWeights，才能注入半开渠道
	if len(channels) == 1 && SmartWeightHook == nil {
		ch := channels[0]
		return ch, RouteBandDecision{Picked: ch.Id, Priority: routedPriority(group, model, ch), Official: true, Reason: "single", Group: group}, nil
	}
	return SelectFromPriorityBands(groupChannelsByRoutedPriority(group, model, channels), retry, model, group, caps)
}

func routedPriority(group, modelName string, ch *Channel) int64 {
	if ch == nil {
		return 0
	}
	if p, ok := cachedAbilityPriority(group, modelName, ch.Id); ok {
		return p
	}
	return ch.GetPriority()
}

func cachedAbilityPriority(group, modelName string, channelID int) (int64, bool) {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if abilityPriorityCache == nil {
		return 0, false
	}
	p, ok := abilityPriorityCache[abilityPriorityKey(group, modelName, channelID)]
	return p, ok
}

func groupChannelsByRoutedPriority(group, modelName string, channels []*Channel) []PriorityBand {
	prioritySet := map[int64]struct{}{}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		prioritySet[routedPriority(group, modelName, channel)] = struct{}{}
	}
	priorities := make([]int64, 0, len(prioritySet))
	for priority := range prioritySet {
		priorities = append(priorities, priority)
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	bands := make([]PriorityBand, 0, len(priorities))
	for _, priority := range priorities {
		var band []*Channel
		for _, channel := range channels {
			if channel != nil && routedPriority(group, modelName, channel) == priority {
				band = append(band, channel)
			}
		}
		if len(band) > 0 {
			bands = append(bands, PriorityBand{Priority: priority, Channels: band})
		}
	}
	return bands
}

// BuildRoutedBands returns the same candidate bands the live selector uses.
// Cache-off reads abilities and does not fall back to channel.models.
func BuildRoutedBands(group, modelName string, filters []dto.ChannelFilter) ([]PriorityBand, RouteCaps, error) {
	caps := RouteCaps{Filters: filters}
	if common.MemoryCacheEnabled {
		caps.PriorityOf = func(channelID int) (int64, bool) {
			return cachedAbilityPriority(group, modelName, channelID)
		}
		ids, channels, missing, err := snapshotSatisfiedChannels(group, modelName, filters)
		if err != nil {
			return nil, caps, err
		}
		if missing != 0 {
			return nil, caps, fmt.Errorf("database consistency error: channel #%d does not exist, please contact the administrator", missing)
		}
		if len(ids) == 0 {
			return nil, caps, nil
		}
		return groupChannelsByRoutedPriority(group, modelName, channels), caps, nil
	}
	return buildDBRoutedBands(group, modelName, filters, RouteCaps{})
}

// snapshotSatisfiedChannels copies candidate pointers under the cache lock, then releases it.
// The returned pointers stay valid because cache rebuild replaces the map instead of mutating objects.
func snapshotSatisfiedChannels(group, modelName string, filters []dto.ChannelFilter) ([]int, []*Channel, int, error) {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	ids, _ := filterCandidateIDs(group2model2channels[group][modelName], modelName, filters)
	if len(ids) == 0 {
		normalized := ratio_setting.RoutingMatchModelName(modelName)
		ids, _ = filterCandidateIDs(group2model2channels[group][normalized], modelName, filters)
	}
	channels := make([]*Channel, 0, len(ids))
	for _, id := range ids {
		channel, ok := channelsIDM[id]
		if !ok || channel == nil {
			return ids, nil, id, nil
		}
		channels = append(channels, channel)
	}
	return ids, channels, 0, nil
}

// InstallChannelCacheForTest swaps the memory cache and restores it on cleanup.
// SetGroupModelChannelsForTest replaces one group/model candidate list without mutating the previous map.
// SetAbilityPriorityForTest records an ability priority the cache path should use.
func SetAbilityPriorityForTest(group, modelName string, channelID int, priority int64) {
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if abilityPriorityCache == nil {
		abilityPriorityCache = map[string]int64{}
	}
	abilityPriorityCache[abilityPriorityKey(group, modelName, channelID)] = priority
}

func SetGroupModelChannelsForTest(group, modelName string, ids []int) {
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	next := map[string]map[string][]int{}
	for g, models := range group2model2channels {
		copied := map[string][]int{}
		for m, list := range models {
			copied[m] = append([]int(nil), list...)
		}
		next[g] = copied
	}
	if next[group] == nil {
		next[group] = map[string][]int{}
	}
	next[group][modelName] = append([]int(nil), ids...)
	group2model2channels = next
}

func InstallChannelCacheForTest(channels []*Channel) func() {
	channelSyncLock.Lock()
	oldMap := channelsIDM
	oldGroups := group2model2channels
	oldEnabled := common.MemoryCacheEnabled
	oldWeight := SmartWeightHook
	oldSelected := OnChannelSelectedHook
	oldProbe := OnHalfOpenProbeHook
	oldHalf := HalfOpenFallbackHook
	oldAbility := abilityPriorityCache
	next := make(map[int]*Channel, len(channels))
	for _, ch := range channels {
		if ch != nil {
			next[ch.Id] = ch
		}
	}
	channelsIDM = next
	abilityPriorityCache = map[string]int64{}
	common.MemoryCacheEnabled = true
	channelSyncLock.Unlock()
	return func() {
		channelSyncLock.Lock()
		channelsIDM = oldMap
		group2model2channels = oldGroups
		abilityPriorityCache = oldAbility
		common.MemoryCacheEnabled = oldEnabled
		SmartWeightHook = oldWeight
		OnChannelSelectedHook = oldSelected
		OnHalfOpenProbeHook = oldProbe
		HalfOpenFallbackHook = oldHalf
		channelSyncLock.Unlock()
	}
}

// LookupChannel serves a cache hit and, on a miss, confirms the row in the
// database. Only a confirmed absence is ErrChannelNotFound. Transient database
// errors are returned unchanged so callers can retry instead of refunding.
func LookupChannel(id int) (*Channel, error) {
	if id <= 0 {
		return nil, fmt.Errorf("channel #%d no longer exists: %w", id, ErrChannelNotFound)
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		ch, ok := channelsIDM[id]
		channelSyncLock.RUnlock()
		if ok && ch != nil {
			return ch, nil
		}
	}
	ch, err := GetChannelById(id, true)
	if err == nil {
		if common.MemoryCacheEnabled {
			CacheUpdateChannel(ch)
		}
		return ch, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("channel #%d no longer exists: %w", id, ErrChannelNotFound)
	}
	return nil, err
}

// CacheGetChannelMemory returns a channel only from the in-memory map.
// A disabled cache or a miss is a miss. It never queries the database.
func CacheGetChannelMemory(id int) (*Channel, bool) {
	if !common.MemoryCacheEnabled || id <= 0 {
		return nil, false
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	c, ok := channelsIDM[id]
	if !ok || c == nil {
		return nil, false
	}
	return c, true
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("channel #%d no longer exists", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("channel #%d no longer exists", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*kitdto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}
