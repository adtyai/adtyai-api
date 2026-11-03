package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/adtylimit"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
)

// 按用户限制 RPM、TPM、并发（任务 7.4）。挂在上游 ModelRequestRateLimit 之后。

// adtyRateLimitedCode 是超限时报错里的 code，自己定义，不改上游 relaykit。
const adtyRateLimitedCode types.ErrorCode = "rate_limit_exceeded"

var (
	adtyLimitStoreOnce sync.Once
	adtyLimitStore     adtylimit.Store
	adtyLimitNow       = time.Now
)

// adtyStore 开了 Redis 用 Redis，否则用进程内存。
func adtyStore() adtylimit.Store {
	adtyLimitStoreOnce.Do(func() {
		if common.RedisEnabled && common.RDB != nil {
			adtyLimitStore = adtylimit.NewRedisStore(common.RDB)
		} else {
			adtyLimitStore = adtylimit.NewMemoryStore()
		}
	})
	return adtyLimitStore
}

func init() {
	model.AdtyOnConsumeTokens = func(userId int, tokens int, withoutToken bool) {
		if withoutToken || tokens <= 0 || userId <= 0 || !adty_setting.GetLimitSetting().Enabled {
			return
		}
		if err := adtyStore().AddTPM(userId, adtylimit.MinuteOf(adtyLimitNow()), int64(tokens)); err != nil {
			common.SysError(common.LogText("adty tpm count failed user=%d: %s", userId, err.Error()))
		}
	}
}

// SetAdtyLimitStoreForTest 换成给定的计数和时钟，结束时还原。只给测试用。
func SetAdtyLimitStoreForTest(t interface{ Cleanup(func()) }, store adtylimit.Store, now func() time.Time) {
	adtyStore()
	oldStore, oldNow := adtyLimitStore, adtyLimitNow
	adtyLimitStore, adtyLimitNow = store, now
	t.Cleanup(func() { adtyLimitStore, adtyLimitNow = oldStore, oldNow })
}

// adtyEffectiveLimits 是用户自己的限制（-1 跟默认）合上全局默认值。
func adtyEffectiveLimits(userID int) adtylimit.Limits {
	cfg := adty_setting.GetLimitSetting()
	limits := adtylimit.Limits{RPM: cfg.DefaultRPM, TPM: cfg.DefaultTPM, Concurrency: cfg.DefaultConcurrency}
	own, err := model.GetAdtyUserLimit(userID)
	if err != nil {
		return limits
	}
	if own.RPM >= 0 {
		limits.RPM = own.RPM
	}
	if own.TPM >= 0 {
		limits.TPM = own.TPM
	}
	if own.Concurrency >= 0 {
		limits.Concurrency = own.Concurrency
	}
	return limits
}

// AdtyUserLimit 是中转接口的按用户限制。midjourney 为真时按 Midjourney 的格式报错。
func AdtyUserLimit(midjourney ...bool) gin.HandlerFunc {
	isMJ := len(midjourney) > 0 && midjourney[0]
	return func(c *gin.Context) {
		userID := c.GetInt("id")
		if !adty_setting.GetLimitSetting().Enabled || userID <= 0 {
			c.Next()
			return
		}
		now := adtyLimitNow()
		member := c.GetString(common.RequestIdKey)
		if member == "" {
			member = strconv.FormatInt(now.UnixNano(), 36)
		}
		release, reason, err := adtylimit.Check(adtyStore(), userID, adtyEffectiveLimits(userID), member, now)
		if err != nil {
			// 计数出错时放行，不因为 Redis 抖动挡住所有请求。
			common.SysError(common.LogText("adty user limit check failed user=%d: %s", userID, err.Error()))
			c.Next()
			return
		}
		if reason != adtylimit.ReasonNone {
			adtyRejectLimited(c, reason, now, isMJ)
			return
		}
		if release != nil {
			defer release()
		}
		c.Next()
	}
}

func adtyRejectLimited(c *gin.Context, reason adtylimit.Reason, now time.Time, isMJ bool) {
	key := i18n.MsgAdtyLimitRPM
	switch reason {
	case adtylimit.ReasonTPM:
		key = i18n.MsgAdtyLimitTPM
	case adtylimit.ReasonConcurrency:
		key = i18n.MsgAdtyLimitConcurrency
	}
	retry := adtylimit.RetryAfter(now)
	if reason == adtylimit.ReasonConcurrency {
		retry = 1
	}
	c.Header("Retry-After", strconv.FormatInt(retry, 10))
	message := i18n.T(c, key)
	if isMJ {
		abortWithMidjourneyMessage(c, http.StatusTooManyRequests, 429, message)
		return
	}
	abortWithOpenAiMessage(c, http.StatusTooManyRequests, message, adtyRateLimitedCode)
}
