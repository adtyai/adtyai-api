package model

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSelectorReleasesCacheLockBeforeWeightHook(t *testing.T) {
	ch := &Channel{Id: 1, Weight: bandWeight(1)}
	withChannelCache(t, []*Channel{ch}, "g", "m")
	var called atomic.Bool
	var lockedInside atomic.Bool
	hookEntered := make(chan struct{})
	var once sync.Once
	writerDone := make(chan struct{})
	SmartWeightHook = func(channels []*Channel, modelName, group string, caps RouteCaps) SmartWeightResult {
		called.Store(true)
		once.Do(func() { close(hookEntered) })
		time.Sleep(100 * time.Millisecond)
		select {
		case <-writerDone:
		default:
			lockedInside.Store(true)
		}
		return SmartWeightResult{Channels: channels, Weights: []int{1}}
	}
	go func() {
		<-hookEntered
		channelSyncLock.Lock()
		channelSyncLock.Unlock()
		close(writerDone)
	}()
	_, _, _ = GetRandomSatisfiedChannelRouted("g", "m", 0, nil, RouteCaps{})
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("writer still blocked; selector held the cache lock")
	}
	if !called.Load() {
		t.Fatal("weight hook was not called")
	}
	if lockedInside.Load() {
		t.Fatal("writer was still blocked while the weight hook ran")
	}
}
