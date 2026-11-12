package service

import "regexp"

var adtySecretText = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_\-]{8,}|bot\d{6,}:[A-Za-z0-9_\-]{10,}|\d{6,12}:[A-Za-z0-9_\-]{20,}|api[_-]?key\s*[:=]\s*\S+|password\s*[:=]\s*\S+|验证码\s*[:=]\s*\S+)`)

func redactAdtySecrets(text string) string {
	return adtySecretText.ReplaceAllString(text, "[redacted]")
}

func adtyTextHasSecret(text string) bool {
	return adtySecretText.MatchString(text)
}
