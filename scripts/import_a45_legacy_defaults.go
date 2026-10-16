//go:build ignore

// A45: import legacy-fork defaults for the six business params into options.
// Default mode is dry-run (validate + print old→new). Use -apply only with SQL_DSN
// after I5 provides real JSON. Never commit secrets; these six keys are non-sensitive.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func main() {
	var (
		file     string
		backup   string
		apply    bool
		rollback bool
		repoRoot string
	)
	flag.StringVar(&file, "file", "", "JSON data source (default: scripts/a45_legacy_defaults.sample.json)")
	flag.StringVar(&backup, "backup", "", "rollback backup path (default: scripts/a45_legacy_defaults.rollback.json)")
	flag.BoolVar(&apply, "apply", false, "write options (requires SQL_DSN); default is dry-run")
	flag.BoolVar(&rollback, "rollback", false, "restore six keys from -backup file (requires SQL_DSN)")
	flag.StringVar(&repoRoot, "repo", "", "repo root (default: parent of scripts/)")
	flag.Parse()

	if repoRoot == "" {
		exe, err := os.Getwd()
		if err != nil {
			fatal(err)
		}
		// Prefer cwd if it looks like repo root; else parent of this script via relative.
		if _, err := os.Stat(filepath.Join(exe, "go.mod")); err == nil {
			repoRoot = exe
		} else {
			repoRoot = exe
		}
	}
	if file == "" {
		file = filepath.Join(repoRoot, "scripts", "a45_legacy_defaults.sample.json")
	}
	if backup == "" {
		backup = model.DefaultA45RollbackPath(repoRoot)
	}

	if apply && rollback {
		fatal(fmt.Errorf("use either -apply or -rollback, not both"))
	}

	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true

	needDB := apply || rollback
	if needDB {
		if os.Getenv("SQL_DSN") == "" {
			fatal(fmt.Errorf("-apply/-rollback require SQL_DSN (refusing to guess a DSN)"))
		}
		if err := model.InitDB(); err != nil {
			fatal(err)
		}
		if err := model.InitLogDB(); err != nil {
			fatal(err)
		}
		model.InitOptionMap()
		fmt.Println("InitDB+OptionMap OK")
	}

	if rollback {
		changes, err := model.RollbackA45LegacyImport(backup)
		if err != nil {
			fatal(err)
		}
		fmt.Print(model.FormatA45LegacyPlan(changes, model.A45PlanModeRollback))
		fmt.Printf("rollback applied from %s\n", backup)
		return
	}

	values, err := model.ParseA45LegacyFile(file)
	if err != nil {
		fatal(err)
	}

	var currentFn func(string) string
	if needDB {
		currentFn = model.BusinessParamCurrentValue
	} else {
		currentFn = nil // catalog defaults
	}

	if !apply {
		changes, err := model.PlanA45LegacyImport(values, currentFn)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("file=%s\n", file)
		fmt.Print(model.FormatA45LegacyPlan(changes, model.A45PlanModeDryRun))
		fmt.Println("dry-run OK (no DB write). Pass -apply with SQL_DSN to persist after I5 real JSON.")
		return
	}

	changes, err := model.ApplyA45LegacyImport(values, backup)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("file=%s backup=%s\n", file, backup)
	fmt.Print(model.FormatA45LegacyPlan(changes, model.A45PlanModeApply))
	fmt.Println("apply OK")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "import_a45_legacy_defaults: %v\n", err)
	os.Exit(1)
}
