package adtylimit

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 两种计数都跑同一组用例：内存和 miniredis。
func stores(t *testing.T) map[string]Store {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return map[string]Store{"memory": NewMemoryStore(), "redis": NewRedisStore(client)}
}

var minuteStart = time.Unix(1_700_000_040, 0) // 一分钟的开头

func TestAdtyLimitRPM(t *testing.T) {
	for name, store := range stores(t) {
		limits := Limits{RPM: 2}
		for i := range 2 {
			_, reason, err := Check(store, 7, limits, "m", minuteStart.Add(time.Duration(i)*time.Second))
			require.NoError(t, err, name)
			assert.Equal(t, ReasonNone, reason, name)
		}
		_, reason, err := Check(store, 7, limits, "m", minuteStart.Add(30*time.Second))
		require.NoError(t, err, name)
		assert.Equal(t, ReasonRPM, reason, "%s: the third request in the same minute", name)
		_, reason, _ = Check(store, 7, limits, "m", minuteStart.Add(61*time.Second))
		assert.Equal(t, ReasonNone, reason, "%s: next minute starts over", name)
		_, reason, _ = Check(store, 8, limits, "m", minuteStart)
		assert.Equal(t, ReasonNone, reason, "%s: other users are separate", name)
	}
}

func TestAdtyLimitConcurrency(t *testing.T) {
	for name, store := range stores(t) {
		limits := Limits{Concurrency: 1}
		release, reason, err := Check(store, 7, limits, "a", minuteStart)
		require.NoError(t, err, name)
		require.Equal(t, ReasonNone, reason, name)
		require.NotNil(t, release, name)
		_, reason, _ = Check(store, 7, limits, "b", minuteStart)
		assert.Equal(t, ReasonConcurrency, reason, "%s: second while the first runs", name)
		release()
		next, reason, _ := Check(store, 7, limits, "c", minuteStart)
		assert.Equal(t, ReasonNone, reason, "%s: third after the first finished", name)
		next()
	}
}

func TestAdtyLimitConcurrencyExpiredMember(t *testing.T) {
	for name, store := range stores(t) {
		limits := Limits{Concurrency: 1}
		_, reason, _ := Check(store, 7, limits, "crashed", minuteStart)
		require.Equal(t, ReasonNone, reason, name)
		_, reason, _ = Check(store, 7, limits, "later", minuteStart.Add(ConcurrencyTTL+time.Second))
		assert.Equal(t, ReasonNone, reason, "%s: a request never released expires", name)
	}
}

func TestAdtyLimitTPM(t *testing.T) {
	for name, store := range stores(t) {
		limits := Limits{TPM: 1000}
		minute := MinuteOf(minuteStart)
		require.NoError(t, store.AddTPM(7, minute, 600), name)
		_, reason, _ := Check(store, 7, limits, "m", minuteStart)
		assert.Equal(t, ReasonNone, reason, name)
		require.NoError(t, store.AddTPM(7, minute, 400), name)
		_, reason, _ = Check(store, 7, limits, "m", minuteStart.Add(time.Second))
		assert.Equal(t, ReasonTPM, reason, "%s: 1000 tokens used", name)
		_, reason, _ = Check(store, 7, limits, "m", minuteStart.Add(time.Minute))
		assert.Equal(t, ReasonNone, reason, "%s: next minute", name)
	}
}

func TestRetryAfter(t *testing.T) {
	assert.EqualValues(t, 60, RetryAfter(minuteStart))
	assert.EqualValues(t, 15, RetryAfter(minuteStart.Add(45*time.Second)))
}
