package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func isPaymentComplianceConfirmed() bool {
	return operation_setting.IsPaymentComplianceConfirmed()
}

func isStripeTopUpEnabled() bool {
	// Stripe checkout is retired. Credentials may remain in settings, but
	// customers must not be offered or charged through this entry.
	return false
}

func isStripeWebhookConfigured() bool {
	return strings.TrimSpace(setting.StripeWebhookSecret) != ""
}

func isStripeWebhookEnabled() bool {
	// Retired Stripe webhooks must not credit a wallet.
	return false
}

func isCreemTopUpEnabled() bool {
	// Creem checkout is retired. Credentials may remain in settings, but
	// customers must not be offered or charged through this entry.
	return false
}

func isCreemWebhookConfigured() bool {
	return strings.TrimSpace(setting.CreemWebhookSecret) != ""
}

func isCreemWebhookEnabled() bool {
	// Retired Creem webhooks must not credit a wallet.
	return false
}

func isWaffoTopUpEnabled() bool {
	// Waffo checkout is retired. Credentials may remain in settings, but
	// customers must not be offered or charged through this entry.
	return false
}

func isWaffoWebhookConfigured() bool {
	return isWaffoEnvironmentConfigured(true) || isWaffoEnvironmentConfigured(false)
}

func isWaffoEnvironmentConfigured(sandbox bool) bool {
	if sandbox {
		return strings.TrimSpace(setting.WaffoSandboxApiKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPrivateKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPublicCert) != ""
	}

	return strings.TrimSpace(setting.WaffoApiKey) != "" &&
		strings.TrimSpace(setting.WaffoPrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPublicCert) != ""
}

func isWaffoWebhookEnabled() bool {
	// Retired Waffo webhooks must not credit a wallet.
	return false
}

func isWaffoPancakeTopUpEnabled() bool {
	// Waffo Pancake checkout is retired. Credentials may remain in
	// settings, but customers must not be offered or charged through this entry.
	return false
}

func isWaffoPancakeWebhookConfigured() bool {
	return true // Verification uses the SDK's test/prod public keys; each order binds its store.
}

func isWaffoPancakeWebhookEnabled() bool {
	// Retired Waffo Pancake webhooks must not credit a wallet.
	return false
}

func isEpayTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return isEpayWebhookConfigured() && len(operation_setting.PayMethods) > 0
}

func isEpayWebhookConfigured() bool {
	return strings.TrimSpace(operation_setting.PayAddress) != "" &&
		strings.TrimSpace(operation_setting.EpayId) != "" &&
		strings.TrimSpace(operation_setting.EpayKey) != ""
}

func isEpayWebhookEnabled() bool {
	return isEpayWebhookConfigured()
}
