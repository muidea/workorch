package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInternalDTOJSONBoundaryDoesNotRegress(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../../../.."))
	files := []string{
		"internal/services/workorch/biz/backends/eventhub.go",
		"internal/services/workorch/biz/backends/snapshot.go",
		"internal/modules/application/workorchd/biz/appservice/clicore/resources.go",
		"internal/modules/application/workorchd/biz/appservice/clicore/payload_values.go",
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(data)
		for _, forbidden := range []string{
			`"encoding/json"`,
			"json.Marshal",
			"json.Unmarshal",
			"DecodeOrZero",
			"ResourceMapAlias",
			"resourceMapAlias",
			"typedWorkspaceSnapshotInfo",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains forbidden internal DTO JSON boundary pattern %q", rel, forbidden)
			}
		}
	}
}
