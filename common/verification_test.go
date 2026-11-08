package common

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedVerificationConsumesOnlyMatchingCode(t *testing.T) {
	server := miniredis.RunT(t)
	previousEnabled, previousClient := RedisEnabled, RDB
	RedisEnabled = true
	RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = RDB.Close()
		RedisEnabled, RDB = previousEnabled, previousClient
	})
	const email = "reset@example.com"
	require.NoError(t, RegisterVerificationCodeWithKey(email, "valid-token", PasswordResetPurpose))
	_, ok := ConsumeVerificationCodeWithKey(email, "wrong-token", PasswordResetPurpose)
	assert.False(t, ok)
	require.True(t, VerifyCodeWithKey(email, "valid-token", PasswordResetPurpose), "a wrong guess must preserve the issued link")
	var successes atomic.Int32
	var callers sync.WaitGroup
	for range 8 {
		callers.Go(func() {
			if _, matched := ConsumeVerificationCodeWithKey(email, "valid-token", PasswordResetPurpose); matched {
				successes.Add(1)
			}
		})
	}
	callers.Wait()
	assert.EqualValues(t, 1, successes.Load())
}
