package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// A45LegacyKeys are the six business params whose defaults come from the legacy
// fork (见 business-params-v1.md「老版」). Imported via scripts/import_a45_legacy_defaults.go.
var A45LegacyKeys = []string{
	"TopupOrderTimeoutMinutes",
	"AffTopupCommissionRate",
	"AffSubscriptionCommissionRate",
	"AffMinWithdrawAmount",
	"AffWithdrawFeeRate",
	"BalanceLowAlertThreshold",
}

// A45LegacyChange is one planned old→new pair for dry-run / reporting.
type A45LegacyChange struct {
	Key string
	Old string
	New string
}

// A45LegacyBackup is written before -apply so -rollback can restore.
type A45LegacyBackup struct {
	SavedAt string            `json:"saved_at"`
	Values  map[string]string `json:"values"`
}

// ParseA45LegacyFile reads a JSON object of the six A45 keys (and optional
// underscore-prefixed comment fields). Values may be JSON numbers or strings.
func ParseA45LegacyFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseA45LegacyJSON(raw)
}

// ParseA45LegacyJSON parses A45 legacy default JSON bytes.
func ParseA45LegacyJSON(raw []byte) (map[string]string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("a45 legacy json: %w", err)
	}
	out := make(map[string]string, len(A45LegacyKeys))
	allowed := make(map[string]struct{}, len(A45LegacyKeys))
	for _, k := range A45LegacyKeys {
		allowed[k] = struct{}{}
	}
	for k, v := range obj {
		if strings.HasPrefix(k, "_") {
			continue
		}
		if _, ok := allowed[k]; !ok {
			return nil, fmt.Errorf("a45 legacy json: unexpected key %q (only the six legacy keys allowed)", k)
		}
		s, err := normalizeA45JSONValue(v)
		if err != nil {
			return nil, fmt.Errorf("a45 legacy json: key %s: %w", k, err)
		}
		norm, err := NormalizeBusinessParamValue(k, s)
		if err != nil {
			return nil, fmt.Errorf("a45 legacy json: key %s: %w", k, err)
		}
		out[k] = norm
	}
	for _, k := range A45LegacyKeys {
		if _, ok := out[k]; !ok {
			return nil, fmt.Errorf("a45 legacy json: missing required key %s", k)
		}
	}
	return out, nil
}

func normalizeA45JSONValue(raw json.RawMessage) (string, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return "", fmt.Errorf("empty value")
	}
	// string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s), nil
	}
	// number (int or float)
	var n json.Number
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		d, err := decimal.NewFromString(string(n))
		if err != nil {
			return "", err
		}
		if d.Equal(d.Truncate(0)) && !strings.Contains(string(n), ".") {
			return d.StringFixed(0), nil
		}
		return d.String(), nil
	}
	return "", fmt.Errorf("value must be string or number, got %s", string(raw))
}

// ValidateA45LegacyValues runs catalog ValidateBusinessParam on every key.
func ValidateA45LegacyValues(values map[string]string) error {
	for _, k := range A45LegacyKeys {
		v, ok := values[k]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownBusinessParam, k)
		}
		if err := ValidateBusinessParam(k, v, values); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	}
	return nil
}

// PlanA45LegacyImport builds old→new rows. currentFn supplies the live value
// (typically BusinessParamCurrentValue); when nil, catalog defaults are used.
func PlanA45LegacyImport(values map[string]string, currentFn func(string) string) ([]A45LegacyChange, error) {
	if err := ValidateA45LegacyValues(values); err != nil {
		return nil, err
	}
	if currentFn == nil {
		currentFn = func(key string) string {
			if d, ok := businessParamDefByKey(key); ok {
				return d.Default
			}
			return ""
		}
	}
	changes := make([]A45LegacyChange, 0, len(A45LegacyKeys))
	for _, k := range A45LegacyKeys {
		old := currentFn(k)
		if n, err := NormalizeBusinessParamValue(k, old); err == nil {
			old = n
		}
		changes = append(changes, A45LegacyChange{
			Key: k,
			Old: old,
			New: values[k],
		})
	}
	return changes, nil
}

// A45 plan modes for FormatA45LegacyPlan header text.
const (
	A45PlanModeDryRun   = "dry-run"
	A45PlanModeApply    = "apply"
	A45PlanModeRollback = "rollback"
)

// FormatA45LegacyPlan prints a human-readable plan whose header matches mode
// (dry-run / apply / rollback). Unknown mode falls back to dry-run wording.
func FormatA45LegacyPlan(changes []A45LegacyChange, mode string) string {
	var b strings.Builder
	switch mode {
	case A45PlanModeApply:
		b.WriteString("A45 legacy defaults applied (DB written):\n")
	case A45PlanModeRollback:
		b.WriteString("A45 legacy defaults rolled back (DB written):\n")
	default:
		b.WriteString("A45 legacy defaults plan (dry-run; no DB write):\n")
	}
	for _, c := range changes {
		same := ""
		if c.Old == c.New {
			same = " (unchanged)"
		}
		fmt.Fprintf(&b, "  %s: %q → %q%s\n", c.Key, c.Old, c.New, same)
	}
	return b.String()
}

// DefaultA45RollbackPath returns scripts/a45_legacy_defaults.rollback.json
// relative to repoRoot (parent of scripts/ when path is under scripts).
func DefaultA45RollbackPath(repoRoot string) string {
	return filepath.Join(repoRoot, "scripts", "a45_legacy_defaults.rollback.json")
}

// WriteA45LegacyBackup writes current values for the six keys to path.
func WriteA45LegacyBackup(path string, currentFn func(string) string) error {
	if currentFn == nil {
		currentFn = BusinessParamCurrentValue
	}
	vals := make(map[string]string, len(A45LegacyKeys))
	for _, k := range A45LegacyKeys {
		vals[k] = currentFn(k)
	}
	bak := A45LegacyBackup{
		SavedAt: time.Now().Format(time.RFC3339),
		Values:  vals,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(bak, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

// ReadA45LegacyBackup loads a rollback file written by WriteA45LegacyBackup
// (or a plain six-key map for convenience).
func ReadA45LegacyBackup(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bak A45LegacyBackup
	if err := json.Unmarshal(raw, &bak); err == nil && len(bak.Values) > 0 {
		return ParseA45LegacyJSON(mustMarshalMap(bak.Values))
	}
	return ParseA45LegacyJSON(raw)
}

func mustMarshalMap(m map[string]string) []byte {
	// re-encode as JSON object with string values for ParseA45LegacyJSON
	type pair struct {
		k, v string
	}
	pairs := make([]pair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	var b strings.Builder
	b.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%q:%q", p.k, p.v)
	}
	b.WriteByte('}')
	return []byte(b.String())
}

// ApplyA45LegacyImport validates, writes backup, then persists via
// ApplyBusinessParamUpdates (no audit revival). Requires model.DB.
func ApplyA45LegacyImport(values map[string]string, backupPath string) ([]A45LegacyChange, error) {
	if DB == nil {
		return nil, fmt.Errorf("a45 apply: DB not initialized (set SQL_DSN and call InitDB)")
	}
	changes, err := PlanA45LegacyImport(values, BusinessParamCurrentValue)
	if err != nil {
		return nil, err
	}
	if backupPath == "" {
		return nil, fmt.Errorf("a45 apply: backup path required")
	}
	if err := WriteA45LegacyBackup(backupPath, BusinessParamCurrentValue); err != nil {
		return nil, fmt.Errorf("a45 apply: write backup: %w", err)
	}
	if err := ApplyBusinessParamUpdates(values); err != nil {
		return nil, fmt.Errorf("a45 apply: %w", err)
	}
	return changes, nil
}

// RollbackA45LegacyImport restores values from a backup file via ApplyBusinessParamUpdates.
func RollbackA45LegacyImport(backupPath string) ([]A45LegacyChange, error) {
	if DB == nil {
		return nil, fmt.Errorf("a45 rollback: DB not initialized (set SQL_DSN and call InitDB)")
	}
	values, err := ReadA45LegacyBackup(backupPath)
	if err != nil {
		return nil, err
	}
	changes, err := PlanA45LegacyImport(values, BusinessParamCurrentValue)
	if err != nil {
		return nil, err
	}
	if err := ApplyBusinessParamUpdates(values); err != nil {
		return nil, fmt.Errorf("a45 rollback: %w", err)
	}
	return changes, nil
}

// A45LegacyKeySet returns a set for quick membership tests.
func A45LegacyKeySet() map[string]struct{} {
	m := make(map[string]struct{}, len(A45LegacyKeys))
	for _, k := range A45LegacyKeys {
		m[k] = struct{}{}
	}
	return m
}
