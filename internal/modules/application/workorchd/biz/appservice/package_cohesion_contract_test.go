package appservice

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWD6ThinPairsHaveOneOwnerPackage keeps the low-risk appservice merges
// structural.  The merge is intentionally mechanical: content and setupdraft
// own their DTOs, normalization and typed owner/LLM adapters in one package;
// no compatibility package or alias is allowed to reappear.
func TestWD6ThinPairsHaveOneOwnerPackage(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	if _, err := os.Stat(filepath.Join(root, "read_model_projection_service.go")); !os.IsNotExist(err) {
		t.Fatalf("projection forwarding wrapper still exists (err=%v)", err)
	}
	for _, name := range []string{"content", "setupdraft"} {
		path := filepath.Join(root, name)
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatalf("read merged package %s: %v", name, err)
		}
		goFiles := 0
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				goFiles++
			}
		}
		if goFiles < 4 {
			t.Fatalf("merged package %s has %d Go files, want at least 4", name, goFiles)
		}
		for _, legacy := range []string{name + "core", name + "ports"} {
			if _, err := os.Stat(filepath.Join(root, legacy)); !os.IsNotExist(err) {
				t.Fatalf("legacy split package %s still exists (err=%v)", legacy, err)
			}
		}
	}
}
