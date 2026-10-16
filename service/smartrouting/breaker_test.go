package smartrouting

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
)

func TestHalfOpenInflightFullRejects(t *testing.T) {
	ResetHalfOpenInflightForTest()
	cfg := DefaultConfig()
	cfg.HalfOpenMaxInflight = 2
	SetConfig(cfg)

	id := 424242
	if HalfOpenInflightFull(id, 2) {
		t.Fatal("empty should not be full")
	}
	if !TryIncHalfOpenInflight(id, 2) {
		t.Fatal("inc 1")
	}
	if !TryIncHalfOpenInflight(id, 2) {
		t.Fatal("inc 2")
	}
	if !HalfOpenInflightFull(id, 2) {
		t.Fatal("expected full at 2")
	}
	if TryIncHalfOpenInflight(id, 2) {
		t.Fatal("inc 3 must fail")
	}
	DecHalfOpenInflight(id)
	if HalfOpenInflightFull(id, 2) {
		t.Fatal("after dec should have room")
	}
	if !TryIncHalfOpenInflight(id, 2) {
		t.Fatal("inc after dec")
	}
	ResetHalfOpenInflightForTest()
}

func TestApplyUserFailMeltCap(t *testing.T) {
	fail := 100.0
	uf := map[int]float64{1: 80, 2: 20} // user1 is 80% > 40%
	out := ApplyUserFailMeltCap(fail, uf, 0.40)
	// excess from user1 = 80 - 40 = 40 → fail 60
	if out < 59 || out > 61 {
		t.Fatalf("got %v want ~60", out)
	}
}


// 审查 R-P2-6：后台循环在跑时，快照失效后热路径不同步查库（DB=nil 也不会 panic），继续读旧快照。
func TestListHalfOpenReadyStaleDoesNotHitDB(t *testing.T) {
	prevDB := model.DB
	prevRunning := halfOpenLoopRunning.Load()
	t.Cleanup(func() {
		model.DB = prevDB
		halfOpenLoopRunning.Store(prevRunning)
		select {
		case <-halfOpenKick:
		default:
		}
	})
	model.DB = nil
	halfOpenLoopRunning.Store(true)
	halfOpenCacheMu.Lock()
	halfOpenCache, halfOpenCacheAt = []HalfOpenInfo{{ChannelID: 7, Stage: "half_2"}}, time.Now()
	halfOpenCacheMu.Unlock()
	InvalidateHalfOpenCache()
	list := ListHalfOpenReady()
	if len(list) != 1 || list[0].ChannelID != 7 {
		t.Fatalf("expected stale snapshot, got %v", list)
	}
	if !IsHalfOpenChannel(7) {
		t.Fatal("IsHalfOpenChannel must read snapshot")
	}
	select {
	case <-halfOpenKick:
	default:
		t.Fatal("refresh should have been kicked")
	}
}
