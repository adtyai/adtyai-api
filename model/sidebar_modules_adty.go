package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

// OptSidebarModulesAdmin is the official sidebar modules option key.
const OptSidebarModulesAdmin = "SidebarModulesAdmin"

// OptMenuVisibilityMergedAt marks that SidebarUserVisible/SidebarAdminVisible
// have been merged into SidebarModulesAdmin (idempotent gate).
const OptMenuVisibilityMergedAt = "MenuVisibilityMergedAt"

// SidebarSectionModules is one sidebar section: enabled + module bools.
// Shape matches frontend DEFAULT_SIDEBAR_MODULES / SIDEBAR_MODULES_DEFAULT.
type SidebarSectionModules map[string]bool

// SidebarModulesAdmin is the official modules JSON root
// (chat / console / personal / admin → section).
type SidebarModulesAdmin map[string]SidebarSectionModules

// DefaultSidebarModulesAdmin returns the frontend-compatible default shape
// (all known modules visible). Used when parsing empty/invalid stored JSON
// and as the merge base for missing sections/keys.
func DefaultSidebarModulesAdmin() SidebarModulesAdmin {
	return SidebarModulesAdmin{
		"chat": {
			"enabled":    true,
			"playground": true,
			"chat":       true,
		},
		"console": {
			"enabled":    true,
			"detail":     true,
			"token":      true,
			"log":        true,
			"audit":      true,
			"midjourney": true,
			"task":       true,
		},
		"personal": {
			"enabled":  true,
			"topup":    true,
			"personal": true,
			"security": true,
		},
		"admin": {
			"enabled":            true,
			"channel":            true,
			"smart_routing":      true,
			"models":             true,
			"redemption":         true,
			"user":               true,
			"setting":            true,
			"system_info":        true,
			"task_plugins":       true,
			"orders":             true,
			"enterprise_pricing": true,
			"alerts":             true,
			"chatwoot":           true,
			"risk_reviews":       true,
			"aff_commissions":    true,
			"aff_withdrawals":    true,
			"bonus_ledger":       true,
			"order_refunds":      true,
		},
	}
}

// menuVisibilityUserTarget maps SidebarUserVisible keys → section.module.
// wallet+affiliate both land on personal.topup (任一 false → module false).
var menuVisibilityUserTarget = map[string]struct{ section, module string }{
	"wallet":     {"personal", "topup"},
	"affiliate":  {"personal", "topup"},
	"keys":       {"console", "token"},
	"api-tokens": {"console", "token"},
	"overview":   {"console", "detail"},
	"dashboard":  {"console", "detail"},
	"playground": {"chat", "playground"},
	"usage-logs": {"console", "log"},
	"profile":    {"personal", "personal"},
	"security":   {"personal", "security"},
}

// menuVisibilityAdminTarget maps SidebarAdminVisible keys → section.module.
var menuVisibilityAdminTarget = map[string]struct{ section, module string }{
	"channels":           {"admin", "channel"},
	"models":             {"admin", "models"},
	"users":              {"admin", "user"},
	"redemption-codes":   {"admin", "redemption"},
	"system-settings":    {"admin", "setting"},
	"system-info":        {"admin", "system_info"},
	"task-plugins":       {"admin", "task_plugins"},
	"plugins":            {"admin", "task_plugins"}, // alias
	"orders":             {"admin", "orders"},
	"enterprise-pricing": {"admin", "enterprise_pricing"},
	"alerts":             {"admin", "alerts"},
	"chatwoot":           {"admin", "chatwoot"},
	"risk-reviews":       {"admin", "risk_reviews"},
	"aff-commissions":    {"admin", "aff_commissions"},
	"aff-withdrawals":    {"admin", "aff_withdrawals"},
	"bonus-ledger":       {"admin", "bonus_ledger"},
	"order-refunds":      {"admin", "order_refunds"},
}

// ParseSidebarModulesAdmin parses option JSON into modules.
// Empty / whitespace / "null" → empty map (caller may merge defaults).
// Invalid JSON → error.
func ParseSidebarModulesAdmin(raw string) (SidebarModulesAdmin, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return SidebarModulesAdmin{}, nil
	}
	var loose map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &loose); err != nil {
		return nil, fmt.Errorf("invalid SidebarModulesAdmin json: %w", err)
	}
	out := make(SidebarModulesAdmin, len(loose))
	for section, mods := range loose {
		if mods == nil {
			continue
		}
		sec := make(SidebarSectionModules, len(mods))
		for k, v := range mods {
			sec[k] = coerceOptionBool(v, true)
		}
		out[section] = sec
	}
	return out, nil
}

func coerceOptionBool(v any, fallback bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		if t == 1 {
			return true
		}
		if t == 0 {
			return false
		}
		return fallback
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		if s == "true" || s == "1" {
			return true
		}
		if s == "false" || s == "0" {
			return false
		}
		return fallback
	default:
		return fallback
	}
}

// MergeSidebarModulesAdmin overlays stored onto defaults.
// Missing sections/keys take defaults; stored values win (including false).
// Never flips an existing false back to true.
func MergeSidebarModulesAdmin(stored SidebarModulesAdmin) SidebarModulesAdmin {
	base := DefaultSidebarModulesAdmin()
	if stored == nil {
		return base
	}
	for section, mods := range stored {
		if mods == nil {
			continue
		}
		dst, ok := base[section]
		if !ok {
			dst = SidebarSectionModules{"enabled": true}
			base[section] = dst
		}
		for k, v := range mods {
			dst[k] = v
		}
	}
	return base
}

// MarshalSidebarModulesAdmin encodes modules JSON.
func MarshalSidebarModulesAdmin(m SidebarModulesAdmin) (string, error) {
	if m == nil {
		m = SidebarModulesAdmin{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ApplyMenuVisibilityFalseToModules applies only false Visible entries onto
// modules. Missing/true Visible keys are ignored (缺省可见；禁止 silent 全开).
// Multi-key → same module: 任一 false → module false.
// Unknown keys land under personal (user) or admin (admin) as snake_case.
func ApplyMenuVisibilityFalseToModules(modules SidebarModulesAdmin, userVis, adminVis map[string]bool) SidebarModulesAdmin {
	if modules == nil {
		modules = SidebarModulesAdmin{}
	}
	applySide := func(side string, vis map[string]bool, known map[string]struct{ section, module string }) {
		for key, visible := range vis {
			if visible {
				continue
			}
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			section, module := "", ""
			if t, ok := known[key]; ok {
				section, module = t.section, t.module
			} else {
				section = "personal"
				if side == "admin" {
					section = "admin"
				}
				module = kebabToSnake(key)
				common.SysLog(common.LogText(
					"menu-visibility merge: unknown %s key %q → %s.%s",
					side, key, section, module,
				))
			}
			disableModule(modules, section, module)
		}
	}
	applySide("user", userVis, menuVisibilityUserTarget)
	applySide("admin", adminVis, menuVisibilityAdminTarget)
	return modules
}

func disableModule(modules SidebarModulesAdmin, section, module string) {
	sec, ok := modules[section]
	if !ok || sec == nil {
		sec = SidebarSectionModules{"enabled": true}
		modules[section] = sec
	}
	sec[module] = false
}

func kebabToSnake(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '-' || r == ' ':
			b.WriteByte('_')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func optionMapGetUnlocked(key string) string {
	return common.OptionMap[key]
}

// MenuVisibilityAlreadyMerged reports whether the one-shot merge has run.
func MenuVisibilityAlreadyMerged() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.OptionMap[OptMenuVisibilityMergedAt]) != ""
}

// MigrateMenuVisibilityIntoSidebarModules merges legacy Visible maps into
// SidebarModulesAdmin (false→disable only), sets MenuVisibilityMergedAt,
// and clears old Visible keys to "{}". Idempotent when MergedAt is set.
// Safe no-op when DB is nil.
func MigrateMenuVisibilityIntoSidebarModules() error {
	if DB == nil {
		return nil
	}
	if MenuVisibilityAlreadyMerged() {
		return nil
	}

	common.OptionMapRWMutex.RLock()
	modulesRaw := optionMapGetUnlocked(OptSidebarModulesAdmin)
	userRaw := optionMapGetUnlocked(OptSidebarUserVisible)
	adminRaw := optionMapGetUnlocked(OptSidebarAdminVisible)
	common.OptionMapRWMutex.RUnlock()

	userVis, err := ParseMenuVisibilityJSON(userRaw)
	if err != nil {
		common.SysError(common.LogText("menu-visibility merge: parse user Visible: %v", err))
		userVis = map[string]bool{}
	}
	adminVis, err := ParseMenuVisibilityJSON(adminRaw)
	if err != nil {
		common.SysError(common.LogText("menu-visibility merge: parse admin Visible: %v", err))
		adminVis = map[string]bool{}
	}

	stored, err := ParseSidebarModulesAdmin(modulesRaw)
	if err != nil {
		common.SysError(common.LogText("menu-visibility merge: parse SidebarModulesAdmin: %v", err))
		stored = SidebarModulesAdmin{}
	}
	// Work on a copy of stored (do not inflate missing defaults into write
	// unless we disable something in a missing section — disableModule creates
	// the section). Preserves existing false; never silent-opens.
	modules := cloneSidebarModules(stored)
	ApplyMenuVisibilityFalseToModules(modules, userVis, adminVis)

	modulesJSON, err := MarshalSidebarModulesAdmin(modules)
	if err != nil {
		return fmt.Errorf("marshal SidebarModulesAdmin: %w", err)
	}
	mergedAt := time.Now().UTC().Format(time.RFC3339)
	if err := UpdateOptionsBulk(map[string]string{
		OptSidebarModulesAdmin:    modulesJSON,
		OptMenuVisibilityMergedAt: mergedAt,
		OptSidebarUserVisible:     "{}",
		OptSidebarAdminVisible:    "{}",
	}); err != nil {
		return fmt.Errorf("persist menu-visibility merge: %w", err)
	}
	common.SysLog(common.LogText("menu-visibility merged into SidebarModulesAdmin at %s", mergedAt))
	return nil
}

func cloneSidebarModules(in SidebarModulesAdmin) SidebarModulesAdmin {
	out := make(SidebarModulesAdmin, len(in))
	for section, mods := range in {
		if mods == nil {
			continue
		}
		cp := make(SidebarSectionModules, len(mods))
		for k, v := range mods {
			cp[k] = v
		}
		out[section] = cp
	}
	return out
}

// SeedSidebarModulesMergeOptionIntoOptionMap registers the merge marker default.
func SeedSidebarModulesMergeOptionIntoOptionMap() {
	common.OptionMap[OptMenuVisibilityMergedAt] = ""
}
