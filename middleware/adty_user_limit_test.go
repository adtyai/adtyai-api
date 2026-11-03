package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/adtylimit"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 按用户限制的中间件（任务 7.4）。用户单独的限制存在主库，这里用内存 SQLite 只放那一张表。

type limitFixture struct {
	engine *gin.Engine
	store  *adtylimit.MemoryStore
	now    time.Time
	block  chan struct{}
}

func newLimitFixture(t *testing.T, cfg adty_setting.LimitSetting) *limitFixture {
	t.Helper()
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AdtyUserLimit{}))
	oldDB := model.DB
	model.DB = db
	setting := adty_setting.GetLimitSetting()
	saved := *setting
	*setting = cfg
	t.Cleanup(func() {
		model.DB = oldDB
		*setting = saved
	})

	f := &limitFixture{store: adtylimit.NewMemoryStore(), now: time.Unix(1_700_000_040, 0), block: make(chan struct{})}
	SetAdtyLimitStoreForTest(t, f.store, func() time.Time { return f.now })
	gin.SetMode(gin.TestMode)
	f.engine = gin.New()
	auth := func(c *gin.Context) { c.Set("id", 7); c.Set("token_id", 1); c.Next() }
	f.engine.POST("/v1/chat/completions", auth, AdtyUserLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	f.engine.POST("/v1/slow", auth, AdtyUserLimit(), func(c *gin.Context) { <-f.block; c.Status(http.StatusOK) })
	f.engine.POST("/mj/submit/imagine", auth, AdtyUserLimit(true), func(c *gin.Context) { c.Status(http.StatusOK) })
	f.engine.GET("/mj/task/:id/fetch", auth, func(c *gin.Context) { c.Status(http.StatusOK) })
	return f
}

func (f *limitFixture) do(method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func TestAdtyLimitDisabledPassesAll(t *testing.T) {
	f := newLimitFixture(t, adty_setting.LimitSetting{Enabled: false, DefaultRPM: 1})
	for range 5 {
		assert.Equal(t, http.StatusOK, f.do(http.MethodPost, "/v1/chat/completions").Code)
	}
}

func TestAdtyLimitRPMReturns429WithRetryAfter(t *testing.T) {
	f := newLimitFixture(t, adty_setting.LimitSetting{Enabled: true, DefaultRPM: 2})
	assert.Equal(t, http.StatusOK, f.do(http.MethodPost, "/v1/chat/completions").Code)
	assert.Equal(t, http.StatusOK, f.do(http.MethodPost, "/v1/chat/completions").Code)
	f.now = f.now.Add(45 * time.Second)
	resp := f.do(http.MethodPost, "/v1/chat/completions")
	assert.Equal(t, http.StatusTooManyRequests, resp.Code)
	assert.Equal(t, "15", resp.Header().Get("Retry-After"))
	assert.Contains(t, resp.Body.String(), `"code":"rate_limit_exceeded"`)
}

func TestAdtyLimitConcurrencyThroughMiddleware(t *testing.T) {
	f := newLimitFixture(t, adty_setting.LimitSetting{Enabled: true, DefaultConcurrency: 1})
	done := make(chan int)
	go func() { done <- f.do(http.MethodPost, "/v1/slow").Code }()
	require.Eventually(t, func() bool {
		return f.do(http.MethodPost, "/v1/chat/completions").Code == http.StatusTooManyRequests
	}, time.Second, 5*time.Millisecond, "the second request waits for the first")
	close(f.block)
	assert.Equal(t, http.StatusOK, <-done)
	assert.Equal(t, http.StatusOK, f.do(http.MethodPost, "/v1/chat/completions").Code, "the slot is released")
}

func TestAdtyLimitUserOverride(t *testing.T) {
	f := newLimitFixture(t, adty_setting.LimitSetting{Enabled: true, DefaultRPM: 1})
	require.NoError(t, model.SaveAdtyUserLimit(model.AdtyUserLimit{UserId: 7, RPM: 0, TPM: -1, Concurrency: -1}))
	for range 3 {
		assert.Equal(t, http.StatusOK, f.do(http.MethodPost, "/v1/chat/completions").Code, "0 means no limit for this user")
	}
	require.NoError(t, model.SaveAdtyUserLimit(model.AdtyUserLimit{UserId: 7, RPM: -1, TPM: -1, Concurrency: -1}))
	f.now = f.now.Add(time.Minute)
	assert.Equal(t, http.StatusOK, f.do(http.MethodPost, "/v1/chat/completions").Code)
	assert.Equal(t, http.StatusTooManyRequests, f.do(http.MethodPost, "/v1/chat/completions").Code, "-1 follows the default")
}

func TestAdtyLimitMJQueryNotCounted(t *testing.T) {
	f := newLimitFixture(t, adty_setting.LimitSetting{Enabled: true, DefaultRPM: 1})
	for range 3 {
		assert.Equal(t, http.StatusOK, f.do(http.MethodGet, "/mj/task/1/fetch").Code)
	}
	assert.Equal(t, http.StatusOK, f.do(http.MethodPost, "/mj/submit/imagine").Code)
	resp := f.do(http.MethodPost, "/mj/submit/imagine")
	assert.Equal(t, http.StatusTooManyRequests, resp.Code)
	assert.Contains(t, resp.Body.String(), `"description"`, "Midjourney error shape")
}

func TestAdtyLimitTPMCountsWhenConsumeLogDisabled(t *testing.T) {
	f := newLimitFixture(t, adty_setting.LimitSetting{Enabled: true, DefaultTPM: 100})
	old := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = old })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("token_id", 1)
	model.RecordConsumeLog(c, 7, model.RecordConsumeLogParams{PromptTokens: 60, CompletionTokens: 50})

	used, err := f.store.TPM(7, adtylimit.MinuteOf(f.now))
	require.NoError(t, err)
	assert.EqualValues(t, 110, used)
	assert.Equal(t, http.StatusTooManyRequests, f.do(http.MethodPost, "/v1/chat/completions").Code)
}

func TestAdtyLimitSkipsChannelTest(t *testing.T) {
	f := newLimitFixture(t, adty_setting.LimitSetting{Enabled: true, DefaultTPM: 100})
	old := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = old })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	// Channel tests run without an API token.
	model.RecordConsumeLog(c, 7, model.RecordConsumeLogParams{PromptTokens: 500, CompletionTokens: 500})

	used, err := f.store.TPM(7, adtylimit.MinuteOf(f.now))
	require.NoError(t, err)
	assert.Zero(t, used)
}
