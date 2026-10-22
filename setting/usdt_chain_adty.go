package setting

import (
	"errors"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// USDT chains users can pay on. TradeType values match BEpusdt
// (github.com/v03413/bepusdt app/model/order.go).
const (
	UsdtChainTRC20   = "trc20"
	UsdtChainBEP20   = "bep20"
	UsdtChainPolygon = "polygon"
	UsdtChainXLayer  = "xlayer"
)

// ErrUsdtChainRequired means a USDT order did not name a network.
var ErrUsdtChainRequired = errors.New("usdt chain required")

// ErrUsdtChainInvalid means the network id is not one of the four supported chains.
var ErrUsdtChainInvalid = errors.New("usdt chain invalid")

// ErrUsdtChainDisabled means the network is known but the admin has turned it off.
var ErrUsdtChainDisabled = errors.New("usdt chain disabled")

// ErrBepusdtEnabledChainsInvalid means the stored option is not a JSON array of known chain ids.
var ErrBepusdtEnabledChainsInvalid = errors.New("bepusdt enabled chains invalid")

// ErrUsdtPayoutAddressInvalid means the address does not match the selected payout network.
var ErrUsdtPayoutAddressInvalid = errors.New("usdt payout address invalid")

// TRON addresses are base58 and start with T. EVM chains share one address shape;
// the selected chain is what tells the operator which network to pay on.
var (
	tronPayoutAddress = regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`)
	evmPayoutAddress  = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
)

// UsdtChain is one checkout network. Network is the user-facing warning name.
type UsdtChain struct {
	ID        string `json:"id"`
	TradeType string `json:"trade_type"`
	Label     string `json:"label"`
	Network   string `json:"network"`
}

// Catalog order is the only order we persist, so toggles do not reshuffle.
var usdtChainCatalog = []UsdtChain{
	{ID: UsdtChainTRC20, TradeType: "usdt.trc20", Label: "USDT-TRC20", Network: "TRON (TRC20)"},
	{ID: UsdtChainBEP20, TradeType: "usdt.bep20", Label: "USDT-BEP20", Network: "BNB Smart Chain (BEP20)"},
	{ID: UsdtChainPolygon, TradeType: "usdt.polygon", Label: "USDT-Polygon", Network: "Polygon"},
	{ID: UsdtChainXLayer, TradeType: "usdt.xlayer", Label: "USDT-X Layer", Network: "X Layer"},
}

// AllUsdtChains returns the four supported networks in catalog order.
func AllUsdtChains() []UsdtChain {
	out := make([]UsdtChain, len(usdtChainCatalog))
	copy(out, usdtChainCatalog)
	return out
}

// LookupUsdtChain returns a catalog entry. Empty and unknown ids are not found.
func LookupUsdtChain(id string) (UsdtChain, bool) {
	id = normalizeUsdtChainID(id)
	for _, chain := range usdtChainCatalog {
		if chain.ID == id {
			return chain, true
		}
	}
	return UsdtChain{}, false
}

// TradeTypeForUsdtChain maps a stored order chain to the BEpusdt trade_type.
// Empty means the order has no chain (legacy). Unknown non-empty ids return "".
func TradeTypeForUsdtChain(id string) string {
	chain, ok := LookupUsdtChain(id)
	if !ok {
		return ""
	}
	return chain.TradeType
}

// RequireEnabledUsdtChain checks a new USDT order. In-flight orders keep the
// chain they were created with even if an admin later turns that chain off.
func RequireEnabledUsdtChain(raw string) (string, error) {
	id := normalizeUsdtChainID(raw)
	if id == "" {
		return "", ErrUsdtChainRequired
	}
	if _, ok := LookupUsdtChain(id); !ok {
		return "", ErrUsdtChainInvalid
	}
	for _, enabled := range GetBepusdtEnabledChainIDs() {
		if enabled == id {
			return id, nil
		}
	}
	return "", ErrUsdtChainDisabled
}

// ValidateUsdtPayoutAddress checks a withdrawal address against the four-chain catalog.
// Checkout enablement does not apply: a user can be paid on a network the site no longer collects on.
// EVM chains cannot be told apart by the address, so the caller must pass the chain the user confirmed.
func ValidateUsdtPayoutAddress(chainRaw, addressRaw string) (string, string, error) {
	id := normalizeUsdtChainID(chainRaw)
	if id == "" {
		return "", "", ErrUsdtChainRequired
	}
	if _, ok := LookupUsdtChain(id); !ok {
		return "", "", ErrUsdtChainInvalid
	}
	address := strings.TrimSpace(addressRaw)
	var matched bool
	if id == UsdtChainTRC20 {
		matched = tronPayoutAddress.MatchString(address)
	} else {
		matched = evmPayoutAddress.MatchString(address)
	}
	if !matched {
		return "", "", ErrUsdtPayoutAddressInvalid
	}
	return id, address, nil
}

// EnabledUsdtChains is the public checkout list. Only chains the admin left on.
func EnabledUsdtChains() []UsdtChain {
	enabled := map[string]struct{}{}
	for _, id := range GetBepusdtEnabledChainIDs() {
		enabled[id] = struct{}{}
	}
	out := make([]UsdtChain, 0, len(usdtChainCatalog))
	for _, chain := range usdtChainCatalog {
		if _, ok := enabled[chain.ID]; ok {
			out = append(out, chain)
		}
	}
	return out
}

// GetBepusdtEnabledChainIDs reads the option. Missing means all four (not launched).
// Invalid JSON fails closed so a corrupt value cannot silently reopen a chain.
func GetBepusdtEnabledChainIDs() []string {
	bepusdtMu.RLock()
	raw := BepusdtEnabledChains
	bepusdtMu.RUnlock()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return allUsdtChainIDs()
	}
	ids, err := parseUsdtChainIDList(raw)
	if err != nil {
		return nil
	}
	return ids
}

// NormalizeBepusdtEnabledChainsJSON accepts a JSON array and returns catalog order.
// An empty array is valid (USDT checkout off). Unknown ids are rejected.
func NormalizeBepusdtEnabledChainsJSON(raw string) (string, error) {
	ids, err := parseUsdtChainIDList(raw)
	if err != nil {
		return "", err
	}
	return mustUsdtChainJSON(ids), nil
}

// SetBepusdtEnabledChains stores a normalized list. Invalid input fails closed.
func SetBepusdtEnabledChains(raw string) {
	normalized, err := NormalizeBepusdtEnabledChainsJSON(raw)
	bepusdtMu.Lock()
	defer bepusdtMu.Unlock()
	if err != nil {
		BepusdtEnabledChains = "[]"
		return
	}
	BepusdtEnabledChains = normalized
}

func allUsdtChainIDs() []string {
	ids := make([]string, len(usdtChainCatalog))
	for i, chain := range usdtChainCatalog {
		ids[i] = chain.ID
	}
	return ids
}

func normalizeUsdtChainID(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func parseUsdtChainIDList(raw string) ([]string, error) {
	var parsed []string
	if err := common.UnmarshalJsonStr(strings.TrimSpace(raw), &parsed); err != nil {
		return nil, ErrBepusdtEnabledChainsInvalid
	}
	if parsed == nil {
		return nil, ErrBepusdtEnabledChainsInvalid
	}
	seen := map[string]struct{}{}
	for _, item := range parsed {
		id := normalizeUsdtChainID(item)
		if id == "" {
			return nil, ErrBepusdtEnabledChainsInvalid
		}
		if _, ok := LookupUsdtChain(id); !ok {
			return nil, ErrBepusdtEnabledChainsInvalid
		}
		seen[id] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for _, chain := range usdtChainCatalog {
		if _, ok := seen[chain.ID]; ok {
			out = append(out, chain.ID)
		}
	}
	return out, nil
}

func mustUsdtChainJSON(ids []string) string {
	raw, err := common.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(raw)
}
