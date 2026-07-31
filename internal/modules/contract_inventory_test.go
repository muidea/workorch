package modules

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestComponentContractMatrixCoversRegisteredComponents(t *testing.T) {
	root := findRepoRoot(t)
	// The active CWI reconciliation plan is the sole current component-boundary
	// matrix. Archived plans document history only and must not constrain the
	// current component shape.
	planPath := filepath.Join(root, "docs", "70-roadmap", "active", "runtime-v2-cwi-reconciliation-single-owner-closure-plan.md")
	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read closure plan: %v", err)
	}
	plan := string(planBytes)

	registered := registeredComponentDirs(t, root)
	for _, rel := range registered {
		if !strings.Contains(plan, "`"+rel+"`") && !strings.Contains(plan, "| "+rel+" |") && !strings.Contains(plan, rel) {
			t.Fatalf("closure plan does not cover registered component %s", rel)
		}
	}
}

func registeredComponentDirs(t *testing.T, root string) []string {
	t.Helper()

	var dirs []string
	for _, base := range []string{"internal/initiators", "internal/modules"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Name() != "module.go" {
				return nil
			}
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			dirs = append(dirs, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}

	sort.Strings(dirs)
	return dirs
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("go.mod not found from %s", dir)
		}
		dir = next
	}
}
