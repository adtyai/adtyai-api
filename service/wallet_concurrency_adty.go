package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// 审查 R-P2-3：低余额用户并发透支防护。
// 余额低于 LowBalanceGuardUSD（默认 $5）的钱包请求，单用户同时进行中的请求数不超过
// LowBalanceMaxInflight（默认 2）。这样透支上限从「并发数 × 单次成本」收敛到约「(N-1) × 单次成本」。
// 每个占位带租约（15 分钟），即使会话未走到 Settle/Refund 也不会永久占用。
// 环境变量：ADTY_LOW_BALANCE_GUARD_USD、ADTY_LOW_BALANCE_MAX_INFLIGHT（<=0 关闭）。
var (
	LowBalanceGuardUSD    = envFloat("ADTY_LOW_BALANCE_GUARD_USD", 5)
	LowBalanceMaxInflight = envInt("ADTY_LOW_BALANCE_MAX_INFLIGHT", 2)
	lowBalanceLease       = 15 * time.Minute

	lowBalMu    sync.Mutex
	lowBalLocal = map[int][]time.Time{}
)

func envFloat(k string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(k), 64); err == nil {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

// isLowBalance 余额是否进入并发防护区间。
func isLowBalance(userQuota int) bool {
	if LowBalanceMaxInflight <= 0 || LowBalanceGuardUSD <= 0 {
		return false
	}
	return float64(userQuota) < LowBalanceGuardUSD*common.QuotaPerUnit
}

func lowBalanceKey(userID int) string { return fmt.Sprintf("adty:lowbal_inflight:%d", userID) }

// acquireLowBalanceSlot 占一个并发位；成功返回幂等的释放函数，满了返回 nil,false。
func acquireLowBalanceSlot(userID int) (func(), bool) {
	max := LowBalanceMaxInflight
	if common.RedisEnabled && common.RDB != nil {
		ctx := context.Background()
		key := lowBalanceKey(userID)
		n, err := common.RDB.Incr(ctx, key).Result()
		if err == nil {
			common.RDB.Expire(ctx, key, lowBalanceLease)
			if n > int64(max) {
				common.RDB.Decr(ctx, key)
				return nil, false
			}
			var once sync.Once
			return func() {
				once.Do(func() {
					if v, err := common.RDB.Decr(context.Background(), key).Result(); err == nil && v < 0 {
						common.RDB.Set(context.Background(), key, 0, lowBalanceLease)
					}
				})
			}, true
		}
		// Redis 出错：退回本机计数
	}
	now := time.Now()
	lowBalMu.Lock()
	defer lowBalMu.Unlock()
	live := lowBalLocal[userID][:0]
	for _, t := range lowBalLocal[userID] {
		if now.Sub(t) < lowBalanceLease {
			live = append(live, t)
		}
	}
	if len(live) >= max {
		lowBalLocal[userID] = live
		return nil, false
	}
	live = append(live, now)
	lowBalLocal[userID] = live
	var once sync.Once
	return func() {
		once.Do(func() {
			lowBalMu.Lock()
			defer lowBalMu.Unlock()
			l := lowBalLocal[userID]
			for i, t := range l {
				if t.Equal(now) {
					lowBalLocal[userID] = append(l[:i], l[i+1:]...)
					break
				}
			}
			if len(lowBalLocal[userID]) == 0 {
				delete(lowBalLocal, userID)
			}
		})
	}, true
}

// resetLowBalanceSlotsForTest 清空本机计数（仅测试）。
func resetLowBalanceSlotsForTest() {
	lowBalMu.Lock()
	lowBalLocal = map[int][]time.Time{}
	lowBalMu.Unlock()
}
