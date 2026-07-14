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
		"internal/services/workorch/biz/backends/cli_typed_response.go",
		"internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go",
		"internal/modules/application/workorchd/pkg/events/cli_contracts.go",
		"internal/modules/application/workorchd/pkg/events/messages.go",
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
			"decodeResultResource",
			"decodeResultResources",
			"func resourceMap(",
			"typedWorkspaceSnapshotInfo",
			"type CLICommand struct",
			"type CLIResult struct",
			"TopicCLICommand =",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains forbidden internal DTO JSON boundary pattern %q", rel, forbidden)
			}
		}
	}
}
