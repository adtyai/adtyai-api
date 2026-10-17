package service

import (
	"bytes"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func withCallbackAddrs(t *testing.T, custom, server string) {
	t.Helper()
	prevC, prevS := operation_setting.CustomCallbackAddress, system_setting.ServerAddress
	operation_setting.CustomCallbackAddress = custom
	system_setting.ServerAddress = server
	t.Cleanup(func() {
		operation_setting.CustomCallbackAddress = prevC
		system_setting.ServerAddress = prevS
	})
}

func TestGetCallbackAddressPrefersCustom(t *testing.T) {
	withCallbackAddrs(t, "https://cb.example/hook/", "https://server.example")
	if got := GetCallbackAddress(); got != "https://cb.example/hook" {
		t.Fatalf("want custom trimmed, got %q", got)
	}
}

func TestGetCallbackAddressFallsBackToServer(t *testing.T) {
	withCallbackAddrs(t, "", "https://server.example/")
	if got := GetCallbackAddress(); got != "https://server.example" {
		t.Fatalf("want server, got %q", got)
	}
}

func TestGetCallbackAddressEmptyWhenUnset(t *testing.T) {
	withCallbackAddrs(t, "", "")
	if got := GetCallbackAddress(); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestCallbackAddressWarnReasonBrandDefault(t *testing.T) {
	withCallbackAddrs(t, "", BrandLegacyCallbackHost)
	reason := CallbackAddressWarnReason()
	if reason == "" || !strings.Contains(reason, "legacy brand") {
		t.Fatalf("want brand-default warn, got %q", reason)
	}
	withCallbackAddrs(t, "", "")
	reason = CallbackAddressWarnReason()
	if reason == "" || !strings.Contains(reason, "both empty") {
		t.Fatalf("want empty warn, got %q", reason)
	}
	withCallbackAddrs(t, "https://pay.example", "https://server.example")
	if reason = CallbackAddressWarnReason(); reason != "" {
		t.Fatalf("want no warn, got %q", reason)
	}
}

func TestWarnCallbackAddressMisconfigObservable(t *testing.T) {
	withCallbackAddrs(t, "", BrandLegacyCallbackHost)
	var buf bytes.Buffer
	prev := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &buf
	t.Cleanup(func() { gin.DefaultErrorWriter = prev })
	WarnCallbackAddressMisconfig()
	out := buf.String()
	if !strings.Contains(out, "callback address misconfig") || !strings.Contains(out, "legacy brand") {
		t.Fatalf("want SysError warn, got %q", out)
	}
	_ = common.SystemName // keep common import used if LogText path needs package warm
}
