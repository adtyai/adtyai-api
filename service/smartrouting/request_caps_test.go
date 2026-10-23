package smartrouting

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func TestBindRouteCapsReadsThisRequestOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"m","stream":true,"tools":[{"type":"function"}],"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}],"response_format":{"type":"json_schema"}}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	caps := BindRouteCaps(c, false, 0, 12)
	if !caps.IsStream || !caps.NeedTools || !caps.NeedVision || !caps.NeedJSON || caps.NeedContext != 12 {
		t.Fatalf("%+v", caps)
	}
	again := RouteCapsFromContext(c)
	if again.IsStream != caps.IsStream || again.NeedTools != caps.NeedTools || again.NeedVision != caps.NeedVision || again.NeedJSON != caps.NeedJSON || again.NeedContext != caps.NeedContext {
		t.Fatalf("context caps %+v", again)
	}

	empty, _ := gin.CreateTestContext(httptest.NewRecorder())
	empty.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	none := BindRouteCaps(empty, false, 0, 0)
	if none.IsStream || none.NeedTools || none.NeedVision || none.NeedJSON || none.NeedContext != 0 {
		t.Fatalf("missing body must not reuse the previous request: %+v", none)
	}
}

func TestMissingRoutingConfigStaysActive(t *testing.T) {
	prevLoaded := configLoadedFromOptions.Load()
	configLoadedFromOptions.Store(false)
	prevCfg := GetConfig()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	prev, had := common.OptionMap[OptionKey]
	delete(common.OptionMap, OptionKey)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		SetConfig(prevCfg)
		configLoadedFromOptions.Store(prevLoaded)
		common.OptionMapRWMutex.Lock()
		if had {
			common.OptionMap[OptionKey] = prev
		} else {
			delete(common.OptionMap, OptionKey)
		}
		common.OptionMapRWMutex.Unlock()
	})
	LoadConfigFromOptions()
	cfg := GetConfig()
	if cfg.Mode != "active" || !cfg.Enabled {
		t.Fatalf("missing config must stay active, got %+v", cfg)
	}
}

func TestBadConfigJSONDoesNotForceOff(t *testing.T) {
	prevLoaded := configLoadedFromOptions.Load()
	configLoadedFromOptions.Store(false)
	prevCfg := GetConfig()
	SetConfig(DefaultConfig())
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	prev, had := common.OptionMap[OptionKey]
	common.OptionMap[OptionKey] = `{"mode":`
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		SetConfig(prevCfg)
		configLoadedFromOptions.Store(prevLoaded)
		common.OptionMapRWMutex.Lock()
		if had {
			common.OptionMap[OptionKey] = prev
		} else {
			delete(common.OptionMap, OptionKey)
		}
		common.OptionMapRWMutex.Unlock()
	})
	LoadConfigFromOptions()
	cfg := GetConfig()
	if cfg.Mode != "active" || !cfg.Enabled {
		t.Fatalf("bad json before the first load must not force off, got %+v", cfg)
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap[OptionKey] = `{"mode":"observe","enabled":true}`
	common.OptionMapRWMutex.Unlock()
	LoadConfigFromOptions()
	common.OptionMapRWMutex.Lock()
	common.OptionMap[OptionKey] = `{"mode":`
	common.OptionMapRWMutex.Unlock()
	LoadConfigFromOptions()
	if GetConfig().Mode != "observe" {
		t.Fatalf("bad json after a load must keep the last good config, got %+v", GetConfig())
	}
}

func TestOmittedDefaultTrueRefillsButExplicitFalseStays(t *testing.T) {
	prevLoaded := configLoadedFromOptions.Load()
	prevCfg := GetConfig()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	prev, had := common.OptionMap[OptionKey]
	common.OptionMap[OptionKey] = `{"mode":"observe"}`
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		SetConfig(prevCfg)
		configLoadedFromOptions.Store(prevLoaded)
		common.OptionMapRWMutex.Lock()
		if had {
			common.OptionMap[OptionKey] = prev
		} else {
			delete(common.OptionMap, OptionKey)
		}
		common.OptionMapRWMutex.Unlock()
	})
	LoadConfigFromOptions()
	if !GetConfig().HalfOpenPreferNonStream || !GetConfig().BalancePatrolEnabled || !GetConfig().PriceScoreSkipNonToken {
		t.Fatalf("omitted default-true keys must be refilled, got %+v", GetConfig())
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap[OptionKey] = `{"mode":"observe","halfopen_prefer_nonstream":false,"balance_patrol_enabled":false,"price_score_skip_non_token":false}`
	common.OptionMapRWMutex.Unlock()
	LoadConfigFromOptions()
	cfg := GetConfig()
	if cfg.HalfOpenPreferNonStream || cfg.BalancePatrolEnabled || cfg.PriceScoreSkipNonToken {
		t.Fatalf("explicit false must stay false, got %+v", cfg)
	}
}

func TestBindRouteCapsGeminiURLStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1beta/models/gemini:streamGenerateContent?alt=sse", strings.NewReader("not-json"))
	caps := BindRouteCaps(c, false, 0, 0)
	if !caps.IsStream {
		t.Fatal("gemini stream url must set IsStream even when the body is not json")
	}
}

func TestBindRouteCapsCountsWhenBillingCountIsOff(t *testing.T) {
	prev := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = prev })
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte("hello tokens")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(gin.BodyBytesKey, body)
	caps := BindRouteCaps(c, false, 0, 0)
	if caps.NeedContext == 0 {
		t.Fatal("routing must count this request even when billing token count is off")
	}
}

func TestBindRouteCapsEstimatesOmittedPromptTokens(t *testing.T) {
	prev := constant.CountToken
	constant.CountToken = true
	t.Cleanup(func() { constant.CountToken = prev })
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("hello tokens"))
	caps := BindRouteCaps(c, false, 0, 0)
	if caps.NeedContext == 0 {
		t.Fatal("omitted prompt tokens should be estimated from the raw body")
	}
}

func TestEnabledFalseWithoutModeIsOff(t *testing.T) {
	prevCfg := GetConfig()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	prev, had := common.OptionMap[OptionKey]
	common.OptionMap[OptionKey] = `{"enabled":false}`
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		SetConfig(prevCfg)
		common.OptionMapRWMutex.Lock()
		if had {
			common.OptionMap[OptionKey] = prev
		} else {
			delete(common.OptionMap, OptionKey)
		}
		common.OptionMapRWMutex.Unlock()
	})
	LoadConfigFromOptions()
	cfg := GetConfig()
	if cfg.Mode != "off" || cfg.Enabled {
		t.Fatalf("enabled false without mode must be off, got %+v", cfg)
	}
}
