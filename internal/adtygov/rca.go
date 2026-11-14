package adtygov

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RCARecord is the official ADTY-24-05 root-cause / fix linkage record.
// It is a landed capability, not a rewrite of Phase C execution.
type RCARecord struct {
	IssueID     string `json:"issue_id"`
	RootCause   string `json:"root_cause"`
	FixEvidence string `json:"fix_evidence"`
	Commit      string `json:"commit"`
}

// ValidateRCA checks that a linkage record has the official required fields.
func ValidateRCA(rec RCARecord) error {
	switch {
	case strings.TrimSpace(rec.IssueID) == "":
		return fmt.Errorf("issue_id is required")
	case strings.TrimSpace(rec.RootCause) == "":
		return fmt.Errorf("root_cause is required")
	case strings.TrimSpace(rec.FixEvidence) == "":
		return fmt.Errorf("fix_evidence is required")
	case strings.TrimSpace(rec.Commit) == "":
		return fmt.Errorf("commit is required")
	default:
		return nil
	}
}

// ReadRCARecords loads official RCA/fix linkage records from jsonl.
func ReadRCARecords(r io.Reader) ([]RCARecord, error) {
	var out []RCARecord
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec RCARecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if err := ValidateRCA(rec); err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", lineNo, rec.IssueID, err)
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}
