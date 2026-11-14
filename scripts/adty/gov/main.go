// Command adtygov is the official in-repo entry point for H.24 process
// capabilities ADTY-24-01 through 24-08. ADTY-24-06 is landed as a
// retirement-chain inventory. This tool does not claim ADTY-24-06, 24-09,
// or 24-10 closed, and it does not run Phase D.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/QuantumNous/new-api/internal/adtygov"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	switch cmd {
	case "enumerate-units":
		root, output := flagRootOutput(args)
		if root == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			root = wd
		}
		units, err := adtygov.EnumerateUnits(root)
		if err != nil {
			return err
		}
		return writeJSONL(output, units)
	case "map-rules":
		_, output := flagRootOutput(args)
		rules, err := adtygov.OfficialRuleMappings()
		if err != nil {
			return err
		}
		return writeJSONL(output, rules)
	case "review-checks":
		checks, err := adtygov.OfficialChecks()
		if err != nil {
			return err
		}
		return writeJSON(os.Stdout, checks)
	case "review-record":
		runID := "review-pending"
		output := ""
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--run-id":
				i++
				if i < len(args) {
					runID = args[i]
				}
			case "-o", "--output":
				i++
				if i < len(args) {
					output = args[i]
				}
			}
		}
		rec, err := adtygov.NewReviewRun(runID)
		if err != nil {
			return err
		}
		raw, err := adtygov.EncodeReviewRun(rec)
		if err != nil {
			return err
		}
		return writeBytes(output, raw)
	case "validate-issues":
		if len(args) != 1 {
			return fmt.Errorf("usage: adtygov validate-issues <manifest.jsonl>")
		}
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		records, err := adtygov.ValidateIssueManifest(f)
		if err != nil {
			return err
		}
		for _, id := range adtygov.ListIssueIDs(records) {
			fmt.Println(id)
		}
		return nil
	case "record-rca":
		if len(args) != 1 {
			return fmt.Errorf("usage: adtygov record-rca <rca.jsonl>")
		}
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		records, err := adtygov.ReadRCARecords(f)
		if err != nil {
			return err
		}
		for _, rec := range records {
			fmt.Println(rec.IssueID)
		}
		return nil
	case "cleanup-campaign":
		return writeJSON(os.Stdout, adtygov.OfficialCleanupCampaign())
	case "retired-chains":
		return writeJSON(os.Stdout, adtygov.OfficialRetiredChainInventory())
	case "rereview-record":
		return writeJSON(os.Stdout, adtygov.NewRereviewEntry())
	case "capabilities":
		return writeJSON(os.Stdout, map[string]any{
			"landed":       adtygov.LandedCapabilities,
			"not_claimed":  adtygov.NotClaimedCapabilities,
			"entry_points": adtygov.OfficialEntryPoints(),
		})
	case "-h", "--help", "help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `Official H.24 process tooling (ADTY-24-01..08).
ADTY-24-06 is landed as an inventory and is not claimed closed.
Does not claim ADTY-24-06, 24-09, or 24-10 closed, and does not run Phase D or smoke/scale.

Commands:
  enumerate-units [--root DIR] [-o FILE]   ADTY-24-01 current-tree units.jsonl
  map-rules [-o FILE]                      ADTY-24-02 official G/A rule mappings
  review-checks                            ADTY-24-03 official checks.yaml
  review-record [--run-id ID] [-o FILE]    ADTY-24-03 recorded, not-executed run
  validate-issues FILE                     ADTY-24-04 validate a manifest (does not edit frozen issues)
  record-rca FILE                          ADTY-24-05 validate RCA/fix linkage records
  retired-chains                           ADTY-24-06 retirement-chain inventory (not claimed closed)
  cleanup-campaign                         ADTY-24-07 campaign artifact (not executed)
  rereview-record                          ADTY-24-08 rereview entry (Phase D not run)
  capabilities                             landed vs not-claimed IDs
`)
}

func flagRootOutput(args []string) (root, output string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			if i < len(args) {
				root = args[i]
			}
		case "-o", "--output":
			i++
			if i < len(args) {
				output = args[i]
			}
		}
	}
	return root, output
}

func writeJSONL(path string, rows any) error {
	list, err := toSlice(rows)
	if err != nil {
		return err
	}
	var raw []byte
	for _, row := range list {
		line, err := json.Marshal(row)
		if err != nil {
			return err
		}
		raw = append(raw, line...)
		raw = append(raw, '\n')
	}
	return writeBytes(path, raw)
}

func toSlice(rows any) ([]any, error) {
	b, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	var list []any
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeBytes(path string, raw []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(raw)
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
