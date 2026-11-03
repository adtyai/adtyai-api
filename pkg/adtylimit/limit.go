// Package adtylimit 按用户限制每分钟请求数（RPM）、每分钟 token 数（TPM）和同时进行的请求数（任务 7.4）。
// 开了 Redis 用 Redis（多实例共享），没开用进程内存（只适合单实例）。
//
// TPM 是事后累计：token 数在请求结束写消费日志时才加上，同一分钟里并发进来的大请求拦不住，
// 只能挡住之后的请求。
package adtylimit

import (
	"strconv"
	"time"
)

// Limits 是一个用户的限额，0 表示不限。
type Limits struct {
	RPM         int
	TPM         int
	Concurrency int
}

// Store 是计数的存放处。
type Store interface {
	// IncrRPM 给这一分钟的请求数加一，返回加之后的值。
	IncrRPM(userID int, minute int64) (int64, error)
	// TPM 返回这一分钟已经累计的 token 数。
	TPM(userID int, minute int64) (int64, error)
	// AddTPM 给这一分钟加上 token 数。
	AddTPM(userID int, minute int64, tokens int64) error
	// Acquire 在没超过上限时登记一个进行中的请求（ttl 后自动过期），返回是否登记成功。
	Acquire(userID int, member string, limit int, now time.Time, ttl time.Duration) (bool, error)
	// Release 去掉一个进行中的请求。
	Release(userID int, member string) error
}

// Reason 是拒绝的原因。
type Reason string

const (
	ReasonNone        Reason = ""
	ReasonRPM         Reason = "rpm"
	ReasonTPM         Reason = "tpm"
	ReasonConcurrency Reason = "concurrency"
)

// ConcurrencyTTL 是进行中请求的最长登记时间：请求没走到释放（进程崩溃）时，到期自动清掉。
const ConcurrencyTTL = 15 * time.Minute

// MinuteOf 是分钟编号，RPM 和 TPM 都按它分桶。
func MinuteOf(now time.Time) int64 { return now.Unix() / 60 }

// RetryAfter 是到下一分钟还剩的秒数，至少 1 秒。
func RetryAfter(now time.Time) int64 { return max(60-now.Unix()%60, 1) }

// Redis 的键（内存实现直接按分钟和用户分桶）。
func rpmKey(userID int, minute int64) string {
	return "ADTY:RPM:" + strconv.Itoa(userID) + ":" + strconv.FormatInt(minute, 10)
}

func tpmKey(userID int, minute int64) string {
	return "ADTY:TPM:" + strconv.Itoa(userID) + ":" + strconv.FormatInt(minute, 10)
}

func concurrencyKey(userID int) string { return "ADTY:CONC:" + strconv.Itoa(userID) }

// Check 按顺序检查 TPM、RPM、并发；通过时如果登记了并发，release 非空，请求结束时调用。
func Check(store Store, userID int, limits Limits, member string, now time.Time) (release func(), reason Reason, err error) {
	minute := MinuteOf(now)
	if limits.TPM > 0 {
		used, err := store.TPM(userID, minute)
		if err != nil {
			return nil, ReasonNone, err
		}
		if used >= int64(limits.TPM) {
			return nil, ReasonTPM, nil
		}
	}
	if limits.RPM > 0 {
		count, err := store.IncrRPM(userID, minute)
		if err != nil {
			return nil, ReasonNone, err
		}
		if count > int64(limits.RPM) {
			return nil, ReasonRPM, nil
		}
	}
	if limits.Concurrency <= 0 {
		return nil, ReasonNone, nil
	}
	ok, err := store.Acquire(userID, member, limits.Concurrency, now, ConcurrencyTTL)
	if err != nil {
		return nil, ReasonNone, err
	}
	if !ok {
		return nil, ReasonConcurrency, nil
	}
	return func() { _ = store.Release(userID, member) }, ReasonNone, nil
}
