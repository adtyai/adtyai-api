package setting

import (
	"strings"
	"sync"
)

// adtyai BEpusdt / USDT gateway config (options).
// BepusdtApiToken is sensitive (suffix Token): GET /api/option/ never returns plaintext.
var (
	bepusdtMu sync.RWMutex

	BepusdtBaseURL             string
	BepusdtApiToken            string
	BepusdtLegacyNotifyEnabled bool
	BepusdtTradeType           string = "usdt.trc20"
	BepusdtCurrency            string = "USD" // order amount is USD tiers; getter ignores other values
	BepusdtEnabledChains       string = `["trc20","bep20","polygon","xlayer"]`
	BepusdtNotifyCallbackBase  string // optional override; empty → ServerAddress / callback helper
)

func GetBepusdtBaseURL() string {
	bepusdtMu.RLock()
	defer bepusdtMu.RUnlock()
	return strings.TrimSpace(BepusdtBaseURL)
}

func GetBepusdtApiToken() string {
	bepusdtMu.RLock()
	defer bepusdtMu.RUnlock()
	return BepusdtApiToken
}

func IsBepusdtLegacyNotifyEnabled() bool {
	bepusdtMu.RLock()
	defer bepusdtMu.RUnlock()
	return BepusdtLegacyNotifyEnabled
}

func GetBepusdtTradeType() string {
	bepusdtMu.RLock()
	defer bepusdtMu.RUnlock()
	t := strings.TrimSpace(strings.ToLower(BepusdtTradeType))
	if t == "" {
		return "usdt.trc20"
	}
	return t
}

// GetBepusdtCurrency is always USD. Tiers are dollar amounts, so a stored
// currency must not change what BEpusdt asks the user to pay.
func GetBepusdtCurrency() string {
	return "USD"
}

func SetBepusdtBaseURL(v string) {
	bepusdtMu.Lock()
	defer bepusdtMu.Unlock()
	BepusdtBaseURL = strings.TrimSpace(v)
}

func SetBepusdtApiToken(v string) {
	bepusdtMu.Lock()
	defer bepusdtMu.Unlock()
	BepusdtApiToken = v
}

func SetBepusdtLegacyNotifyEnabled(v bool) {
	bepusdtMu.Lock()
	defer bepusdtMu.Unlock()
	BepusdtLegacyNotifyEnabled = v
}

func SetBepusdtTradeType(v string) {
	bepusdtMu.Lock()
	defer bepusdtMu.Unlock()
	BepusdtTradeType = v
}

func SetBepusdtCurrency(v string) {
	bepusdtMu.Lock()
	defer bepusdtMu.Unlock()
	BepusdtCurrency = v
}

// IsBepusdtConfigured: API address + token both set (probe is separate).
func IsBepusdtConfigured() bool {
	return GetBepusdtBaseURL() != "" && strings.TrimSpace(GetBepusdtApiToken()) != ""
}
