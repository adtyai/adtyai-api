package smartrouting

import (
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// ErrClass classification for connectivity / breaker.
type ErrClass string

const (
	ClassOK           ErrClass = "ok"
	ClassBanned       ErrClass = "banned"        // 401/403 → immediate break
	ClassRateLimit    ErrClass = "rate_limit"    // 429 → 0.5 fail, no single-shot break
	ClassServer       ErrClass = "server"         // 5xx / timeout / empty / stream cut
	ClassBalance      ErrClass = "balance"        // insufficient quota
	ClassUserError    ErrClass = "user_error"     // 400 / moderation / client cancel — not channel fail
	ClassAuthMismatch ErrClass = "auth_mismatch" // fake model — not connectivity fail
	ClassUnknown      ErrClass = "unknown"
)

// Classify maps a relay error to ErrClass.
func Classify(err *types.NewAPIError) ErrClass {
	if err == nil {
		return ClassOK
	}
	code := err.StatusCode
	msg := strings.ToLower(err.Error())
	switch {
	case code == 400 || code == 422:
		return ClassUserError
	case code == 401 || code == 403:
		return ClassBanned
	case code == 402 || strings.Contains(msg, "insufficient") || strings.Contains(msg, "余额不足") ||
		strings.Contains(msg, "credit") && strings.Contains(msg, "quota"):
		return ClassBalance
	case code == 429:
		return ClassRateLimit
	case code >= 500 || code == 408 || code == 504:
		return ClassServer
	case types.IsSkipRetryError(err):
		// often client cancel / content policy
		if strings.Contains(msg, "cancel") || strings.Contains(msg, "moderation") ||
			strings.Contains(msg, "content_policy") || strings.Contains(msg, "content filter") {
			return ClassUserError
		}
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "empty response") ||
		strings.Contains(msg, "stream") && strings.Contains(msg, "interrupt") {
		return ClassServer
	}
	if strings.Contains(msg, "invalid key") || strings.Contains(msg, "deactivated") ||
		strings.Contains(msg, "banned") || strings.Contains(msg, "账号") {
		return ClassBanned
	}
	return ClassUnknown
}

// FailWeight returns connectivity failure weight (0 = not a channel failure).
func FailWeight(c ErrClass) float64 {
	switch c {
	case ClassOK, ClassUserError, ClassAuthMismatch:
		return 0
	case ClassRateLimit:
		return 0.5
	default:
		return 1.0
	}
}
