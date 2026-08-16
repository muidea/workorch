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
	// The current component-alignment inventory documents the registered shape.
	// Roadmap plans and archived closure records must not constrain it.
	inventoryPath := filepath.Join(root, "docs", "01-overview", "component-documentation-alignment.md")
	inventoryBytes, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatalf("read component inventory: %v", err)
	}
	inventory := string(inventoryBytes)

	registered := registeredComponentDirs(t, root)
	for _, rel := range registered {
		readmeRel := rel + "/readme.md"
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(readmeRel))); err != nil {
			t.Fatalf("registered component %s has no readable contract: %v", rel, err)
		}
		if !strings.Contains(inventory, readmeRel) {
			t.Fatalf("component inventory does not cover registered component %s", rel)
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
