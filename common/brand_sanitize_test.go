package common

import "testing"

func TestSanitizeSystemName(t *testing.T) {
	if got := SanitizeSystemName("New API"); got != DefaultSystemName {
		t.Fatalf("got %q", got)
	}
	if got := SanitizeSystemName("ADTY AI"); got != "ADTY AI" {
		t.Fatalf("got %q", got)
	}
	if got := SanitizeSystemName(""); got != "" {
		t.Fatalf("empty should stay empty, got %q", got)
	}
}

func TestSanitizeFooterHTML(t *testing.T) {
	if got := SanitizeFooterHTML("© 2025 New API. All rights reserved."); got != "" {
		t.Fatalf("expected clear, got %q", got)
	}
	custom := `<p>欢迎使用 ADTY AI</p><p>基于 <a href="https://github.com/QuantumNous/new-api">new-api</a></p>`
	if got := SanitizeFooterHTML(custom); got != custom {
		t.Fatalf("custom footer should keep, got %q", got)
	}
}

func TestSanitizeAbout(t *testing.T) {
	if got := SanitizeAbout("Welcome to our New API..."); got != "" {
		t.Fatalf("expected clear, got %q", got)
	}
}
