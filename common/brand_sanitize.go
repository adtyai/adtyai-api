package common

import (
	"strings"
)

// DefaultSystemName is the adtyai product display name (replaces upstream "New API").
const DefaultSystemName = "ADTY AI"

// knownLegacySystemNames are exact OptionMap SystemName values left by upstream
// seed / old demos. Channel type label "New API" is unrelated and not listed here.
var knownLegacySystemNames = map[string]struct{}{
	"New API": {},
	"NewAPI":  {},
	"new-api": {},
	"NewApi":  {},
}

// SanitizeSystemName maps legacy New API brand leftovers to ADTY AI.
// Empty stays empty so callers can distinguish "unset" from a real name;
// InitOptionMap still seeds DefaultSystemName as the in-memory default.
func SanitizeSystemName(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return value
	}
	if _, ok := knownLegacySystemNames[v]; ok {
		return DefaultSystemName
	}
	return value
}

// SanitizeFooterHTML clears footer HTML that is only upstream New API brand residue
// (copyright / powered-by lines). Custom footers that merely mention the upstream
// project alongside other content are left untouched.
func SanitizeFooterHTML(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return value
	}
	lower := strings.ToLower(v)
	hasNewAPI := strings.Contains(lower, "new api") || strings.Contains(lower, "newapi")
	if !hasNewAPI {
		return value
	}
	// Short brand-only snippets (typical seeded © / Powered by lines).
	if len(v) <= 180 {
		return ""
	}
	// Pure attribution one-liners without other site content.
	if !strings.Contains(lower, "<a ") && !strings.Contains(lower, "href=") {
		if strings.Contains(lower, "powered") || strings.Contains(lower, "©") || strings.Contains(lower, "&copy;") {
			return ""
		}
	}
	return value
}

// SanitizeAbout clears About content that is only the upstream New API welcome blurb.
func SanitizeAbout(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return value
	}
	lower := strings.ToLower(v)
	if strings.Contains(lower, "welcome to our new api") {
		return ""
	}
	if len(v) <= 120 && (strings.Contains(lower, "new api") || strings.Contains(lower, "newapi")) {
		return ""
	}
	return value
}
