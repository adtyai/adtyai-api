package model

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// routeStateSnap is the hot-path copy of tags, cost guards and capabilities.
// A miss means "not excluded / capability unknown", never "exclude".
type routeStateSnap struct {
	loaded bool
	tags   map[string]struct{}
	cost   map[int]struct{}
	caps   map[int]ChannelCapability
}

var routeState atomic.Value

func routeTagKey(channelID int, modelName, tag string) string {
	if modelName == "" {
		modelName = "*"
	}
	return fmt.Sprintf("%d\x00%s\x00%s", channelID, modelName, tag)
}

// routeStateQuery is the reload seam. Tests stub it so a failed query cannot wipe the last snapshot.
var routeStateQuery = queryRouteState

// RefreshRouteState reloads tags, today's cost exclusions and capabilities.
// A failed reload keeps the last good snapshot. Other processes pick this up on their next score cycle.
func RefreshRouteState() {
	snap, err := routeStateQuery()
	if err != nil {
		common.SysError(common.LogText("smartrouting: route state reload failed: %v", err))
		return
	}
	routeState.Store(snap)
}

func queryRouteState() (*routeStateSnap, error) {
	if DB == nil {
		return nil, errors.New("database is not configured")
	}
	snap := &routeStateSnap{
		loaded: true,
		tags:   map[string]struct{}{},
		cost:   map[int]struct{}{},
		caps:   map[int]ChannelCapability{},
	}
	var tags []ChannelRouteTag
	if err := DB.Find(&tags).Error; err != nil {
		return nil, err
	}
	for _, t := range tags {
		modelName := t.Model
		if modelName == "" {
			modelName = "*"
		}
		snap.tags[routeTagKey(t.ChannelID, modelName, t.Tag)] = struct{}{}
	}
	day := time.Now().Format("2006-01-02")
	var guards []ChannelCostGuard
	if err := DB.Where("day = ? AND excluded = ?", day, true).Find(&guards).Error; err != nil {
		return nil, err
	}
	for _, g := range guards {
		snap.cost[g.ChannelID] = struct{}{}
	}
	var caps []ChannelCapability
	if err := DB.Find(&caps).Error; err != nil {
		return nil, err
	}
	for _, c := range caps {
		snap.caps[c.ChannelID] = c
	}
	return snap, nil
}

// ReplaceRouteStateForTest installs a snapshot without touching PostgreSQL.
func ReplaceRouteStateForTest(tags []ChannelRouteTag, costExcluded []int, caps []ChannelCapability) {
	snap := &routeStateSnap{
		loaded: true,
		tags:   map[string]struct{}{},
		cost:   map[int]struct{}{},
		caps:   map[int]ChannelCapability{},
	}
	for _, t := range tags {
		modelName := t.Model
		if modelName == "" {
			modelName = "*"
		}
		snap.tags[routeTagKey(t.ChannelID, modelName, t.Tag)] = struct{}{}
	}
	for _, id := range costExcluded {
		snap.cost[id] = struct{}{}
	}
	for _, c := range caps {
		snap.caps[c.ChannelID] = c
	}
	routeState.Store(snap)
}

func currentRouteState() *routeStateSnap {
	snap, _ := routeState.Load().(*routeStateSnap)
	return snap
}

// CachedHasRouteTag reads the memory snapshot. Unloaded or missing means false.
func CachedHasRouteTag(channelID int, modelName, tag string) bool {
	snap := currentRouteState()
	if snap == nil || !snap.loaded {
		return false
	}
	if modelName == "" {
		modelName = "*"
	}
	if _, ok := snap.tags[routeTagKey(channelID, modelName, tag)]; ok {
		return true
	}
	if modelName != "*" {
		_, ok := snap.tags[routeTagKey(channelID, "*", tag)]
		return ok
	}
	return false
}

// CachedCostGuardExcluded reads today's cost-guard snapshot. Miss means not excluded.
func CachedCostGuardExcluded(channelID int) bool {
	snap := currentRouteState()
	if snap == nil || !snap.loaded {
		return false
	}
	_, ok := snap.cost[channelID]
	return ok
}

// CachedCapability returns the snapshot row. ok is false when unknown.
func CachedCapability(channelID int) (ChannelCapability, bool) {
	snap := currentRouteState()
	if snap == nil || !snap.loaded {
		return ChannelCapability{}, false
	}
	c, ok := snap.caps[channelID]
	return c, ok
}
