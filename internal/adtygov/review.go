package adtygov

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Check is one official Appendix E.2 checklist item from checks.yaml.
type Check struct {
	ID               string   `yaml:"id" json:"id"`
	AppliesTo        []string `yaml:"applies_to" json:"applies_to"`
	Question         string   `yaml:"question" json:"question"`
	EvidenceRequired []string `yaml:"evidence_required" json:"evidence_required"`
	FailureSeverity  string   `yaml:"failure_severity" json:"failure_severity"`
}

// Finding is one recorded review conclusion. Recording a finding is not a
// claim that a full global review has passed.
type Finding struct {
	FindingID string `json:"finding_id"`
	UnitID    string `json:"unit_id"`
	CheckID   string `json:"check_id"`
	Result    string `json:"result"`
	Path      string `json:"path,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
}

// ReviewRun is a stored review session. New runs start as recorded and not
// executed; FullReviewPassed stays false until a later process records otherwise.
type ReviewRun struct {
	RunID            string    `json:"run_id"`
	CapabilityID     string    `json:"capability_id"`
	Status           string    `json:"status"`
	FullReviewPassed bool      `json:"full_review_passed"`
	Checks           []string  `json:"checks"`
	Findings         []Finding `json:"findings,omitempty"`
}

const (
	ReviewStatusRecordedNotExecuted = "recorded_not_executed"
	FindingResultPending            = "pending"
	FindingResultPass               = "pass"
	FindingResultFail               = "fail"
	FindingResultNotApplicable      = "not_applicable"
)

var officialCheckOrder = []string{
	"CHK-CALLCHAIN",
	"CHK-TX",
	"CHK-PERM",
	"CHK-IDEMP",
	"CHK-FUND",
	"CHK-ERROR",
	"CHK-RETIRE",
}

// OfficialChecks loads the in-repo Appendix E.2 checklist.
func OfficialChecks() ([]Check, error) {
	var checks []Check
	if err := yaml.Unmarshal(officialChecksYAML, &checks); err != nil {
		return nil, err
	}
	if err := validateOfficialChecks(checks); err != nil {
		return nil, err
	}
	return checks, nil
}

func validateOfficialChecks(checks []Check) error {
	seen := map[string]bool{}
	for _, check := range checks {
		seen[check.ID] = true
		if check.Question == "" || check.FailureSeverity == "" {
			return fmt.Errorf("check %s is incomplete", check.ID)
		}
	}
	for _, id := range officialCheckOrder {
		if !seen[id] {
			return fmt.Errorf("official checks.yaml is missing %s", id)
		}
	}
	return nil
}

// NewReviewRun creates a findings-store entry for a review that has not been
// executed and is not marked as a full-review pass.
func NewReviewRun(runID string) (*ReviewRun, error) {
	ids, err := OfficialCheckIDs()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(runID) == "" {
		runID = "review-pending"
	}
	return &ReviewRun{
		RunID:            runID,
		CapabilityID:     CapGlobalReview,
		Status:           ReviewStatusRecordedNotExecuted,
		FullReviewPassed: false,
		Checks:           ids,
	}, nil
}

// WriteFindingsJSONL writes finding records to the official jsonl store format.
func WriteFindingsJSONL(w io.Writer, findings []Finding) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, finding := range findings {
		if err := enc.Encode(finding); err != nil {
			return err
		}
	}
	return nil
}

// ReadFindingsJSONL loads previously recorded findings. It never claims they passed.
func ReadFindingsJSONL(r io.Reader) ([]Finding, error) {
	var out []Finding
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var finding Finding
		if err := json.Unmarshal([]byte(line), &finding); err != nil {
			return nil, err
		}
		out = append(out, finding)
	}
	return out, sc.Err()
}

// EncodeReviewRun serializes a review run record.
func EncodeReviewRun(run *ReviewRun) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(run); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
