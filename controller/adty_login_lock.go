package controller

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

var errLoginLockUnavailable = errors.New("login lock store unavailable")

const adtyLoginFailureLimit = 8

type adtyLoginFailure struct {
	count int
	until time.Time
}

var (
	adtyLoginFailuresMu sync.Mutex
	adtyLoginFailures   = map[string]adtyLoginFailure{}
)

func adtyLoginKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func adtyLoginRedisKey(username string) string {
	return "adty:login-fail:" + adtyLoginKey(username)
}

func adtyLoginUsesRedis() bool {
	return common.RedisEnabled && common.RDB != nil
}

func adtyLoginLocked(username string) bool {
	key := adtyLoginKey(username)
	if key == "" {
		return false
	}
	if common.RedisEnabled && common.RDB == nil {
		return true
	}
	if adtyLoginUsesRedis() {
		n, err := common.RDB.Get(context.Background(), adtyLoginRedisKey(username)).Int()
		if err == redis.Nil {
			return false
		}
		if err != nil {
			return true
		}
		return n >= adtyLoginFailureLimit
	}
	adtyLoginFailuresMu.Lock()
	defer adtyLoginFailuresMu.Unlock()
	entry, ok := adtyLoginFailures[key]
	if !ok || time.Now().After(entry.until) {
		delete(adtyLoginFailures, key)
		return false
	}
	return entry.count >= adtyLoginFailureLimit
}

func adtyRecordLoginFailure(username string) error {
	key := adtyLoginKey(username)
	if key == "" {
		return nil
	}
	if common.RedisEnabled && common.RDB == nil {
		return errLoginLockUnavailable
	}
	if adtyLoginUsesRedis() {
		ctx := context.Background()
		redisKey := adtyLoginRedisKey(username)
		if err := common.RDB.Incr(ctx, redisKey).Err(); err != nil {
			return err
		}
		if err := common.RDB.Expire(ctx, redisKey, 15*time.Minute).Err(); err != nil {
			return err
		}
		return nil
	}
	adtyLoginFailuresMu.Lock()
	defer adtyLoginFailuresMu.Unlock()
	entry := adtyLoginFailures[key]
	if time.Now().After(entry.until) {
		entry = adtyLoginFailure{}
	}
	entry.count++
	entry.until = time.Now().Add(15 * time.Minute)
	adtyLoginFailures[key] = entry
	if len(adtyLoginFailures) > 10000 {
		for name, stored := range adtyLoginFailures {
			if time.Now().After(stored.until) {
				delete(adtyLoginFailures, name)
			}
		}
	}
	return nil
}

func adtyClearLoginFailures(username string) {
	key := adtyLoginKey(username)
	if adtyLoginUsesRedis() && key != "" {
		_ = common.RDB.Del(context.Background(), adtyLoginRedisKey(username)).Err()
	}
	adtyLoginFailuresMu.Lock()
	delete(adtyLoginFailures, key)
	adtyLoginFailuresMu.Unlock()
}

func adtyLoginFailureResponse() *common.Message {
	return common.NewMessage("Username or password is incorrect, or user has been banned")
}
