package smartrouting

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// OnAttemptEnd is the relay hook: record sample, cost, breaker, half-open, auth.
func OnAttemptEnd(c *gin.Context, ev AttemptEvent) {
	releaseHalfOpenReservation(c, true, ev.Success, ev.Class)
	if !Collects() {
		return
	}
	if ev.Model == "" && ev.ReqModel != "" {
		ev.Model = ev.ReqModel
	}
	RecordSample(ev)
	IncRPM(ev.ChannelID)
	NoteAttemptResult(ev.ChannelID, ev.Model, ev.Success, ev.Class)

	if !ev.CacheHit {
		WriteCostLedger(ev)
	}
	if ev.Success {
		CheckPassiveAuth(ev)
	}
}

// ReleaseHalfOpenReservation frees a slot reserved by this request. It does not report a probe result.
func ReleaseHalfOpenReservation(c *gin.Context) {
	releaseHalfOpenReservation(c, false, false, ClassOK)
}

var reportHalfOpenResult = OnHalfOpenResult

func halfOpenProbeResultCounts(success bool, class ErrClass) bool {
	if success {
		return true
	}
	switch class {
	case ClassUserError, ClassAuthMismatch, ClassBalance:
		return false
	default:
		return true
	}
}

func releaseHalfOpenReservation(c *gin.Context, report, success bool, class ErrClass) {
	if c == nil {
		return
	}
	id := common.GetContextKeyInt(c, constant.ContextKeyHalfOpenReserved)
	if id == 0 {
		return
	}
	common.SetContextKey(c, constant.ContextKeyHalfOpenReserved, 0)
	DecHalfOpenInflight(id)
	if report && IsHalfOpenChannel(id) && halfOpenProbeResultCounts(success, class) {
		reportHalfOpenResult(id, success)
	}
}

// NoteSelectedAttempt records one attempt only after a channel was selected.
// SkipRetry errors are user errors so they do not count as a channel failure.
func NoteSelectedAttempt(c *gin.Context, channelID int, modelName string, err *types.NewAPIError, latencyMs int64) {
	if c == nil || channelID == 0 {
		return
	}
	class := ClassForRecordedAttempt(err)
	OnAttemptEnd(c, AttemptEvent{
		ChannelID: channelID,
		Model:     modelName,
		Success:   err == nil,
		Class:     class,
		LatencyMs: latencyMs,
	})
}

// OnAttemptEndFromError builds event from NewAPIError.
func OnAttemptEndFromError(c *gin.Context, channelID int, modelName string, userID, tokenID int, requestID string, latencyMs int64, err *types.NewAPIError, pinned bool) {
	class := ClassForRecordedAttempt(err)
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
