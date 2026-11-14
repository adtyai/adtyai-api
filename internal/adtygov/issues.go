package adtygov

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// IssueRecord covers the official plan 3.13.1 problem-list fields used by
// the in-repo manifest tool. The tool validates and lists records; it does
// not open or edit the frozen external issues.jsonl.
type IssueRecord struct {
	IssueID      string `json:"issue_id"`
	Type         string `json:"type"`
	Severity     string `json:"severity"`
	RuleID       string `json:"rule_id"`
	Path         string `json:"path"`
	Expected     string `json:"expected"`
	Actual       string `json:"actual"`
	RootCause    string `json:"root_cause"`
	Status       string `json:"status"`
	AuditID      string `json:"audit_id,omitempty"`
	Baseline     string `json:"baseline,omitempty"`
	Roles        string `json:"affected_roles,omitempty"`
	Sites        string `json:"affected_sites,omitempty"`
	Teams        string `json:"affected_teams,omitempty"`
	Repro        string `json:"repro,omitempty"`
	Version      string `json:"first_failed_version,omitempty"`
	Environment  string `json:"environment,omitempty"`
	Impact       string `json:"impact,omitempty"`
	Evidence     string `json:"evidence,omitempty"`
	FixBatch     string `json:"fix_batch,omitempty"`
	Verification string `json:"verification,omitempty"`
	Commit       string `json:"commit,omitempty"`
}

var officialSeverities = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}

// ValidateIssueRecord checks that a single manifest row has the official required fields.
func ValidateIssueRecord(rec IssueRecord) error {
	switch {
	case strings.TrimSpace(rec.IssueID) == "":
		return fmt.Errorf("issue_id is required")
	case strings.TrimSpace(rec.Type) == "":
		return fmt.Errorf("type is required")
	case !officialSeverities[rec.Severity]:
		return fmt.Errorf("severity must be P0, P1, P2, or P3")
	case strings.TrimSpace(rec.RuleID) == "":
		return fmt.Errorf("rule_id is required")
	case strings.TrimSpace(rec.Path) == "":
		return fmt.Errorf("path is required")
	case strings.TrimSpace(rec.Expected) == "":
		return fmt.Errorf("expected is required")
	case strings.TrimSpace(rec.Actual) == "":
		return fmt.Errorf("actual is required")
	case strings.TrimSpace(rec.RootCause) == "":
		return fmt.Errorf("root_cause is required")
	case strings.TrimSpace(rec.Status) == "":
		return fmt.Errorf("status is required")
	default:
		return nil
	}
}

// ValidateIssueManifest reads jsonl issue records from r. It never writes.
func ValidateIssueManifest(r io.Reader) ([]IssueRecord, error) {
	var out []IssueRecord
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec IssueRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if err := ValidateIssueRecord(rec); err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", lineNo, rec.IssueID, err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListIssueIDs returns stable issue identifiers from an already-validated list.
func ListIssueIDs(records []IssueRecord) []string {
	ids := make([]string, 0, len(records))
	for _, rec := range records {
		ids = append(ids, rec.IssueID)
	}
	return ids
}
