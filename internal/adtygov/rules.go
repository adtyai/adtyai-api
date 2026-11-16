package adtygov

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

//go:embed official_rule_ids.jsonl
var officialRuleIDsJSONL []byte

//go:embed checks.yaml
var officialChecksYAML []byte

// RuleMapping is one R_rule checkpoint: an official G/A rule_id mapped to
// the official Appendix E.2 checks. It does not invent new business rules.
type RuleMapping struct {
	RuleID      string   `json:"rule_id"`
	Title       string   `json:"title"`
	Source      string   `json:"source"`
	Checkpoints []string `json:"checkpoints"`
}

type officialRuleID struct {
	RuleID string `json:"rule_id"`
	Title  string `json:"title"`
	Source string `json:"source"`
}

// OfficialRuleMappings returns the in-repo mapping of official volume-1 rule IDs
// to the official E.2 checkpoint list.
func OfficialRuleMappings() ([]RuleMapping, error) {
	return DecodeRuleCatalog(bytes.NewReader(officialRuleIDsJSONL))
}

// DecodeRuleCatalog loads rule_id records and attaches official checkpoints.
func DecodeRuleCatalog(r io.Reader) ([]RuleMapping, error) {
	checks, err := OfficialCheckIDs()
	if err != nil {
		return nil, err
	}
	var out []RuleMapping
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	// Some official titles are short, but a line may still be long.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row officialRuleID
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("rule catalog: %w", err)
		}
		if row.RuleID == "" || seen[row.RuleID] {
			continue
		}
		seen[row.RuleID] = true
		src := row.Source
		if src == "" {
			src = "volume1"
		}
		out = append(out, RuleMapping{
			RuleID:      row.RuleID,
			Title:       row.Title,
			Source:      src,
			Checkpoints: append([]string(nil), checks...),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rule catalog is empty")
	}
	return out, nil
}

// OfficialCheckIDs returns the Appendix E.2 check identifiers in file order.
func OfficialCheckIDs() ([]string, error) {
	checks, err := OfficialChecks()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.ID)
	}
	return ids, nil
}
