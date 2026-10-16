package setting

import (
	"encoding/json"
	"strings"
	"sync"
)

// adtyai A43 Telegram alert config (independent of official Telegram OAuth).
// AdtyTelegramBotToken ends with Token → A15 sensitive strip on GET /api/option/.
// Official common.TelegramBotToken / system_setting.telegram stay for OAuth only.
var (
	alertsMu sync.RWMutex

	AdtyTelegramBotToken   string
	TelegramUrgentChatId   string
	TelegramPendingChatId  string
	AdtyAlertUrgentEnabled = true
	AdtyAlertPendingEnabled = true
	// AdtyAlertKindEnabledJSON maps B7 category → enabled (default all true).
	AdtyAlertKindEnabledJSON = defaultAlertKindEnabledJSON()
)

func defaultAlertKindEnabledJSON() string {
	m := DefaultAlertKindEnabled()
	b, _ := json.Marshal(m)
	return string(b)
}

// DefaultAlertKindEnabled covers B7 urgent/pending major categories.
func DefaultAlertKindEnabled() map[string]bool {
	return map[string]bool{
		"site_core_db":                true,
		"payment_down":                true,
		"reconcile_mismatch":          true,
		"pay_unknown_escalate":        true,
		"hourly_rollup_missing":       true,
		"sensitive_burst":             true,
		"new_ip_sensitive":            true,
		"chatwoot_down":               true,
		"self_approve_over_threshold": true,
		"refund_urgent":               true,
		"epay_demote":                 true,
		"channel_disable":             true,
		"withdraw":                    true,
		"usdt_underpay":               true,
		"build_fail":                  true,
		"balance_adjust":              true,
		"self_approve":                true,
		"refund_each":                 true,
		"aff_negative":                true,
		"hourly_rollup_stale":         true,
		"sensitive_403_daily":         true,
		"ops_job_stuck":               true,
		"disk":                        true,
		"channel_balance":             true,
	}
}

func GetAdtyTelegramBotToken() string {
	alertsMu.RLock()
	defer alertsMu.RUnlock()
	return AdtyTelegramBotToken
}

func SetAdtyTelegramBotToken(v string) {
	alertsMu.Lock()
	defer alertsMu.Unlock()
	AdtyTelegramBotToken = v
}

func GetTelegramUrgentChatId() string {
	alertsMu.RLock()
	defer alertsMu.RUnlock()
	return strings.TrimSpace(TelegramUrgentChatId)
}

func SetTelegramUrgentChatId(v string) {
	alertsMu.Lock()
	defer alertsMu.Unlock()
	TelegramUrgentChatId = strings.TrimSpace(v)
}

func GetTelegramPendingChatId() string {
	alertsMu.RLock()
	defer alertsMu.RUnlock()
	return strings.TrimSpace(TelegramPendingChatId)
}

func SetTelegramPendingChatId(v string) {
	alertsMu.Lock()
	defer alertsMu.Unlock()
	TelegramPendingChatId = strings.TrimSpace(v)
}

func IsAdtyAlertUrgentEnabled() bool {
	alertsMu.RLock()
	defer alertsMu.RUnlock()
	return AdtyAlertUrgentEnabled
}

func SetAdtyAlertUrgentEnabled(v bool) {
	alertsMu.Lock()
	defer alertsMu.Unlock()
	AdtyAlertUrgentEnabled = v
}

func IsAdtyAlertPendingEnabled() bool {
	alertsMu.RLock()
	defer alertsMu.RUnlock()
	return AdtyAlertPendingEnabled
}

func SetAdtyAlertPendingEnabled(v bool) {
	alertsMu.Lock()
	defer alertsMu.Unlock()
	AdtyAlertPendingEnabled = v
}

func GetAdtyAlertKindEnabled() map[string]bool {
	alertsMu.RLock()
	raw := AdtyAlertKindEnabledJSON
	alertsMu.RUnlock()
	out := DefaultAlertKindEnabled()
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var parsed map[string]bool
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return out
	}
	for k, v := range parsed {
		out[k] = v
	}
	return out
}

func SetAdtyAlertKindEnabledJSON(v string) {
	alertsMu.Lock()
	defer alertsMu.Unlock()
	if strings.TrimSpace(v) == "" {
		AdtyAlertKindEnabledJSON = defaultAlertKindEnabledJSON()
		return
	}
	AdtyAlertKindEnabledJSON = v
}

func GetAdtyAlertKindEnabledJSON() string {
	alertsMu.RLock()
	defer alertsMu.RUnlock()
	return AdtyAlertKindEnabledJSON
}

// IsAdtyAlertBotConfigured: token + at least one chat id.
func IsAdtyAlertBotConfigured() bool {
	return strings.TrimSpace(GetAdtyTelegramBotToken()) != "" &&
		(GetTelegramUrgentChatId() != "" || GetTelegramPendingChatId() != "")
}
