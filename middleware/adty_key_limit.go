package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdtyKeyRateLimit enforces a per-key request cap when one is configured.
// A key with no row is not extra-limited. This does not replace the user limit.
func AdtyKeyRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenID := c.GetInt("token_id")
		if tokenID <= 0 {
			c.Next()
			return
		}
		limit, err := model.GetAdtyKeyLimit(tokenID)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgAdtyLimitUnavailable), "rate_limit_unavailable")
			return
		}
		if limit.PerMinute <= 0 {
			c.Next()
			return
		}
		allowed, err := adtyKeyRateAllow(tokenID, limit.PerMinute, time.Now())
		if err != nil {
			common.SysError(common.LogText("adty key limit store unavailable token=%d: %s", tokenID, err.Error()))
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgAdtyLimitUnavailable), "rate_limit_unavailable")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, i18n.T(c, i18n.MsgAdtyLimitRPM))
			return
		}
		c.Next()
	}
}

// adtyKeyRateBucket is one key's counter inside a single minute window. The minute is
// stored next to the count so an expired window can be dropped instead of kept forever.
type adtyKeyRateBucket struct {
	minute int64
	count  int
}

var (
	adtyKeyRateMemory            = map[int]adtyKeyRateBucket{}
	adtyKeyRateMemorySwept int64 = -1
	adtyKeyRateMemoryMu    sync.Mutex
)

func adtyKeyRateAllow(tokenID, perMinute int, now time.Time) (bool, error) {
	if common.RedisEnabled {
		if common.RDB == nil {
			return false, errAdtyLimitStoreUnavailable
		}
		key := fmt.Sprintf("ADTY:KEYRPM:%d:%d", tokenID, now.Unix()/60)
		count, err := common.RDB.Incr(context.Background(), key).Result()
		if err != nil {
			return false, err
		}
		if count == 1 {
			_ = common.RDB.Expire(context.Background(), key, 2*time.Minute).Err()
		}
		return count <= int64(perMinute), nil
	}
	minute := now.Unix() / 60
	adtyKeyRateMemoryMu.Lock()
	defer adtyKeyRateMemoryMu.Unlock()
	// The Redis branch gives every per-minute bucket a two-minute TTL. The memory branch
	// has to expire the same way, otherwise it keeps one bucket per key per minute for the
	// life of the process. Sweeping only when the minute changes keeps that to one pass per
	// minute, and the map then holds at most two windows instead of every window ever seen.
	if minute != adtyKeyRateMemorySwept {
		adtyKeyRateMemorySwept = minute
		for id, bucket := range adtyKeyRateMemory {
			if bucket.minute < minute-1 {
				delete(adtyKeyRateMemory, id)
			}
		}
	}
	bucket := adtyKeyRateMemory[tokenID]
	if bucket.minute != minute {
		bucket = adtyKeyRateBucket{minute: minute}
	}
	bucket.count++
	adtyKeyRateMemory[tokenID] = bucket
	return bucket.count <= perMinute, nil
}
