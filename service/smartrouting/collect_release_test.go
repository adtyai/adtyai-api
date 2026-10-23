package smartrouting

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func TestOnAttemptEndRateLimitUsesRequestModel(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	prev := GetConfig()
	cfg := prev
	cfg.Mode = "observe"
	cfg.RateLimitCooldownSec = 60
	SetConfig(cfg)
	prevCounter := routingCounter
	routingCounter = nil
	id := 88111
	modelName := "gpt-closeout"
	key := rateLimitKey(id, modelName)
	other := rateLimitKey(id, "other-model")
	empty := rateLimitKey(id, "")
	rateLimitMu.Lock()
	delete(rateLimitUntil, key)
	delete(rateLimitUntil, other)
	delete(rateLimitUntil, empty)
	rateLimitMu.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = prevRedis
		SetConfig(prev)
		routingCounter = prevCounter
		rateLimitMu.Lock()
		delete(rateLimitUntil, key)
		delete(rateLimitUntil, other)
		delete(rateLimitUntil, empty)
		rateLimitMu.Unlock()
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	OnAttemptEnd(c, AttemptEvent{ChannelID: id, ReqModel: modelName, Success: false, Class: ClassRateLimit})
	if !InRateLimitCooldown(id, modelName) {
		t.Fatal("rate limit must follow the request model, not an empty model key")
	}
	if InRateLimitCooldown(id, "other-model") {
		t.Fatal("rate limit must not apply to a different model")
	}
}

func TestOnAttemptEndDoesNotStealUnreservedHalfOpen(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	prev := GetConfig()
	cfg := prev
	cfg.Mode = "off"
	cfg.Enabled = false
	SetConfig(cfg)
	t.Cleanup(func() { SetConfig(prev) })

	halfOpenInflightMu.Lock()
	prevCount := halfOpenInflightLocal[42]
	halfOpenInflightLocal[42] = 1
	halfOpenInflightMu.Unlock()
	t.Cleanup(func() {
		halfOpenInflightMu.Lock()
		if prevCount == 0 {
			delete(halfOpenInflightLocal, 42)
		} else {
			halfOpenInflightLocal[42] = prevCount
		}
		halfOpenInflightMu.Unlock()
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	OnAttemptEnd(c, AttemptEvent{ChannelID: 42, Model: "m", Success: false, Class: ClassServer})
	if got := halfOpenInflightCount(42); got != 1 {
		t.Fatalf("unreserved attempt stole a slot, count=%d", got)
	}

	common.SetContextKey(c, constant.ContextKeyHalfOpenReserved, 42)
	OnAttemptEnd(c, AttemptEvent{ChannelID: 99, Model: "m", Success: false, Class: ClassServer})
	if got := halfOpenInflightCount(42); got != 0 {
		t.Fatalf("reserved slot was not released, count=%d", got)
	}
	if common.GetContextKeyInt(c, constant.ContextKeyHalfOpenReserved) != 0 {
		t.Fatal("reservation key must be cleared")
	}
}
