package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// A39 menu-visibility option keys (adtyai-tech A1).
const (
	OptSidebarUserVisible  = "SidebarUserVisible"
	OptSidebarAdminVisible = "SidebarAdminVisible"
)

// MenuVisibilityView is GET /api/admin/menu-visibility payload.
type MenuVisibilityView struct {
	User  map[string]bool `json:"user"`
	Admin map[string]bool `json:"admin"`
}

// MenuVisibilityPutInput is the PUT body.
// Nil pointer = leave that side unchanged; non-nil (even empty map) = replace that side wholesale.
type MenuVisibilityPutInput struct {
	User  *map[string]bool `json:"user"`
	Admin *map[string]bool `json:"admin"`
}

// ParseMenuVisibilityJSON parses option JSON into a map.
// Empty / whitespace / "null" → empty map (all keys default visible).
// Invalid JSON → error.
func ParseMenuVisibilityJSON(raw string) (map[string]bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]bool{}, nil
	}
	var m map[string]bool
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("invalid menu-visibility json: %w", err)
	}
	if m == nil {
		m = map[string]bool{}
	}
	return m, nil
}

// MenuVisibilityMapFromRaw is the lock-free parse used under OptionMap RLock (e.g. GetStatus).
// On parse error returns empty map so status never 500s.
func MenuVisibilityMapFromRaw(raw string) map[string]bool {
	m, err := ParseMenuVisibilityJSON(raw)
	if err != nil || m == nil {
		return map[string]bool{}
	}
	return m
}

func menuVisibilityOptionRaw(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[key]
}

// GetMenuVisibilityMap reads and parses one side (user|admin).
func GetMenuVisibilityMap(side string) map[string]bool {
	key := OptSidebarUserVisible
	if side == "admin" {
		key = OptSidebarAdminVisible
	}
	m, err := ParseMenuVisibilityJSON(menuVisibilityOptionRaw(key))
	if err != nil || m == nil {
		return map[string]bool{}
	}
	return m
}

// BuildMenuVisibilityView assembles GET payload.
func BuildMenuVisibilityView() MenuVisibilityView {
	return MenuVisibilityView{
		User:  GetMenuVisibilityMap("user"),
		Admin: GetMenuVisibilityMap("admin"),
	}
}

// MenuKeyVisible reports whether a menu key is visible.
// Missing key / empty map → true (upstream new menus default visible).
func MenuKeyVisible(side, key string) bool {
	m := GetMenuVisibilityMap(side)
	v, ok := m[key]
	if !ok {
		return true
	}
	return v
}

func normalizeMenuVisibilityMap(in map[string]bool) (map[string]bool, error) {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, fmt.Errorf("empty menu key")
		}
		out[k] = v
	}
	return out, nil
}

func marshalMenuVisibilityMap(m map[string]bool) (string, error) {
	if m == nil {
		m = map[string]bool{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ApplyMenuVisibilityPut persists user and/or admin maps. At least one side required.
func ApplyMenuVisibilityPut(in MenuVisibilityPutInput) (oldSnap, newSnap map[string]any, err error) {
	if in.User == nil && in.Admin == nil {
		return nil, nil, fmt.Errorf("user and/or admin map required")
	}
	oldSnap = map[string]any{
		"user":  GetMenuVisibilityMap("user"),
		"admin": GetMenuVisibilityMap("admin"),
	}
	updates := map[string]string{}
	if in.User != nil {
		norm, nerr := normalizeMenuVisibilityMap(*in.User)
		if nerr != nil {
			return oldSnap, nil, nerr
		}
		raw, merr := marshalMenuVisibilityMap(norm)
		if merr != nil {
			return oldSnap, nil, merr
		}
		updates[OptSidebarUserVisible] = raw
	}
	if in.Admin != nil {
		norm, nerr := normalizeMenuVisibilityMap(*in.Admin)
		if nerr != nil {
			return oldSnap, nil, nerr
		}
		raw, merr := marshalMenuVisibilityMap(norm)
		if merr != nil {
			return oldSnap, nil, merr
		}
		updates[OptSidebarAdminVisible] = raw
	}
	for k, v := range updates {
		if err = UpdateOption(k, v); err != nil {
			return oldSnap, nil, fmt.Errorf("%s: %w", k, err)
		}
	}
	newSnap = map[string]any{
		"user":  GetMenuVisibilityMap("user"),
		"admin": GetMenuVisibilityMap("admin"),
	}
	return oldSnap, newSnap, nil
}

// SeedMenuVisibilityOptionsIntoOptionMap registers defaults (called from InitOptionMap).
func SeedMenuVisibilityOptionsIntoOptionMap() {
	common.OptionMap[OptSidebarUserVisible] = "{}"
	common.OptionMap[OptSidebarAdminVisible] = "{}"
}
