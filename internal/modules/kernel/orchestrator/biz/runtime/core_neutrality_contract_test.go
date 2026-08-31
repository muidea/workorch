package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuntimeCoreClassificationDoesNotDependOnCapabilityPrivateVocabulary
// keeps owner-private diagnostic codes and concrete capability identities out
// of the Runtime reducer. Owners retain their exact diagnostics for audit, but
// only typed statuses, sealed descriptors and Profile declarations may select
// an SM-2/SM-3 transition.
func TestRuntimeCoreClassificationDoesNotDependOnCapabilityPrivateVocabulary(t *testing.T) {
	files := make([]string, 0, 64)
	if err := filepath.WalkDir(".", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		t.Fatalf("enumerate Runtime core source: %v", err)
	}
	// This outer owner adapter classifies typed provider feedback before it
	// enters a private Runtime fact. It may route by typed category, but must not
	// interpret provider-private codes or capability identities.
	files = append(files, "../runtime_semantic_drive.go")
	if len(files) < 2 {
		t.Fatal("Runtime core neutrality scanner found no production sources")
	}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read Runtime core source %s: %v", path, err)
		}
		text := string(content)
		for _, owner := range []string{
			"RuntimeModuleFile",
			"RuntimeModuleGit",
			"RuntimeModuleShell",
			"RuntimeModuleSSH",
			"RuntimeModuleWeb",
			"RuntimeModuleMCP",
		} {
			if strings.Contains(text, owner) {
				t.Fatalf("Runtime core source %s hard-codes capability owner %s", path, owner)
			}
		}
		for _, prefix := range []string{`"file_`, `"git_`, `"shell_`, `"ssh_`, `"web_`, `"mcp_`} {
			if strings.Contains(text, prefix) {
				t.Fatalf("Runtime core source %s interprets capability-private diagnostic prefix %s", path, prefix)
			}
		}
	}
}
