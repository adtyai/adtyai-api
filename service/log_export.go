package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var logExportLockTTL = time.Minute

const logExportLockRenewScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 0`

const logExportLockReleaseScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`

// logExportLocks holds the running exports when Redis is disabled.
var logExportLocks sync.Map

// AcquireLogExportLock reserves the one log export a user may run at a time.
// With Redis the reservation spans all nodes and lapses within a minute if
// this node dies. ok is false while another export of the user is running;
// otherwise release must be called when the export ends.
func AcquireLogExportLock(parent context.Context, userId int) (ctx context.Context, release func(), ok bool, err error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	if !common.RedisEnabled {
		if _, running := logExportLocks.LoadOrStore(userId, struct{}{}); running {
			cancel()
			return nil, nil, false, nil
		}
		return ctx, func() {
			cancel()
			logExportLocks.Delete(userId)
		}, true, nil
	}

	key := fmt.Sprintf("log_export:%d", userId)
	owner := common.GetRandomString(16)
	acquired, err := common.RDB.SetNX(context.Background(), key, owner, logExportLockTTL).Result()
	if err != nil || !acquired {
		cancel()
		return nil, nil, false, err
	}

	stop := make(chan struct{})
	var stopOnce sync.Once
	stopRenew := func() { stopOnce.Do(func() { close(stop) }) }
	go func() {
		ticker := time.NewTicker(logExportLockTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				renewed, evalErr := common.RDB.Eval(context.Background(), logExportLockRenewScript, []string{key}, owner, logExportLockTTL.Milliseconds()).Int64()
				if evalErr != nil || renewed != 1 {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, func() {
		stopRenew()
		cancel()
		common.RDB.Eval(context.Background(), logExportLockReleaseScript, []string{key}, owner)
	}, true, nil
}
