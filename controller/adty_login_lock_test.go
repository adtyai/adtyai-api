package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordLoginLocksAccountWithoutChangingTheError(t *testing.T) {
	previousEnabled, previousClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousEnabled, previousClient
	})
	const name = "lock-target"
	adtyClearLoginFailures(name)
	t.Cleanup(func() { adtyClearLoginFailures(name) })
	assert.False(t, adtyLoginLocked(name))
	for i := 0; i < adtyLoginFailureLimit; i++ {
		require.NoError(t, adtyRecordLoginFailure(name))
	}
	assert.True(t, adtyLoginLocked(name))
	assert.Equal(t, "Username or password is incorrect, or user has been banned", adtyLoginFailureResponse().Key)
	adtyClearLoginFailures(name)
	assert.False(t, adtyLoginLocked(name))
}

func TestLoginLockUsesRedisWhenItIsEnabled(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})
	previousEnabled, previousClient := common.RedisEnabled, common.RDB
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousEnabled, previousClient
		adtyLoginFailuresMu.Lock()
		adtyLoginFailures = map[string]adtyLoginFailure{}
		adtyLoginFailuresMu.Unlock()
	})
	common.RedisEnabled = true
	common.RDB = nil
	assert.True(t, adtyLoginLocked("missing-store"), "a configured Redis that is not connected must not fall open")
	require.Error(t, adtyRecordLoginFailure("missing-store"))

	common.RDB = clientA
	const name = "shared-lock"
	assert.False(t, adtyLoginLocked(name))
	for i := 0; i < adtyLoginFailureLimit; i++ {
		require.NoError(t, adtyRecordLoginFailure(name))
	}
	adtyLoginFailuresMu.Lock()
	adtyLoginFailures = map[string]adtyLoginFailure{}
	adtyLoginFailuresMu.Unlock()
	common.RDB = clientB
	assert.True(t, adtyLoginLocked(name), "another process must see the same failure count")
	adtyClearLoginFailures(name)
	assert.False(t, adtyLoginLocked(name))

	server.Close()
	require.Error(t, adtyRecordLoginFailure("after-outage"))
	assert.True(t, adtyLoginLocked("after-outage"))
}
