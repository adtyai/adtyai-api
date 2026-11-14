package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func oauthProtection() (*common.SSRFProtection, error) {
	return common.NewSSRFProtectionFromFetchSetting(false, false, false, nil, nil, nil, true)
}

func oauthDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	protection, err := oauthProtection()
	if err != nil {
		return nil, err
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if err := protection.ValidateResolvedIP(host, ip); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}
	var last error
	for _, item := range resolved {
		if item.IP == nil {
			continue
		}
		if err := protection.ValidateResolvedIP(host, item.IP); err != nil {
			return nil, err
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(item.IP.String(), portText))
		if dialErr == nil {
			return conn, nil
		}
		last = dialErr
	}
	if last != nil {
		return nil, last
	}
	return nil, fmt.Errorf("no usable address for %s", host)
}

func outboundClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         oauthDialContext,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			protection, err := oauthProtection()
			if err != nil {
				return err
			}
			return protection.ValidateURL(req.URL.String())
		},
	}
}

// endpointURLTestHook lets fixtures use a reserved host such as example.com
// that is rewritten onto a local httptest server. Ambient DNS may map that
// name to a blocked range (for example 198.18.0.0/15), which would otherwise
// make NewOIDCOAuthFlow report the provider as unconfigured.
var endpointURLTestHook struct {
	mu   sync.RWMutex
	host string
}

// AllowEndpointHostForTest skips DNS/IP filtering for host during endpoint
// URL checks. Other hosts still use the production SSRF validator. The hook
// is restored when cleanup runs.
func AllowEndpointHostForTest(cleanup func(func()), host string) {
	endpointURLTestHook.mu.Lock()
	previous := endpointURLTestHook.host
	endpointURLTestHook.host = host
	endpointURLTestHook.mu.Unlock()
	cleanup(func() {
		endpointURLTestHook.mu.Lock()
		endpointURLTestHook.host = previous
		endpointURLTestHook.mu.Unlock()
	})
}

func endpointURLTestHost() string {
	endpointURLTestHook.mu.RLock()
	defer endpointURLTestHook.mu.RUnlock()
	return endpointURLTestHook.host
}

// ValidateEndpointURL rejects loopback, private, and link-local OAuth endpoints.
func ValidateEndpointURL(raw string) error {
	if host := endpointURLTestHost(); host != "" {
		parsed, err := url.Parse(raw)
		if err != nil {
			return err
		}
		if (parsed.Scheme == "http" || parsed.Scheme == "https") &&
			parsed.Host != "" && strings.EqualFold(parsed.Hostname(), host) {
			return nil
		}
	}
	protection, err := oauthProtection()
	if err != nil {
		return err
	}
	return protection.ValidateURL(raw)
}

func doOutbound(ctx context.Context, req *http.Request) (*http.Response, error) {
	protection, err := oauthProtection()
	if err != nil {
		return nil, err
	}
	if err := protection.ValidateURL(req.URL.String()); err != nil {
		return nil, err
	}
	return outboundClient().Do(req.WithContext(ctx))
}
