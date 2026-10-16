package model

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting"
)

// A44: configured external probes (epay / bepusdt / chatwoot). Reachability only — no keys, no orders.

var chatwootScriptSrcRe = regexp.MustCompile(`(?i)(?:src\s*=\s*["']|BASE_URL\s*=\s*["'])(https?://[^"'\s]+)`)
var chatwootURLRe = regexp.MustCompile(`https?://[^"'\s<>;]+`)

// ChatwootProbeURL prefers ChatwootBaseUrl, else first http(s) URL in ChatwootScript.
func ChatwootProbeURL() string {
	if b := chatwootOption(OptChatwootBaseUrl); b != "" {
		return b
	}
	s := chatwootOption(OptChatwootScript)
	if s == "" {
		return ""
	}
	if m := chatwootScriptSrcRe.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return chatwootURLRe.FindString(s)
}

// sanitizeProbeErr returns a short, secret-free reason (no URL / query echoed).
func sanitizeProbeErr(err error) string {
	if err == nil {
		return ""
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return "timeout"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"):
		return "connection refused"
	case strings.Contains(msg, "no such host"):
		return "dns error"
	case strings.Contains(msg, "certificate") || strings.Contains(msg, "tls"):
		return "tls error"
	case strings.Contains(msg, "unsupported protocol") || strings.Contains(msg, "invalid url") || strings.Contains(msg, "missing protocol"):
		return "invalid url"
	}
	return "request failed"
}

// ProbeEpayInstances probes each enabled epay instance's gateway concurrently; target_id = instance id.
func ProbeEpayInstances(ctx context.Context) error {
	rows, err := ListEpayInstances()
	if err != nil {
		return err
	}
	fns := make([]func(context.Context) error, 0, len(rows))
	for i := range rows {
		inst := rows[i]
		if inst.Status != EpayInstanceStatusEnabled {
			continue
		}
		id := strconv.FormatInt(inst.Id, 10)
		addr := inst.PayAddress
		fns = append(fns, func(c context.Context) error {
			_, err := ProbeHTTPStub(c, HealthTargetEpay, id, addr)
			return err
		})
	}
	return runProbesConcurrent(ctx, fns)
}

// RunConfiguredProbes: postgres + redis + epay(enabled instances) + bepusdt + chatwoot，全部并发。
// 单项超时见 HealthProbe*Timeout；ok=false 不取消兄弟项；仅落库等硬错误返回。
func RunConfiguredProbes(ctx context.Context) error {
	rows, err := ListEpayInstances()
	if err != nil {
		return err
	}
	fns := make([]func(context.Context) error, 0, 4+len(rows))
	fns = append(fns,
		func(c context.Context) error { _, err := ProbePostgres(c); return err },
		func(c context.Context) error { _, err := ProbeRedis(c); return err },
	)
	for i := range rows {
		inst := rows[i]
		if inst.Status != EpayInstanceStatusEnabled {
			continue
		}
		id := strconv.FormatInt(inst.Id, 10)
		addr := inst.PayAddress
		fns = append(fns, func(c context.Context) error {
			_, err := ProbeHTTPStub(c, HealthTargetEpay, id, addr)
			return err
		})
	}
	bepURL := setting.GetBepusdtBaseURL()
	cwURL := ChatwootProbeURL()
	fns = append(fns,
		func(c context.Context) error {
			_, err := ProbeHTTPStub(c, HealthTargetBepusdt, HealthTargetIDMain, bepURL)
			return err
		},
		func(c context.Context) error {
			_, err := ProbeHTTPStub(c, HealthTargetChatwoot, HealthTargetIDMain, cwURL)
			return err
		},
	)
	return runProbesConcurrent(ctx, fns)
}
