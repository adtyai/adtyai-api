package common

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useMemoryVerificationStore(t *testing.T) {
	t.Helper()
	previousEnabled, previousClient := RedisEnabled, RDB
	RedisEnabled, RDB = false, nil
	verificationMutex.Lock()
	previousMap := verificationMap
	verificationMap = make(map[string]verificationValue)
	verificationMutex.Unlock()
	t.Cleanup(func() {
		verificationMutex.Lock()
		verificationMap = previousMap
		verificationMutex.Unlock()
		RedisEnabled, RDB = previousEnabled, previousClient
	})
}

func useSharedVerificationStore(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	previousEnabled, previousClient := RedisEnabled, RDB
	RedisEnabled = true
	RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = RDB.Close()
		RedisEnabled, RDB = previousEnabled, previousClient
	})
	return server
}

func TestSharedVerificationConsumesOnlyMatchingCode(t *testing.T) {
	_ = useSharedVerificationStore(t)
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

func TestVerificationCodeAttemptLimit(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		useMemoryVerificationStore(t)
		assertVerificationCodeAttemptLimit(t, nil)
	})
	t.Run("redis", func(t *testing.T) {
		server := useSharedVerificationStore(t)
		assertVerificationCodeAttemptLimit(t, server)
	})
}

func assertVerificationCodeAttemptLimit(t *testing.T, server *miniredis.Miniredis) {
	t.Helper()
	const key = "user@example.com"
	const code = "a1b2c3"
	require.NoError(t, RegisterVerificationCodeWithKey(key, code, EmailVerificationPurpose))

	for i := 1; i < MaxFailAttempts; i++ {
		issued, status := ConsumeVerificationCodeWithKeyStatus(key, "000000", EmailVerificationPurpose)
		assert.True(t, issued.IsZero())
		assert.Equal(t, VerificationInvalid, status, "failed attempt %d should still be retryable", i)
		assert.True(t, VerifyCodeWithKey(key, code, EmailVerificationPurpose), "a counted miss must not consume the code")
	}

	_, status := ConsumeVerificationCodeWithKeyStatus(key, "000000", EmailVerificationPurpose)
	assert.Equal(t, VerificationLocked, status, "the attempt that reaches MaxFailAttempts must lock")

	issued, status := ConsumeVerificationCodeWithKeyStatus(key, code, EmailVerificationPurpose)
	assert.True(t, issued.IsZero())
	assert.Equal(t, VerificationLocked, status, "a later correct guess must not succeed after the lock")
	assert.Equal(t, VerificationLocked, VerifyCodeWithKeyStatus(key, code, EmailVerificationPurpose), "Verify must honor the same lock")
	_, ok := ConsumeVerificationCodeWithKey(key, code, EmailVerificationPurpose)
	assert.False(t, ok, "the locked code must not be consumed as success")

	require.NoError(t, RegisterVerificationCodeWithKey(key, "d4e5f6", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(key, code, EmailVerificationPurpose), "the previous code must stay unusable")
	issued, status = ConsumeVerificationCodeWithKeyStatus(key, "d4e5f6", EmailVerificationPurpose)
	assert.Equal(t, VerificationOK, status)
	assert.False(t, issued.IsZero())
	_, ok = ConsumeVerificationCodeWithKey(key, "d4e5f6", EmailVerificationPurpose)
	assert.False(t, ok, "a newly issued code is still single-use")

	if server != nil {
		require.NoError(t, RegisterVerificationCodeWithKey(key, "legacy1", EmailVerificationPurpose))
		// A payload issued before the failure counter existed must still count misses.
		ttl := time.Duration(VerificationValidMinutes) * time.Minute
		require.NoError(t, RDB.Set(t.Context(), verificationRedisKey(EmailVerificationPurpose, key), time.Now().UTC().Format(time.RFC3339Nano)+"\nlegacy1", ttl).Err())
		for i := 0; i < MaxFailAttempts; i++ {
			_, status = ConsumeVerificationCodeWithKeyStatus(key, "wrong", EmailVerificationPurpose)
		}
		assert.Equal(t, VerificationLocked, status)
		_, status = ConsumeVerificationCodeWithKeyStatus(key, "legacy1", EmailVerificationPurpose)
		assert.Equal(t, VerificationLocked, status)
	}
}

func TestVerificationCodePurposeBindingDoesNotShareAttemptCounter(t *testing.T) {
	stores := []string{"memory", "redis"}
	for _, store := range stores {
		t.Run(store, func(t *testing.T) {
			if store == "memory" {
				useMemoryVerificationStore(t)
			} else {
				_ = useSharedVerificationStore(t)
			}
			const key = "user@example.com"
			require.NoError(t, RegisterVerificationCodeWithKey(key, "111111", EmailVerificationPurpose))
			require.NoError(t, RegisterVerificationCodeWithKey(key, "222222", AccountDeletionPurpose))

			for i := 0; i < MaxFailAttempts; i++ {
				_, status := ConsumeVerificationCodeWithKeyStatus(key, "000000", EmailVerificationPurpose)
				if i+1 < MaxFailAttempts {
					assert.Equal(t, VerificationInvalid, status)
				} else {
					assert.Equal(t, VerificationLocked, status)
				}
			}
			assert.Equal(t, VerificationLocked, VerifyCodeWithKeyStatus(key, "111111", EmailVerificationPurpose))
			_, crossed := ConsumeVerificationCodeWithKeyStatus(key, "111111", AccountDeletionPurpose)
			assert.Equal(t, VerificationInvalid, crossed, "a purpose A code must not satisfy purpose B or change B's counter")
			issued, status := ConsumeVerificationCodeWithKeyStatus(key, "222222", AccountDeletionPurpose)
			assert.Equal(t, VerificationOK, status, "locking purpose A must not consume or lock purpose B")
			assert.False(t, issued.IsZero())
			assert.Equal(t, VerificationLocked, VerifyCodeWithKeyStatus(key, "111111", EmailVerificationPurpose))
		})
	}
}

func TestVerificationSuccessfulAttemptConsumesOnceAndRejectsReplay(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		useMemoryVerificationStore(t)
		assertVerificationSuccessfulConsumeAndReplay(t)
	})
	t.Run("redis", func(t *testing.T) {
		_ = useSharedVerificationStore(t)
		assertVerificationSuccessfulConsumeAndReplay(t)
	})
}

func assertVerificationSuccessfulConsumeAndReplay(t *testing.T) {
	t.Helper()
	const key = "owner@example.com"
	require.NoError(t, RegisterVerificationCodeWithKey(key, "135790", AccountDeletionPurpose))
	assert.Equal(t, VerificationOK, VerifyCodeWithKeyStatus(key, "135790", AccountDeletionPurpose), "Verify must not consume a correct early attempt")
	issued, status := ConsumeVerificationCodeWithKeyStatus(key, "135790", AccountDeletionPurpose)
	assert.Equal(t, VerificationOK, status)
	assert.False(t, issued.IsZero())
	_, status = ConsumeVerificationCodeWithKeyStatus(key, "135790", AccountDeletionPurpose)
	assert.Equal(t, VerificationInvalid, status)
	assert.False(t, VerifyCodeWithKey(key, "135790", AccountDeletionPurpose))
}

func TestVerificationCodeExpiryStillApplies(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		useMemoryVerificationStore(t)
		const key = "expired@example.com"
		require.NoError(t, RegisterVerificationCodeWithKey(key, "abc123", PasswordResetPurpose))
		verificationMutex.Lock()
		value := verificationMap[PasswordResetPurpose+key]
		value.time = time.Now().Add(-time.Duration(VerificationValidMinutes)*time.Minute - time.Second)
		verificationMap[PasswordResetPurpose+key] = value
		verificationMutex.Unlock()
		_, status := ConsumeVerificationCodeWithKeyStatus(key, "abc123", PasswordResetPurpose)
		assert.Equal(t, VerificationInvalid, status)
		assert.False(t, VerifyCodeWithKey(key, "abc123", PasswordResetPurpose))
	})
	t.Run("redis", func(t *testing.T) {
		server := useSharedVerificationStore(t)
		const key = "expired@example.com"
		require.NoError(t, RegisterVerificationCodeWithKey(key, "abc123", PasswordResetPurpose))
		server.FastForward(time.Duration(VerificationValidMinutes)*time.Minute + time.Second)
		_, status := ConsumeVerificationCodeWithKeyStatus(key, "abc123", PasswordResetPurpose)
		assert.Equal(t, VerificationInvalid, status)
		assert.False(t, VerifyCodeWithKey(key, "abc123", PasswordResetPurpose))
	})
}

func TestVerificationVerifyCountsFailedAttempts(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		useMemoryVerificationStore(t)
		assertVerificationVerifyCountsFailedAttempts(t)
	})
	t.Run("redis", func(t *testing.T) {
		_ = useSharedVerificationStore(t)
		assertVerificationVerifyCountsFailedAttempts(t)
	})
}

func assertVerificationVerifyCountsFailedAttempts(t *testing.T) {
	t.Helper()
	const key = "register@example.com"
	require.NoError(t, RegisterVerificationCodeWithKey(key, "feed12", EmailVerificationPurpose))
	for i := 0; i < MaxFailAttempts; i++ {
		assert.Equal(t, expectedStatusAfterFailure(i+1), VerifyCodeWithKeyStatus(key, "000000", EmailVerificationPurpose))
	}
	assert.Equal(t, VerificationLocked, VerifyCodeWithKeyStatus(key, "feed12", EmailVerificationPurpose))
	_, status := ConsumeVerificationCodeWithKeyStatus(key, "feed12", EmailVerificationPurpose)
	assert.Equal(t, VerificationLocked, status)
}

func expectedStatusAfterFailure(attempt int) VerificationStatus {
	if attempt >= MaxFailAttempts {
		return VerificationLocked
	}
	return VerificationInvalid
}
