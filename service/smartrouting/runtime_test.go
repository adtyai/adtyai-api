package smartrouting

import "testing"

func TestObserveRefreshesBeforeFirstSleep(t *testing.T) {
	prev := GetConfig()
	cfg := prev
	cfg.Mode = "observe"
	cfg.Enabled = false
	cfg.ScoreRefreshSec = 3600
	SetConfig(cfg)
	prevRefresh := refreshAllScores
	called := 0
	refreshAllScores = func() { called++ }
	t.Cleanup(func() {
		SetConfig(prev)
		refreshAllScores = prevRefresh
	})
	stop := make(chan struct{})
	close(stop)
	scoreLoopUntil(stop)
	if called != 1 {
		t.Fatalf("observe must refresh once before waiting, called=%d", called)
	}
}
