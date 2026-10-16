package common

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

const (
	fxCacheTTL           = 10 * time.Minute
	fxHTTPTimeout        = 5 * time.Second
	fxFloatMin           = 0.10
	fxFloatMax           = 0.50
	fxDefaultFallback    = 7.3
	fxFrankfurterURL     = "https://api.frankfurter.app/latest?from=USD&to=CNY"
	fxOpenERAPIURL       = "https://open.er-api.com/v6/latest/USD"
)

// TopupFxSnapshot is the effective USD→CNY rate used for topup pricing display/orders.
type TopupFxSnapshot struct {
	BaseRate      float64   `json:"base_rate"`       // live mid-market before float
	FloatApplied  float64   `json:"float_applied"`   // upward float in [0.10, 0.50]
	EffectiveRate float64   `json:"effective_rate"`  // Round2(base + float)
	Source        string    `json:"source"`          // frankfurter | open_er_api | fallback | cache
	FetchedAt     time.Time `json:"fetched_at"`
	Stale         bool      `json:"stale"` // true when serving expired cache / fallback
}

type fxCache struct {
	mu     sync.RWMutex
	snap   TopupFxSnapshot
	expiry time.Time
	has    bool
}

var topupFxCache fxCache

var fxHTTPClient = &http.Client{Timeout: fxHTTPTimeout}

// RoundFX2 rounds a rate to 2 decimal places (half away from zero via math.Round).
func RoundFX2(v float64) float64 {
	return math.Round(v*100) / 100
}

// floatForBucket returns a deterministic upward float in [0.10, 0.50] (0.01 steps)
// keyed by TTL bucket so the float stays stable for the whole cache window.
func floatForBucket(bucket int64) float64 {
	if bucket < 0 {
		bucket = -bucket
	}
	steps := int64(math.Round((fxFloatMax - fxFloatMin) * 100)) // 40 → 0.10..0.50 inclusive = 41 values
	if steps < 0 {
		steps = 0
	}
	n := steps + 1
	return fxFloatMin + float64(bucket%n)/100.0
}

func currentFXBucket(now time.Time) int64 {
	return now.Unix() / int64(fxCacheTTL.Seconds())
}

// GetEffectiveTopupFxRate returns the live floated USD→CNY rate for topup CNY pricing.
// fallback is used when live fetch fails and there is no cached value (typically operation_setting.Price).
func GetEffectiveTopupFxRate(fallback float64) float64 {
	return GetTopupFxSnapshot(fallback).EffectiveRate
}

// GetTopupFxSnapshot returns the full snapshot (for status/tiers exposure).
func GetTopupFxSnapshot(fallback float64) TopupFxSnapshot {
	now := time.Now()
	topupFxCache.mu.RLock()
	if topupFxCache.has && now.Before(topupFxCache.expiry) {
		s := topupFxCache.snap
		topupFxCache.mu.RUnlock()
		return s
	}
	topupFxCache.mu.RUnlock()

	return refreshTopupFx(now, fallback)
}

func refreshTopupFx(now time.Time, fallback float64) TopupFxSnapshot {
	topupFxCache.mu.Lock()
	defer topupFxCache.mu.Unlock()
	// double-check after lock
	if topupFxCache.has && now.Before(topupFxCache.expiry) {
		return topupFxCache.snap
	}

	base, source, err := fetchLiveUsdCny()
	bucket := currentFXBucket(now)
	floatAmt := floatForBucket(bucket)

	if err == nil && base > 0 && !math.IsNaN(base) && !math.IsInf(base, 0) {
		eff := RoundFX2(base + floatAmt)
		snap := TopupFxSnapshot{
			BaseRate:      RoundFX2(base),
			FloatApplied:  RoundFX2(floatAmt),
			EffectiveRate: eff,
			Source:        source,
			FetchedAt:     now,
			Stale:         false,
		}
		topupFxCache.snap = snap
		topupFxCache.expiry = now.Add(fxCacheTTL)
		topupFxCache.has = true
		return snap
	}

	// Prefer last good cache even if expired
	if topupFxCache.has && topupFxCache.snap.EffectiveRate > 0 {
		s := topupFxCache.snap
		s.Stale = true
		s.Source = "cache"
		// keep expiry so we retry soon but don't stampede
		if topupFxCache.expiry.Before(now) {
			topupFxCache.expiry = now.Add(30 * time.Second)
		}
		topupFxCache.snap = s
		return s
	}

	fb := fallback
	if fb <= 0 || math.IsNaN(fb) || math.IsInf(fb, 0) {
		fb = fxDefaultFallback
	}
	snap := TopupFxSnapshot{
		BaseRate:      RoundFX2(fb),
		FloatApplied:  0,
		EffectiveRate: RoundFX2(fb),
		Source:        "fallback",
		FetchedAt:     now,
		Stale:         true,
	}
	topupFxCache.snap = snap
	topupFxCache.expiry = now.Add(30 * time.Second) // retry sooner when on fallback
	topupFxCache.has = true
	return snap
}

func fetchLiveUsdCny() (rate float64, source string, err error) {
	if r, e := fetchFrankfurter(); e == nil {
		return r, "frankfurter", nil
	} else {
		err = e
	}
	if r, e := fetchOpenERAPI(); e == nil {
		return r, "open_er_api", nil
	} else if err == nil {
		err = e
	} else {
		err = fmt.Errorf("%v; %v", err, e)
	}
	return 0, "", err
}

func fetchFrankfurter() (float64, error) {
	resp, err := fxHTTPClient.Get(fxFrankfurterURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("frankfurter status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	r, ok := parsed.Rates["CNY"]
	if !ok || r <= 0 {
		return 0, fmt.Errorf("frankfurter missing CNY")
	}
	return r, nil
}

func fetchOpenERAPI() (float64, error) {
	resp, err := fxHTTPClient.Get(fxOpenERAPIURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("open_er_api status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if parsed.Result != "" && parsed.Result != "success" {
		return 0, fmt.Errorf("open_er_api result %s", parsed.Result)
	}
	r, ok := parsed.Rates["CNY"]
	if !ok || r <= 0 {
		return 0, fmt.Errorf("open_er_api missing CNY")
	}
	return r, nil
}

// ResetTopupFxCacheForTest clears the FX cache (tests only).
func ResetTopupFxCacheForTest() {
	topupFxCache.mu.Lock()
	defer topupFxCache.mu.Unlock()
	topupFxCache.has = false
	topupFxCache.snap = TopupFxSnapshot{}
	topupFxCache.expiry = time.Time{}
}

// SetTopupFxSnapshotForTest injects a cache entry (tests only).
func SetTopupFxSnapshotForTest(snap TopupFxSnapshot, ttl time.Duration) {
	topupFxCache.mu.Lock()
	defer topupFxCache.mu.Unlock()
	topupFxCache.snap = snap
	topupFxCache.expiry = time.Now().Add(ttl)
	topupFxCache.has = true
}
