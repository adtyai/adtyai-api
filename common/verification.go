package common

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrVerificationStoreUnavailable means a code cannot be stored, so it must not be sent.
var ErrVerificationStoreUnavailable = errors.New("verification code store unavailable")

type verificationValue struct {
	code     string
	time     time.Time
	failures int
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
	AccountDeletionPurpose   = "adty_account_delete"
	SecurityStepUpPurpose    = "adty_security_stepup"
)

// VerificationStatus is the outcome of checking a stored email verification code.
type VerificationStatus int

const (
	VerificationOK VerificationStatus = iota
	VerificationInvalid
	VerificationLocked
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

// Email-code guesses use MaxFailAttempts, the same official cap already used by
// TOTP / login 2FA (and matched by EmailBindingMaxAttempts). After that cap the
// current code stays unusable, including a later correct guess, until a new
// code is issued for the same purpose and key. LockoutDuration is not applied
// here; it remains a TOTP/2FA timed lock.

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

func formatVerificationPayload(issued time.Time, code string, failures int) string {
	return issued.UTC().Format(time.RFC3339Nano) + "\n" + code + "\n" + strconv.Itoa(failures)
}

func parseVerificationPayload(payload string) (time.Time, string, int, bool) {
	issuedText, rest, ok := strings.Cut(payload, "\n")
	if !ok {
		return time.Time{}, "", 0, false
	}
	code, failText, hasFailures := strings.Cut(rest, "\n")
	failures := 0
	if hasFailures {
		parsed, err := strconv.Atoi(failText)
		if err != nil || parsed < 0 {
			return time.Time{}, "", 0, false
		}
		failures = parsed
	}
	issued, err := time.Parse(time.RFC3339Nano, issuedText)
	if err != nil {
		return time.Time{}, "", 0, false
	}
	return issued, code, failures, true
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) error {
	if err := VerificationStoreReady(); err != nil {
		return err
	}
	issued := time.Now()
	if verificationStoreShared() {
		payload := formatVerificationPayload(issued, code, 0)
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
	return VerifyCodeWithKeyStatus(key, code, purpose) == VerificationOK
}

// VerifyCodeWithKeyStatus checks a stored code without consuming it on success.
// Wrong guesses and a locked code still increment or honor the attempt limit.
func VerifyCodeWithKeyStatus(key string, code string, purpose string) VerificationStatus {
	_, status := checkVerificationCode(key, code, purpose, false)
	return status
}

// ConsumeVerificationCodeWithKey 核对并立刻删掉验证码。同一个码只有一次能成功。
// 返回的签发时间用于失败时按原有效期放回去。
func ConsumeVerificationCodeWithKey(key string, code string, purpose string) (time.Time, bool) {
	issued, status := ConsumeVerificationCodeWithKeyStatus(key, code, purpose)
	return issued, status == VerificationOK
}

// ConsumeVerificationCodeWithKeyStatus is the status-aware form of
// ConsumeVerificationCodeWithKey. A locked code is not consumed as success.
func ConsumeVerificationCodeWithKeyStatus(key string, code string, purpose string) (time.Time, VerificationStatus) {
	return checkVerificationCode(key, code, purpose, true)
}

func checkVerificationCode(key, code, purpose string, consume bool) (time.Time, VerificationStatus) {
	if verificationStoreShared() {
		return consumeSharedVerification(key, code, purpose, consume)
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	return checkMemoryVerification(key, code, purpose, time.Now(), consume)
}

func checkMemoryVerification(key, code, purpose string, now time.Time, consume bool) (time.Time, VerificationStatus) {
	value, okay := verificationMap[purpose+key]
	if !okay {
		return time.Time{}, VerificationInvalid
	}
	if int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		delete(verificationMap, purpose+key)
		return time.Time{}, VerificationInvalid
	}
	if value.failures >= MaxFailAttempts {
		return time.Time{}, VerificationLocked
	}
	if code != value.code {
		value.failures++
		verificationMap[purpose+key] = value
		if value.failures >= MaxFailAttempts {
			return time.Time{}, VerificationLocked
		}
		return time.Time{}, VerificationInvalid
	}
	if consume {
		delete(verificationMap, purpose+key)
	}
	return value.time, VerificationOK
}

// consumeSharedVerificationLua atomically checks, counts failed guesses, and
// optionally consumes a matching code. ARGV: [code, maxAttempts, consumeFlag].
const consumeSharedVerificationLua = `
local payload = redis.call('GET', KEYS[1])
if not payload or payload == '' then
  return 'missing'
end
local sep = string.find(payload, '\n', 1, true)
if not sep then
  return 'invalid'
end
local issued = string.sub(payload, 1, sep - 1)
local rest = string.sub(payload, sep + 1)
local sep2 = string.find(rest, '\n', 1, true)
local stored
local attempts = 0
if sep2 then
  stored = string.sub(rest, 1, sep2 - 1)
  attempts = tonumber(string.sub(rest, sep2 + 1)) or 0
else
  stored = rest
end
local maxAttempts = tonumber(ARGV[2])
if attempts >= maxAttempts then
  return 'locked'
end
if stored == ARGV[1] then
  if ARGV[3] == '1' then
    redis.call('DEL', KEYS[1])
  end
  return 'ok\n' .. issued
end
attempts = attempts + 1
local newPayload = issued .. '\n' .. stored .. '\n' .. tostring(attempts)
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 then
  redis.call('SET', KEYS[1], newPayload, 'PX', ttl)
elseif ttl == -1 then
  redis.call('SET', KEYS[1], newPayload)
end
if attempts >= maxAttempts then
  return 'locked'
end
return 'mismatch'
`

func consumeSharedVerification(key, code, purpose string, deleteKey bool) (time.Time, VerificationStatus) {
	redisKey := verificationRedisKey(purpose, key)
	consumeFlag := 0
	if deleteKey {
		consumeFlag = 1
	}
	raw, err := RDB.Eval(context.Background(), consumeSharedVerificationLua, []string{redisKey}, code, MaxFailAttempts, consumeFlag).Result()
	if err != nil {
		return time.Time{}, VerificationInvalid
	}
	payload, _ := raw.(string)
	switch {
	case payload == "locked":
		return time.Time{}, VerificationLocked
	case strings.HasPrefix(payload, "ok\n"):
		issued, err := time.Parse(time.RFC3339Nano, strings.TrimPrefix(payload, "ok\n"))
		if err != nil {
			return time.Time{}, VerificationInvalid
		}
		return issued, VerificationOK
	default:
		return time.Time{}, VerificationInvalid
	}
}

// RestoreVerificationCodeWithKey 在消费之后的写入失败时把原码放回去。
// 期间已经发了新码，或原码已经过期，就不再放回。
func RestoreVerificationCodeWithKey(key string, code string, purpose string, issued time.Time) {
	if verificationStoreShared() {
		if code == "" || issued.IsZero() || int(time.Since(issued).Seconds()) >= VerificationValidMinutes*60 {
			return
		}
		payload := formatVerificationPayload(issued, code, 0)
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
