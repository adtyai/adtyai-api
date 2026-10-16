package smartrouting

import "testing"

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

