package smartrouting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

var (
	retryBudgetMu sync.Mutex
	retryBudget   = map[string]struct{ total, retry int }{} // minute key
)

// OnAttemptEnd is the relay hook: record sample, cost, breaker, half-open, auth.
func OnAttemptEnd(c *gin.Context, ev AttemptEvent) {
	cfg := GetConfig()
	if !cfg.Enabled {
		return
	}
	if ev.Model == "" && ev.ReqModel != "" {
		ev.Model = ev.ReqModel
	}
	RecordSample(ev)
	IncRPM(ev.ChannelID)

	if IsHalfOpenChannel(ev.ChannelID) {
		DecHalfOpenInflight(ev.ChannelID)
		OnHalfOpenResult(ev.ChannelID, ev.Success)
	}

	NoteAttemptResult(ev.ChannelID, ev.Success, ev.Class)

	if !ev.CacheHit {
		WriteCostLedger(ev)
	}
	if ev.Success {
		CheckPassiveAuth(ev)
	}
}

// OnAttemptEndFromError builds event from NewAPIError.
func OnAttemptEndFromError(c *gin.Context, channelID int, modelName string, userID, tokenID int, requestID string, latencyMs int64, err *types.NewAPIError, pinned bool) {
	class := Classify(err)
	OnAttemptEnd(c, AttemptEvent{
		ChannelID: channelID,
		Model:     modelName,
		UserID:    userID,
		TokenID:   tokenID,
		RequestID: requestID,
		Success:   false,
		Class:     class,
		LatencyMs: latencyMs,
		Pinned:    pinned,
	})
}

// OnAttemptEndSuccess records a successful attempt.
func OnAttemptEndSuccess(c *gin.Context, ev AttemptEvent) {
	ev.Success = true
	ev.Class = ClassOK
	OnAttemptEnd(c, ev)
}

// RetryBudgetAllows returns false when global retry share exceeds budget (Envoy-style).
func RetryBudgetAllows() bool {
	cfg := GetConfig()
	pct := cfg.RetryBudgetPct
	if pct <= 0 {
		return true
	}
	ctx := context.Background()
	now, _ := RedisTIME(ctx)
	key := fmt.Sprintf("sr:retrybudget:%s", minuteFloor(now).Format("200601021504"))
	retryBudgetMu.Lock()
	defer retryBudgetMu.Unlock()
	if common.RedisEnabled && common.RDB != nil {
		// use hash total/retry
		pipe := common.RDB.Pipeline()
		tCmd := pipe.HIncrBy(ctx, key, "total", 1)
		pipe.Expire(ctx, key, 2*time.Minute)
		_, _ = pipe.Exec(ctx)
		total, _ := tCmd.Result()
		retry, _ := common.RDB.HGet(ctx, key, "retry").Int64()
		if total > 20 && float64(retry)/float64(total)*100 > float64(pct) {
			return false
		}
		return true
	}
	b := retryBudget[key]
	b.total++
	retryBudget[key] = b
	if b.total > 20 && float64(b.retry)/float64(b.total)*100 > float64(pct) {
		return false
	}
	return true
}

func MarkRetryUsed() {
	ctx := context.Background()
	now, _ := RedisTIME(ctx)
	key := fmt.Sprintf("sr:retrybudget:%s", minuteFloor(now).Format("200601021504"))
	if common.RedisEnabled && common.RDB != nil {
		common.RDB.HIncrBy(ctx, key, "retry", 1)
		common.RDB.Expire(ctx, key, 2*time.Minute)
		return
	}
	retryBudgetMu.Lock()
	b := retryBudget[key]
	b.retry++
	retryBudget[key] = b
	retryBudgetMu.Unlock()
}

// WaitRetryAfter sleeps up to min(Retry-After, 2s) for same-channel 429 (总管①).
func WaitRetryAfter(retryAfterSec int) time.Duration {
	cfg := GetConfig()
	maxMs := cfg.RetryAfterMaxWaitMs
	if maxMs <= 0 {
		maxMs = 2000
	}
	ms := retryAfterSec * 1000
	if ms > maxMs {
		ms = maxMs
	}
	if ms <= 0 {
		return 0
	}
	d := time.Duration(ms) * time.Millisecond
	time.Sleep(d)
	return d
}
