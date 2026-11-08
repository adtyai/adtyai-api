package common

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrVerificationStoreUnavailable means a code cannot be stored, so it must not be sent.
var ErrVerificationStoreUnavailable = errors.New("verification code store unavailable")

type verificationValue struct {
	code string
	time time.Time
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

func verificationRedisKey(purpose, key string) string {
	return "adty:verify:" + purpose + ":" + key
}

func verificationStoreShared() bool {
	return RedisEnabled && RDB != nil
}

// VerificationStoreReady reports whether a code can be stored. Redis enabled
// without a client must not fall back to process memory.
func VerificationStoreReady() error {
	if RedisEnabled && RDB == nil {
		return ErrVerificationStoreUnavailable
	}
	return nil
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) error {
	if err := VerificationStoreReady(); err != nil {
		return err
	}
	issued := time.Now()
	if verificationStoreShared() {
		payload := issued.UTC().Format(time.RFC3339Nano) + "\n" + code
		if err := RDB.Set(context.Background(), verificationRedisKey(purpose, key), payload, time.Duration(VerificationValidMinutes)*time.Minute).Err(); err != nil {
			SysError(LogText("verification code store failed: %s", err.Error()))
			return ErrVerificationStoreUnavailable
		}
		return nil
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[purpose+key] = verificationValue{
		code: code,
		time: issued,
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
	return nil
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	if verificationStoreShared() {
		_, ok := consumeSharedVerification(key, code, purpose, false)
		return ok
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	return verificationCodeMatches(key, code, purpose, time.Now())
}

// ConsumeVerificationCodeWithKey 核对并立刻删掉验证码。同一个码只有一次能成功。
// 返回的签发时间用于失败时按原有效期放回去。
func ConsumeVerificationCodeWithKey(key string, code string, purpose string) (time.Time, bool) {
	if verificationStoreShared() {
		return consumeSharedVerification(key, code, purpose, true)
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	now := time.Now()
	if !verificationCodeMatches(key, code, purpose, now) {
		return time.Time{}, false
	}
	issued := verificationMap[purpose+key].time
	delete(verificationMap, purpose+key)
	return issued, true
}

func consumeSharedVerification(key, code, purpose string, deleteKey bool) (time.Time, bool) {
	redisKey := verificationRedisKey(purpose, key)
	var payload string
	var err error
	if deleteKey {
		const consume = `
local payload = redis.call('GET', KEYS[1])
if not payload then return '' end
local separator = string.find(payload, '\n', 1, true)
if not separator or string.sub(payload, separator + 1) ~= ARGV[1] then return '' end
redis.call('DEL', KEYS[1])
return payload`
		payload, err = RDB.Eval(context.Background(), consume, []string{redisKey}, code).Text()
	} else {
		payload, err = RDB.Get(context.Background(), redisKey).Result()
	}
	if err != nil || payload == "" {
		return time.Time{}, false
	}
	issuedText, stored, ok := strings.Cut(payload, "\n")
	if !ok || stored != code {
		return time.Time{}, false
	}
	issued, err := time.Parse(time.RFC3339Nano, issuedText)
	if err != nil {
		return time.Time{}, false
	}
	return issued, true
}

// RestoreVerificationCodeWithKey 在消费之后的写入失败时把原码放回去。
// 期间已经发了新码，或原码已经过期，就不再放回。
func RestoreVerificationCodeWithKey(key string, code string, purpose string, issued time.Time) {
	if verificationStoreShared() {
		if code == "" || issued.IsZero() || int(time.Since(issued).Seconds()) >= VerificationValidMinutes*60 {
			return
		}
		payload := issued.UTC().Format(time.RFC3339Nano) + "\n" + code
		ttl := time.Duration(VerificationValidMinutes)*time.Minute - time.Since(issued)
		if ttl <= 0 {
			return
		}
		if err := RDB.SetNX(context.Background(), verificationRedisKey(purpose, key), payload, ttl).Err(); err != nil {
			SysError(LogText("verification code restore failed: %s", err.Error()))
		}
		return
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	if code == "" || issued.IsZero() {
		return
	}
	if _, exists := verificationMap[purpose+key]; exists {
		return
	}
	if int(time.Since(issued).Seconds()) >= VerificationValidMinutes*60 {
		return
	}
	verificationMap[purpose+key] = verificationValue{code: code, time: issued}
}

func verificationCodeMatches(key string, code string, purpose string, now time.Time) bool {
	value, okay := verificationMap[purpose+key]
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		return false
	}
	return code == value.code
}

func DeleteKey(key string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	delete(verificationMap, purpose+key)
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= VerificationValidMinutes*60 {
			delete(verificationMap, key)
		}
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
