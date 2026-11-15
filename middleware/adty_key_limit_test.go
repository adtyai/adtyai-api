package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestKeyRateLimitRejectsTheNextRequestOverTheCap(t *testing.T) {
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AdtyKeyLimit{}))
	oldDB := model.DB
	model.DB = db
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		common.RedisEnabled = previousRedis
	})
	require.NoError(t, model.SaveAdtyKeyLimit(11, 1))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("token_id", 11)
		c.Next()
	}, AdtyKeyRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	other := gin.New()
	other.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("token_id", 12)
		c.Next()
	}, AdtyKeyRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	first := httptest.NewRecorder()
	engine.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	engine.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusTooManyRequests, second.Code, second.Body.String())

	untouched := httptest.NewRecorder()
	other.ServeHTTP(untouched, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusOK, untouched.Code, "a key without a cap is not limited")
	_ = time.Minute
}

// resetKeyRateMemory 清空内存分支的桶，并把 Redis 关掉（这是仓库支持的部署形态）。
func resetKeyRateMemory(t *testing.T) {
	t.Helper()
	previousRedis := common.RedisEnabled
	previousMemory, previousSwept := adtyKeyRateMemory, adtyKeyRateMemorySwept
	common.RedisEnabled = false
	adtyKeyRateMemory, adtyKeyRateMemorySwept = map[int]adtyKeyRateBucket{}, -1
	t.Cleanup(func() {
		common.RedisEnabled = previousRedis
		adtyKeyRateMemory, adtyKeyRateMemorySwept = previousMemory, previousSwept
	})
}

// 内存桶必须按窗口过期：Redis 分支每个分钟桶有 2 分钟 TTL，内存分支不能一个 key 一分钟留一条、
// 直到进程退出。推进 15 分钟后，不再写过的 key 的旧桶必须消失，map 长度只跟「还在用的 key 数」有关。
func TestKeyRateMemoryDropsBucketsOlderThanTheWindow(t *testing.T) {
	resetKeyRateMemory(t)
	start := time.Unix(1_700_000_040, 0).Truncate(time.Minute)
	const keys = 500

	for id := 1000; id < 1000+keys; id++ {
		allowed, err := adtyKeyRateAllow(id, 10, start)
		require.NoError(t, err)
		require.True(t, allowed)
	}
	require.Len(t, adtyKeyRateMemory, keys)

	// 同一 key 在下一分钟写入时，它上一分钟的桶被当前窗口顶掉；其它 key 的上一分钟桶还在，
	// 与 Redis 那 2 分钟 TTL 一致。
	allowed, err := adtyKeyRateAllow(1000, 10, start.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, allowed)
	require.Len(t, adtyKeyRateMemory, keys)
	require.Equal(t, start.Unix()/60, adtyKeyRateMemory[1001].minute)

	for step := 2; step <= 15; step++ {
		allowed, err := adtyKeyRateAllow(1000, 10, start.Add(time.Duration(step)*time.Minute))
		require.NoError(t, err)
		require.True(t, allowed)
		require.LessOrEqual(t, len(adtyKeyRateMemory), 2,
			"第 %d 分钟写一次后 map 里只剩当前与上一分钟的桶", step)
	}
}

// 清理不能把窗口内的计数也清掉：同一分钟第 2 次请求仍然超限，下一分钟重新开始算。
func TestKeyRateMemoryCapSurvivesTheSweepWithinOneMinute(t *testing.T) {
	resetKeyRateMemory(t)
	start := time.Unix(1_700_000_040, 0).Truncate(time.Minute)

	first, err := adtyKeyRateAllow(21, 1, start)
	require.NoError(t, err)
	require.True(t, first)

	second, err := adtyKeyRateAllow(21, 1, start)
	require.NoError(t, err)
	require.False(t, second, "同一分钟内的计数不能被清理掉")

	next, err := adtyKeyRateAllow(21, 1, start.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, next, "下一分钟是新的窗口")
}
