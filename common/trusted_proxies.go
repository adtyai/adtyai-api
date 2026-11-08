package common

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

var defaultTrustedProxyCIDRs = []string{
	"127.0.0.0/8",
	"::1",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
}

var trustedRequestProxyCIDRs atomic.Pointer[[]netip.Prefix]

// ResolveTrustedProxies parses TRUSTED_PROXIES without applying it to an
// engine. The returned slice can be reused by the outer and plugin engines.
func ResolveTrustedProxies(raw string) (trustedProxies []string, usedDefaults bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil, false, nil
	}
	if strings.EqualFold(raw, "private") {
		return append([]string(nil), defaultTrustedProxyCIDRs...), false, nil
	}
	if strings.EqualFold(raw, "none") {
		return nil, false, nil
	}

	parts := strings.Split(raw, ",")
	trustedProxies = make([]string, 0, len(parts))
	for _, part := range parts {
		trustedProxy := strings.TrimSpace(part)
		if trustedProxy == "" {
			continue
		}
		if strings.EqualFold(trustedProxy, "none") {
			return nil, false, errors.New("TRUSTED_PROXIES=none must be used alone")
		}
		trustedProxies = append(trustedProxies, trustedProxy)
	}
	if len(trustedProxies) == 0 {
		return nil, false, errors.New("TRUSTED_PROXIES does not contain an IP address or CIDR")
	}
	return trustedProxies, false, nil
}

func ConfigureTrustedProxies(engine *gin.Engine, trustedProxies []string) error {
	prefixes := make([]netip.Prefix, 0, len(trustedProxies))
	for _, value := range trustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil {
				return fmt.Errorf("invalid TRUSTED_PROXIES: %w", addressErr)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		prefixes = append(prefixes, prefix)
	}
	if err := engine.SetTrustedProxies(trustedProxies); err != nil {
		return fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}
	trustedRequestProxyCIDRs.Store(&prefixes)
	return nil
}

// RequestScheme accepts forwarded transport metadata only from the configured
// immediate proxy. ClientIP is unsuitable here: it identifies the visitor.
func RequestScheme(request *http.Request) (string, bool) {
	if request.TLS != nil {
		return "https", true
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return "http", true
	}
	address, err := netip.ParseAddr(host)
	prefixes := trustedRequestProxyCIDRs.Load()
	if err != nil || prefixes == nil {
		return "http", true
	}
	trusted := false
	for _, prefix := range *prefixes {
		if prefix.Contains(address.Unmap()) {
			trusted = true
			break
		}
	}
	if !trusted {
		return "http", true
	}
	values := request.Header.Values("X-Forwarded-Proto")
	if len(values) == 0 {
		return "http", true
	}
	if len(values) != 1 || (values[0] != "http" && values[0] != "https") {
		return "", false
	}
	return values[0], true
}
