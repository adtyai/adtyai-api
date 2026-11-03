package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestClearChannelAffinityCacheReportsRedisFailure(t *testing.T) {
	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
		MaxRetries:   0,
	})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/affinity_cache?all=true", nil)
	ClearChannelAffinityCache(ctx)

	require.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestChannelAffinityStatsReportsRedisFailure(t *testing.T) {
	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
		MaxRetries:   0,
	})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/affinity_cache", nil)
	GetChannelAffinityCacheStats(ctx)

	require.Contains(t, recorder.Body.String(), `"success":false`)
	require.NotContains(t, recorder.Body.String(), `"total":0`)
}
