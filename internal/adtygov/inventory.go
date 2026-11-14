package adtygov

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Unit is one plan 3.2.1 units.jsonl record generated from the current tree.
// This generator does not load or rewrite the external frozen denominator.
type Unit struct {
	UnitID     string   `json:"unit_id"`
	Kind       string   `json:"kind"`
	Path       string   `json:"path"`
	Dims       []string `json:"dims"`
	Slice      string   `json:"slice"`
	SourceHash string   `json:"source_hash"`
	State      string   `json:"state"`
}

var backendDims = []string{"callchain", "tx", "perm", "error", "fund", "cache"}
var frontendDims = []string{"callchain", "perm", "error"}
var authDims = []string{"callchain", "perm"}
var dbDims = []string{"tx", "fund", "error"}

var skipInventoryDirNames = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"coverage":     true,
	"testdata":     true,
}

// EnumerateUnits walks the product repository and emits current review units.
func EnumerateUnits(root string) ([]Unit, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return nil, fmt.Errorf("repository root must contain go.mod: %w", err)
	}

	var units []Unit
	seen := map[string]bool{}
	add := func(u Unit) {
		if u.Path == "" || seen[u.UnitID] {
			return
		}
		seen[u.UnitID] = true
		units = append(units, u)
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			name := d.Name()
			if name != "." && (skipInventoryDirNames[name] || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			if isFeatureDir(rel) {
				unit, err := featureDirUnit(root, rel)
				if err != nil {
					return err
				}
				add(unit)
			}
			return nil
		}
		if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
			unit, err := goFileUnit(root, rel)
			if err != nil {
				return err
			}
			add(unit)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(units, func(i, j int) bool { return units[i].UnitID < units[j].UnitID })
	return units, nil
}

func isFeatureDir(rel string) bool {
	const prefix = "web/src/features/"
	if !strings.HasPrefix(rel, prefix) {
		return false
	}
	rest := strings.TrimPrefix(rel, prefix)
	return rest != "" && !strings.Contains(rest, "/")
}

func goFileUnit(root, rel string) (Unit, error) {
	sum, err := hashFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return Unit{}, err
	}
	stem := strings.TrimSuffix(filepath.Base(rel), ".go")
	dir := filepath.ToSlash(filepath.Dir(rel))
	kind := "code"
	dims := backendDims
	slice := "GEN-BE"
	prefix := "WU-BE"
	switch {
	case dir == "router" || strings.HasPrefix(dir, "router/"):
		prefix = "WU-AUTH"
		dims = authDims
		slice = "GEN-AUTH"
	case isMigrationPath(rel):
		prefix = "WU-DB"
		dims = dbDims
		slice = "GEN-DB"
	}
	idDir := strings.ReplaceAll(dir, "/", "-")
	if idDir == "." {
		idDir = "root"
	}
	return Unit{
		UnitID:     fmt.Sprintf("%s-%s-%s", prefix, idDir, stem),
		Kind:       kind,
		Path:       rel,
		Dims:       append([]string(nil), dims...),
		Slice:      slice,
		SourceHash: sum,
		State:      "pending",
	}, nil
}

func isMigrationPath(rel string) bool {
	base := strings.ToLower(filepath.Base(rel))
	return strings.Contains(base, "migrate") || strings.Contains(base, "migration")
}

func featureDirUnit(root, rel string) (Unit, error) {
	sum, err := hashTree(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return Unit{}, err
	}
	name := filepath.Base(rel)
	return Unit{
		UnitID:     "WU-FE-" + name,
		Kind:       "code",
		Path:       rel,
		Dims:       append([]string(nil), frontendDims...),
		Slice:      "GEN-FE",
		SourceHash: sum,
		State:      "pending",
	}, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

func hashTree(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipInventoryDirNames[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %s\n", filepath.ToSlash(rel), sum)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}
