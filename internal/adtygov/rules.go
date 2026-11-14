package adtygov

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
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

var officialHeading = regexp.MustCompile(`^#{3,4} (([GA])\.\d+(?:\.\d+)?)\s+(\S.*)$`)

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

// ParseOfficialPlanHeadings extracts official G/A heading rule IDs from a
// volume-1 plan fragment. Callers pass a path; this does not import or freeze
// an external issues/ledger file.
func ParseOfficialPlanHeadings(planPath string) ([]officialRuleID, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return nil, err
	}
	body := string(data)
	if idx := strings.Index(body, "# 卷一"); idx >= 0 {
		body = body[idx:]
		if end := strings.Index(body[1:], "\n# "); end >= 0 {
			body = body[:end+1]
		}
	}
	var out []officialRuleID
	for _, line := range strings.Split(body, "\n") {
		m := officialHeading.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		out = append(out, officialRuleID{RuleID: m[1], Title: strings.TrimSpace(m[3]), Source: "volume1"})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no official G/A headings in %s", planPath)
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
