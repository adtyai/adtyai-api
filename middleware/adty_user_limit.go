package middleware

import (
	"errors"
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

var errAdtyLimitStoreUnavailable = errors.New("user limit store unavailable")

// adtyStore 开了 Redis 就只用 Redis。客户端还没接上时不能退回进程内存，否则多实例各算各的限额。
func adtyStore() (adtylimit.Store, error) {
	if adtyLimitStore != nil {
		return adtyLimitStore, nil
	}
	if common.RedisEnabled {
		if common.RDB == nil {
			return nil, errAdtyLimitStoreUnavailable
		}
		adtyLimitStoreOnce.Do(func() {
			if common.RDB != nil {
				adtyLimitStore = adtylimit.NewRedisStore(common.RDB)
			}
		})
		if adtyLimitStore == nil {
			return nil, errAdtyLimitStoreUnavailable
		}
		return adtyLimitStore, nil
	}
	adtyLimitStoreOnce.Do(func() {
		adtyLimitStore = adtylimit.NewMemoryStore()
	})
	return adtyLimitStore, nil
}

func init() {
	model.AdtyOnConsumeTokens = func(userId int, tokens int, withoutToken bool) {
		if withoutToken || tokens <= 0 || userId <= 0 || !adty_setting.GetLimitSetting().Enabled {
			return
		}
		store, err := adtyStore()
		if err != nil {
			common.SysError(common.LogText("adty tpm count skipped user=%d: %s", userId, err.Error()))
			return
		}
		if err := store.AddTPM(userId, adtylimit.MinuteOf(adtyLimitNow()), int64(tokens)); err != nil {
			common.SysError(common.LogText("adty tpm count failed user=%d: %s", userId, err.Error()))
		}
	}
}

// SetAdtyLimitStoreForTest 换成给定的计数和时钟，结束时还原。只给测试用。
func SetAdtyLimitStoreForTest(t interface{ Cleanup(func()) }, store adtylimit.Store, now func() time.Time) {
	_, _ = adtyStore()
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
		store, err := adtyStore()
		if err != nil {
			common.SysError(common.LogText("adty user limit store unavailable user=%d: %s", userID, err.Error()))
			message := i18n.T(c, i18n.MsgAdtyLimitUnavailable)
			if isMJ {
				abortWithMidjourneyMessage(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, message)
				return
			}
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, "rate_limit_unavailable")
			return
		}
		release, reason, err := adtylimit.Check(store, userID, adtyEffectiveLimits(userID), member, now)
		if err != nil {
			// A counter outage must not open the RPM/TPM/concurrency gate.
			common.SysError(common.LogText("adty user limit check failed user=%d: %s", userID, err.Error()))
			message := i18n.T(c, i18n.MsgAdtyLimitUnavailable)
			if isMJ {
				abortWithMidjourneyMessage(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, message)
				return
			}
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, "rate_limit_unavailable")
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
