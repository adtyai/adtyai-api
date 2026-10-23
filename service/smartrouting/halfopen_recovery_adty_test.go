package smartrouting

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type recordedBreakerEvent struct {
	to     string
	reason string
	score  float64
}

func withHalfOpenStore(t *testing.T, store *model.ChannelBreakerState) (saved *[]*model.ChannelBreakerState, events *[]recordedBreakerEvent) {
	t.Helper()
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	prevLoad := loadBreakerStateFor
	prevSave := saveBreakerStateFor
	prevRec := recordBreakerEventFor
	prevScores := GetAllScores()
	savedRows := []*model.ChannelBreakerState{}
	saved = &savedRows
	eventRows := []recordedBreakerEvent{}
	events = &eventRows
	loadBreakerStateFor = func(id int) *model.ChannelBreakerState {
		cp := *store
		return &cp
	}
	saveBreakerStateFor = func(st *model.ChannelBreakerState) {
		if st == nil {
			return
		}
		cp := *st
		savedRows = append(savedRows, &cp)
		*store = cp
	}
	recordBreakerEventFor = func(channelID int, modelName, from, to, reason string, score float64) {
		eventRows = append(eventRows, recordedBreakerEvent{to: to, reason: reason, score: score})
	}
	t.Cleanup(func() {
		common.RedisEnabled = prevRedis
		loadBreakerStateFor = prevLoad
		saveBreakerStateFor = prevSave
		recordBreakerEventFor = prevRec
		storeScores(prevScores)
	})
	return saved, events
}

func elapsedHalf10(channelID int) *model.ChannelBreakerState {
	return &model.ChannelBreakerState{
		ChannelID:   channelID,
		Stage:       "half_10",
		Stage2Since: time.Now().Add(-2 * time.Second),
		CooldownSec: 120,
		HalfOpenOK:  3,
	}
}

func setStage2Window(t *testing.T) {
	t.Helper()
	prev := GetConfig()
	cfg := prev
	cfg.ScoreRefreshSec = 1
	cfg.HalfOpenStage2Stable = 1
	SetConfig(cfg)
	t.Cleanup(func() { SetConfig(prev) })
}

func TestHalfOpenStage2StaysWithoutScore(t *testing.T) {
	setStage2Window(t)
	store := elapsedHalf10(42)
	_, events := withHalfOpenStore(t, store)
	storeScores(map[string]*ScoreEntry{})

	OnHalfOpenResult(42, true)
	if store.Stage != "half_10" {
		t.Fatalf("missing score must stay half_10, got %s", store.Stage)
	}
	for _, ev := range *events {
		if ev.to == "closed" || ev.reason == "recovered" {
			t.Fatalf("missing score must not record recovery, event=%+v", ev)
		}
	}
}

func TestHalfOpenStage2StaysWhenScoreInsufficient(t *testing.T) {
	setStage2Window(t)
	store := elapsedHalf10(42)
	_, events := withHalfOpenStore(t, store)
	storeScores(map[string]*ScoreEntry{
		"42|m": {ChannelID: 42, Model: "m", Score: 90, Insufficient: true},
	})

	OnHalfOpenResult(42, true)
	if store.Stage != "half_10" {
		t.Fatalf("insufficient score must stay half_10, got %s", store.Stage)
	}
	for _, ev := range *events {
		if ev.reason == "recovered" {
			t.Fatalf("insufficient score must not record recovery, event=%+v", ev)
		}
	}
}

func TestHalfOpenStage2ClosesAtScore65(t *testing.T) {
	setStage2Window(t)
	ResetHalfOpenInflightForTest()
	t.Cleanup(ResetHalfOpenInflightForTest)
	if !TryIncHalfOpenInflight(42, 2) {
		t.Fatal("seed slot")
	}
	store := elapsedHalf10(42)
	_, events := withHalfOpenStore(t, store)
	storeScores(map[string]*ScoreEntry{
		"42|m": {ChannelID: 42, Model: "m", Score: 65, Insufficient: false},
	})

	OnHalfOpenResult(42, true)
	if store.Stage != "closed" {
		t.Fatalf("score 65 must close, got %s", store.Stage)
	}
	if got := halfOpenInflightCount(42); got != 0 {
		t.Fatalf("closing must clear leftover probe slots, count=%d", got)
	}
	if len(*events) != 1 || (*events)[0].reason != "recovered" || (*events)[0].score != 65 {
		t.Fatalf("recovery must record the real score, events=%+v", *events)
	}
}

func TestHalfOpenStage2PromotesMidScoreToFull(t *testing.T) {
	setStage2Window(t)
	store := elapsedHalf10(42)
	_, events := withHalfOpenStore(t, store)
	storeScores(map[string]*ScoreEntry{
		"42|m": {ChannelID: 42, Model: "m", Score: 64, Insufficient: false},
	})

	OnHalfOpenResult(42, true)
	if store.Stage != "full" {
		t.Fatalf("score 64 must enter full, got %s", store.Stage)
	}
	for _, ev := range *events {
		if ev.reason == "recovered" {
			t.Fatalf("score 64 must not close the channel, event=%+v", ev)
		}
	}
}

func TestHalfOpenStage2StaysWhenWorstScoreIsLow(t *testing.T) {
	setStage2Window(t)
	store := elapsedHalf10(42)
	_, events := withHalfOpenStore(t, store)
	storeScores(map[string]*ScoreEntry{
		"42|good": {ChannelID: 42, Model: "good", Score: 90, Insufficient: false},
		"42|bad":  {ChannelID: 42, Model: "bad", Score: 40, Insufficient: false},
	})

	OnHalfOpenResult(42, true)
	if store.Stage != "half_10" {
		t.Fatalf("worst sufficient score must stay half_10, got %s", store.Stage)
	}
	for _, ev := range *events {
		if ev.reason == "recovered" {
			t.Fatalf("best score must not close the channel, event=%+v", ev)
		}
	}
}

func TestHalfOpenFailureKeepsOtherProbeSlots(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	ResetHalfOpenInflightForTest()
	t.Cleanup(ResetHalfOpenInflightForTest)
	if !TryIncHalfOpenInflight(42, 2) {
		t.Fatal("seed other probe slot")
	}

	store := &model.ChannelBreakerState{ChannelID: 42, Stage: "half_2", CooldownSec: 120, HalfOpenOK: 2}
	_, events := withHalfOpenStore(t, store)
	OnHalfOpenResult(42, false)

	if got := halfOpenInflightCount(42); got != 1 {
		t.Fatalf("failure must keep other in-flight probe slots, count=%d", got)
	}
	if store.Stage != "open" {
		t.Fatalf("failure must still reopen, got %s", store.Stage)
	}
	if store.HalfOpenOK != 0 {
		t.Fatalf("failure must still clear the ok count, got %d", store.HalfOpenOK)
	}
	reopened := false
	for _, ev := range *events {
		if ev.to == "open" && ev.reason == "halfopen_fail" {
			reopened = true
		}
	}
	if !reopened {
		t.Fatal("failure must still record the reopen")
	}
}
