package common

import (
	"math"
	"testing"
	"time"
)

func TestRoundFX2(t *testing.T) {
	if got := RoundFX2(7.345); got != 7.35 {
		t.Fatalf("RoundFX2(7.345)=%v want 7.35", got)
	}
	if got := RoundFX2(7.344); got != 7.34 {
		t.Fatalf("RoundFX2(7.344)=%v want 7.34", got)
	}
}

func TestFloatForBucketRange(t *testing.T) {
	seen := map[float64]bool{}
	for b := int64(0); b < 200; b++ {
		f := floatForBucket(b)
		if f < fxFloatMin-1e-9 || f > fxFloatMax+1e-9 {
			t.Fatalf("float %v out of [%v,%v] for bucket %d", f, fxFloatMin, fxFloatMax, b)
		}
		rounded := RoundFX2(f)
		if math.Abs(rounded-f) > 1e-9 {
			t.Fatalf("float %v not on 0.01 grid", f)
		}
		seen[f] = true
	}
	if len(seen) < 10 {
		t.Fatalf("expected diverse floats, got %d unique", len(seen))
	}
	// inclusive endpoints reachable
	if !seen[fxFloatMin] || !seen[fxFloatMax] {
		t.Fatalf("want endpoints %v and %v in set, got %v", fxFloatMin, fxFloatMax, seen)
	}
}

func TestGetEffectiveTopupFxRateUsesCache(t *testing.T) {
	ResetTopupFxCacheForTest()
	defer ResetTopupFxCacheForTest()
	SetTopupFxSnapshotForTest(TopupFxSnapshot{
		BaseRate: 7.12, FloatApplied: 0.30, EffectiveRate: 7.42,
		Source: "test", FetchedAt: time.Now(),
	}, time.Hour)
	got := GetEffectiveTopupFxRate(7.3)
	if got != 7.42 {
		t.Fatalf("got %v want 7.42", got)
	}
	snap := GetTopupFxSnapshot(7.3)
	if snap.Source != "test" || snap.EffectiveRate != 7.42 {
		t.Fatalf("unexpected snap %#v", snap)
	}
}

func TestFloatForBucketStableWithinWindow(t *testing.T) {
	b := currentFXBucket(time.Now())
	a, c := floatForBucket(b), floatForBucket(b)
	if a != c {
		t.Fatalf("same bucket must be stable: %v vs %v", a, c)
	}
}
