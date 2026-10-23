package smartrouting

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func TestClassifyStatusCodes(t *testing.T) {
	mk := func(code int, msg string) *types.NewAPIError {
		return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeBadResponseStatusCode, code)
	}
	if Classify(mk(400, "bad")) != ClassUserError {
		t.Fatal("400")
	}
	if Classify(mk(401, "unauthorized")) != ClassBanned {
		t.Fatal("401")
	}
	if Classify(mk(429, "rate")) != ClassRateLimit {
		t.Fatal("429")
	}
	if Classify(mk(500, "boom")) != ClassServer {
		t.Fatal("500")
	}
	if Classify(mk(402, "insufficient_quota")) != ClassBalance {
		t.Fatal("402")
	}
}

func TestRecordedLocalSkipRetryIsUserError(t *testing.T) {
	cases := []struct {
		name string
		err  *types.NewAPIError
	}{
		{
			name: "copy request",
			err:  types.NewError(errors.New("failed to copy request to GeneralOpenAIRequest: boom"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry()),
		},
		{
			name: "model map",
			err:  types.NewError(errors.New("model mapping failed"), types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry()),
		},
		{
			name: "convert",
			err:  types.NewError(errors.New("convert request failed"), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry()),
		},
		{
			name: "status zero skip retry",
			err:  types.NewErrorWithStatusCode(errors.New("failed to copy request to GeneralOpenAIRequest: boom"), types.ErrorCodeInvalidRequest, 0, types.ErrOptionWithSkipRetry()),
		},
	}
	for _, tc := range cases {
		if got := ClassForRecordedAttempt(tc.err); got != ClassUserError {
			t.Fatalf("%s must be a user error, got %s status=%d", tc.name, got, tc.err.StatusCode)
		}
	}
}

func TestLocalSkipRetryDoesNotTripBreaker(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	prevCfg := GetConfig()
	cfg := prevCfg
	cfg.Mode = "observe"
	cfg.ConsecutiveFailBreak = 8
	SetConfig(cfg)
	t.Cleanup(func() { SetConfig(prevCfg) })

	const id = 88021
	ResetHalfOpenInflightForTest()
	t.Cleanup(ResetHalfOpenInflightForTest)
	seedHalfOpenCache(t, []HalfOpenInfo{{ChannelID: id, Stage: "half_2"}})
	if !TryIncHalfOpenInflight(id, 1) {
		t.Fatal("seed slot")
	}
	consecFailMu.Lock()
	prevConsec := consecFails[id]
	consecFails[id] = 0
	consecFailMu.Unlock()
	t.Cleanup(func() {
		consecFailMu.Lock()
		consecFails[id] = prevConsec
		consecFailMu.Unlock()
	})

	prevReport := reportHalfOpenResult
	reported := false
	reportHalfOpenResult = func(channelID int, success bool) {
		reported = true
	}
	t.Cleanup(func() { reportHalfOpenResult = prevReport })

	err := types.NewError(errors.New("failed to copy request to GeneralOpenAIRequest: boom"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyHalfOpenReserved, id)
	OnAttemptEnd(c, AttemptEvent{
		ChannelID: id,
		Model:     "m",
		Success:   false,
		Class:     ClassForRecordedAttempt(err),
	})

	if reported {
		t.Fatal("local skip-retry must not report a half-open probe failure")
	}
	consecFailMu.Lock()
	got := consecFails[id]
	consecFailMu.Unlock()
	if got != 0 {
		t.Fatalf("local skip-retry must not increase consecutive failures, got %d", got)
	}
	if left := halfOpenInflightCount(id); left != 0 {
		t.Fatalf("the request slot must still be released, count=%d", left)
	}
}

func TestRecordedUpstreamFailureStillCounts(t *testing.T) {
	upstream := types.NewErrorWithStatusCode(errors.New("boom"), types.ErrorCodeBadResponseStatusCode, 500)
	if got := ClassForRecordedAttempt(upstream); got != ClassServer {
		t.Fatalf("upstream 500 must stay a channel failure, got %s", got)
	}
	timeout := types.NewErrorWithStatusCode(errors.New("upstream timeout"), types.ErrorCodeBadResponse, 0)
	if got := ClassForRecordedAttempt(timeout); got != ClassServer {
		t.Fatalf("timeout without skip-retry must stay a channel failure, got %s", got)
	}
}
