package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// BrandLegacyCallbackHost is the old compile-time default that must never be
// used silently as a payment notify_url base (audit full-e2e P2-1).
const BrandLegacyCallbackHost = "https://api.adtyai.com"

// GetCallbackAddress returns the public base URL for payment notify_url.
// Prefer CustomCallbackAddress; fall back to ServerAddress. Never hard-codes a domain.
func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress != "" {
		return strings.TrimRight(operation_setting.CustomCallbackAddress, "/")
	}
	return strings.TrimRight(system_setting.ServerAddress, "/")
}

// CallbackAddressWarnReason returns a non-empty reason when callback base is
// missing or still the legacy brand host. Empty string means OK.
func CallbackAddressWarnReason() string {
	custom := strings.TrimSpace(operation_setting.CustomCallbackAddress)
	server := strings.TrimSpace(system_setting.ServerAddress)
	if custom != "" {
		if isBrandLegacyHost(custom) {
			return "CustomCallbackAddress still points at legacy brand host " + BrandLegacyCallbackHost + "; set a deployment-specific callback base"
		}
		return ""
	}
	if server == "" {
		return "CustomCallbackAddress and ServerAddress are both empty; payment notify_url will be relative/wrong — set CustomCallbackAddress or ServerAddress"
	}
	if isBrandLegacyHost(server) {
		return "CustomCallbackAddress empty and ServerAddress is legacy brand host " + BrandLegacyCallbackHost + "; local/unconfigured installs must not silently notify production"
	}
	return ""
}

func isBrandLegacyHost(addr string) bool {
	a := strings.TrimRight(strings.TrimSpace(addr), "/")
	return strings.EqualFold(a, BrandLegacyCallbackHost)
}

// WarnCallbackAddressMisconfig logs SysError when callback base is empty or
// still the legacy brand domain. Call once after model.InitOptionMap().
func WarnCallbackAddressMisconfig() {
	if reason := CallbackAddressWarnReason(); reason != "" {
		common.SysError(common.LogText("callback address misconfig: %s", reason))
	}
}
