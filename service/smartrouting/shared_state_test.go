package smartrouting

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func TestFollowerLoadsPublishedScoresWhenLockMisses(t *testing.T) {
	cfg := GetConfig()
	prev := cfg
	cfg.Mode = "observe"
	SetConfig(cfg)
	prevLock := acquireScoreLockFn
	prevLoad := loadPublishedScoresFn
	acquireScoreLockFn = func(int) bool { return false }
	loadPublishedScoresFn = func() {
		storeScores(map[string]*ScoreEntry{
			scoreKey(9, "m"): {ChannelID: 9, Model: "m", Score: 88, RouteWeight: 40, State: "normal"},
		})
	}
	t.Cleanup(func() {
		SetConfig(prev)
		acquireScoreLockFn = prevLock
		loadPublishedScoresFn = prevLoad
		storeScores(map[string]*ScoreEntry{})
	})
	if !refreshScoresIfCollecting() && GetScore(9, "m") == nil {
		t.Fatal("follower must load published scores when it does not hold the refresh lock")
	}
	got := GetScore(9, "m")
	if got == nil || got.Score != 88 || got.RouteWeight != 40 {
		t.Fatalf("published score=%#v", got)
	}
}

func TestMergePublishedScoresKeepsWeightAndDoesNotWipeOnEmpty(t *testing.T) {
	storeScores(map[string]*ScoreEntry{
		scoreKey(3, "keep"): {ChannelID: 3, Model: "keep", Score: 70, RouteWeight: 5, State: "normal"},
	})
	t.Cleanup(func() { storeScores(map[string]*ScoreEntry{}) })
	merged := mergePublishedScores([]model.ChannelScoreSnapshot{{
		ChannelID: 8, Model: "m", Score: 91, SConn: 1, SPrice: 2, SSpeed: 3, SStab: 4, SAuth: 5,
		Samples: 12, UpdatedAt: time.Unix(99, 0),
	}}, []model.ChannelRouteWeight{{
		ChannelID: 8, Model: "m", RouteWeight: 33, State: "normal",
	}})
	if len(merged) != 1 || merged[scoreKey(8, "m")] == nil || merged[scoreKey(8, "m")].RouteWeight != 33 || merged[scoreKey(8, "m")].Score != 91 {
		t.Fatalf("merge=%#v", merged)
	}
	if got := applyPublishedScores(nil, nil); GetScore(3, "keep") == nil || GetScore(3, "keep").Score != 70 {
		t.Fatalf("empty snapshot must keep local scores, got %#v", got)
	}
}

func TestHalfOpenTickReloadsBreakerStages(t *testing.T) {
	prev := reloadBreakerStagesFn
	calls := 0
	reloadBreakerStagesFn = func() { calls++ }
	t.Cleanup(func() { reloadBreakerStagesFn = prev })
	halfOpenTick()
	if calls != 1 {
		t.Fatalf("half-open tick must reload breaker stages, calls=%d", calls)
	}
}

type fakeSharedCounter struct {
	n      int
	alive  bool
	incrOK bool
	del    int
	set    int
}

func (f *fakeSharedCounter) Incr(string, time.Duration) (int, bool) {
	if !f.incrOK {
		return 0, false
	}
	f.n++
	return f.n, true
}

func (f *fakeSharedCounter) Del(string) bool {
	f.del++
	f.n = 0
	return true
}

func (f *fakeSharedCounter) SetExpiry(string, time.Duration) bool {
	f.set++
	f.alive = true
	return true
}

func (f *fakeSharedCounter) Alive(string) (bool, bool) {
	if !f.incrOK && f.set == 0 {
		return false, false
	}
	return f.alive, true
}

func TestConsecFailUsesSharedCounter(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	cfg := GetConfig()
	prevCfg := cfg
	cfg.Mode = "active"
	cfg.ConsecutiveFailBreak = 8
	SetConfig(cfg)
	prevTrip := tripImmediateAsync
	prevCounter := routingCounter
	tripped := 0
	tripImmediateAsync = func(int, string) { tripped++ }
	fake := &fakeSharedCounter{n: 7, incrOK: true}
	routingCounter = fake
	id := 88091
	consecFailMu.Lock()
	prevN := consecFails[id]
	consecFails[id] = 0
	consecFailMu.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = prevRedis
		SetConfig(prevCfg)
		tripImmediateAsync = prevTrip
		routingCounter = prevCounter
		consecFailMu.Lock()
		consecFails[id] = prevN
		consecFailMu.Unlock()
		noteBreakerStage(id, "")
	})
	NoteAttemptResult(id, "m", false, ClassServer)
	if tripped != 1 || !breakerOpen(id) {
		t.Fatalf("shared counter at threshold must trip before return, tripped=%d open=%v", tripped, breakerOpen(id))
	}
}

func TestRateLimitCooldownReadsSharedStore(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	prevCounter := routingCounter
	fake := &fakeSharedCounter{alive: true, incrOK: true, set: 1}
	routingCounter = fake
	id := 88092
	key := rateLimitKey(id, "m")
	rateLimitMu.Lock()
	_, had := rateLimitUntil[key]
	delete(rateLimitUntil, key)
	rateLimitMu.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = prevRedis
		routingCounter = prevCounter
		rateLimitMu.Lock()
		if !had {
			delete(rateLimitUntil, key)
		}
		rateLimitMu.Unlock()
	})
	if !InRateLimitCooldown(id, "m") {
		t.Fatal("rate-limit cooldown must be visible from the shared store")
	}
}

func TestConsecFailMarksOpenBeforeAsyncTrip(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	cfg := GetConfig()
	prevCfg := cfg
	cfg.Mode = "active"
	cfg.ConsecutiveFailBreak = 1
	SetConfig(cfg)
	prevTrip := tripImmediateAsync
	prevCounter := routingCounter
	tripImmediateAsync = func(int, string) {}
	routingCounter = redisSharedCounter{}
	id := 88093
	consecFailMu.Lock()
	prevN := consecFails[id]
	consecFails[id] = 0
	consecFailMu.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = prevRedis
		SetConfig(prevCfg)
		tripImmediateAsync = prevTrip
		routingCounter = prevCounter
		consecFailMu.Lock()
		consecFails[id] = prevN
		consecFailMu.Unlock()
		noteBreakerStage(id, "")
	})
	NoteAttemptResult(id, "m", false, ClassServer)
	if !breakerOpen(id) {
		t.Fatal("this process must see open before the async trip returns")
	}
}
