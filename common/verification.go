package common

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

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

func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[purpose+key] = verificationValue{
		code: code,
		time: time.Now(),
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	return verificationCodeMatches(key, code, purpose, time.Now())
}

// ConsumeVerificationCodeWithKey 核对并立刻删掉验证码。同一个码只有一次能成功。
// 返回的签发时间用于失败时按原有效期放回去。
func ConsumeVerificationCodeWithKey(key string, code string, purpose string) (time.Time, bool) {
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

// RestoreVerificationCodeWithKey 在消费之后的写入失败时把原码放回去。
// 期间已经发了新码，或原码已经过期，就不再放回。
func RestoreVerificationCodeWithKey(key string, code string, purpose string, issued time.Time) {
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
