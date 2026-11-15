package common

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// 口令失败计数：凡是「拿口令换凭据」的入口都必须共用这一份，否则那个入口就是
// 绕开失败锁定与限流的密码预言机（ISS-R4-0105）。
//
// 有这个计数之后，同一个用户名在所有入口上的失败次数合在一起算：登录、团队
// 注册链接这类匿名入口都一样。多副本共享靠 Redis；没开 Redis 时退回进程内存，
// 语义是「每个进程各自计数」。

// LoginLockLimit 是同一个用户名在 LoginLockWindow 内的最大失败次数。
const LoginLockLimit = 8

// LoginLockWindow 是失败计数的滑动窗口：每次失败都从当下重新计时。
const LoginLockWindow = 15 * time.Minute

// ErrLoginLockUnavailable 表示计数存不上（配置了 Redis 却连不上）。这种情况下
// 入口必须失败关闭，不能悄悄放行。
var ErrLoginLockUnavailable = errors.New("login lock store unavailable")

type loginFailure struct {
	count int
	until time.Time
}

var (
	loginFailuresMu sync.Mutex
	loginFailures   = map[string]loginFailure{}
)

func loginLockKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func loginLockRedisKey(username string) string {
	return "adty:login-fail:" + loginLockKey(username)
}

func loginLockUsesRedis() bool {
	return RedisEnabled && RDB != nil
}

// LoginLocked reports whether this username has used up its password attempts.
// 配了 Redis 却连不上时失败关闭：宁可拦下入口，也不能让猜口令无限次。
func LoginLocked(username string) bool {
	key := loginLockKey(username)
	if key == "" {
		return false
	}
	if RedisEnabled && RDB == nil {
		return true
	}
	if loginLockUsesRedis() {
		n, err := RDB.Get(context.Background(), loginLockRedisKey(username)).Int()
		if err == redis.Nil {
			return false
		}
		if err != nil {
			return true
		}
		return n >= LoginLockLimit
	}
	loginFailuresMu.Lock()
	defer loginFailuresMu.Unlock()
	entry, ok := loginFailures[key]
	if !ok || time.Now().After(entry.until) {
		delete(loginFailures, key)
		return false
	}
	return entry.count >= LoginLockLimit
}

// RecordLoginFailure 记一次口令猜错。存不上要报错，让调用方决定是否记录日志。
func RecordLoginFailure(username string) error {
	key := loginLockKey(username)
	if key == "" {
		return nil
	}
	if RedisEnabled && RDB == nil {
		return ErrLoginLockUnavailable
	}
	if loginLockUsesRedis() {
		ctx := context.Background()
		redisKey := loginLockRedisKey(username)
		if err := RDB.Incr(ctx, redisKey).Err(); err != nil {
			return err
		}
		if err := RDB.Expire(ctx, redisKey, LoginLockWindow).Err(); err != nil {
			return err
		}
		return nil
	}
	loginFailuresMu.Lock()
	defer loginFailuresMu.Unlock()
	entry := loginFailures[key]
	if time.Now().After(entry.until) {
		entry = loginFailure{}
	}
	entry.count++
	entry.until = time.Now().Add(LoginLockWindow)
	loginFailures[key] = entry
	if len(loginFailures) > 10000 {
		for name, stored := range loginFailures {
			if time.Now().After(stored.until) {
				delete(loginFailures, name)
			}
		}
	}
	return nil
}

// ClearLoginFailures 在一个口令验证成功之后清掉这个用户名的失败计数。
func ClearLoginFailures(username string) {
	key := loginLockKey(username)
	if loginLockUsesRedis() && key != "" {
		_ = RDB.Del(context.Background(), loginLockRedisKey(username)).Err()
	}
	loginFailuresMu.Lock()
	delete(loginFailures, key)
	loginFailuresMu.Unlock()
}

// ResetLoginFailuresForTest 清空进程内的失败计数（测试用，等价于换一个进程）。
func ResetLoginFailuresForTest() {
	loginFailuresMu.Lock()
	defer loginFailuresMu.Unlock()
	loginFailures = map[string]loginFailure{}
}
