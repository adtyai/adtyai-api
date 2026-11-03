package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestLogExportStopsWhenRenewFails(t *testing.T) {
	previousTTL := logExportLockTTL
	logExportLockTTL = 300 * time.Millisecond
	t.Cleanup(func() { logExportLockTTL = previousTTL })
	previousRedis, previousClient := common.RedisEnabled, common.RDB
	server := miniredis.RunT(t)
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = previousRedis, previousClient
	})

	ctx, release, ok, err := AcquireLogExportLock(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, common.RDB.Del(context.Background(), "log_export:7").Err())
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("renew failure did not stop the export")
	}

	ctx2, release2, ok, err := AcquireLogExportLock(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, ctx2.Err())
	release2()
	release()
}
