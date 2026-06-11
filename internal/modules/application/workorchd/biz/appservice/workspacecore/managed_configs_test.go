package workspacecore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureManagedConfigsUsesSystemDefaultAndWorkspaceOverlay(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	internalDirectory := filepath.Join(dataRoot, "internal", "workspaces", "ws-managed-config")
	systemHints := map[string]any{
		"managedWorkspaceConfigs": []any{
			map[string]any{
				"path":         "lsp-bridge/mcp-config.json",
				"format":       "json",
				"mergeMissing": true,
				"content": map[string]any{
					"languages": map[string]any{
						"go": map[string]any{"command": []any{"gopls", "serve"}},
					},
				},
			},
		},
	}

	if err := EnsureManagedConfigs(internalDirectory, systemHints, nil); err != nil {
		t.Fatalf("EnsureManagedConfigs system default: %v", err)
	}
	systemPath := filepath.Join(dataRoot, "system", "managed-configs", "lsp-bridge", "mcp-config.json")
	workspacePath := filepath.Join(internalDirectory, "lsp-bridge", "mcp-config.json")
	assertJSONCommand(t, systemPath, "languages.go.command", []string{"gopls", "serve"})
	if _, err := os.Stat(workspacePath); err == nil {
		t.Fatalf("workspace config was created without workspace overlay: %s", workspacePath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat workspace config: %v", err)
	}
	if got := ManagedConfigPathForWorkspace(internalDirectory, "lsp-bridge/mcp-config.json"); got != systemPath {
		t.Fatalf("ManagedConfigPathForWorkspace = %q, want system path %q", got, systemPath)
	}

	workspaceHints := map[string]any{
		"managedWorkspaceConfigs": []any{
			map[string]any{
				"path":         "lsp-bridge/mcp-config.json",
				"format":       "json",
				"mergeMissing": true,
				"content": map[string]any{
					"languages": map[string]any{
						"go": map[string]any{"command": []any{"custom-gopls"}},
					},
				},
			},
		},
	}
	if err := EnsureManagedConfigs(internalDirectory, systemHints, workspaceHints); err != nil {
		t.Fatalf("EnsureManagedConfigs workspace overlay: %v", err)
	}
	assertJSONCommand(t, workspacePath, "languages.go.command", []string{"custom-gopls"})
	if got := ManagedConfigPathForWorkspace(internalDirectory, "lsp-bridge/mcp-config.json"); got != workspacePath {
		t.Fatalf("ManagedConfigPathForWorkspace = %q, want workspace path %q", got, workspacePath)
	}
}

func TestEnsureManagedConfigsRefreshesSystemDefaultWithoutOverwritingWorkspaceOverlay(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	internalDirectory := filepath.Join(dataRoot, "internal", "workspaces", "ws-managed-config-refresh")
	systemHints := managedConfigHints("gopls", "serve")
	workspaceHints := managedConfigHints("custom-gopls")
	if err := EnsureManagedConfigs(internalDirectory, systemHints, workspaceHints); err != nil {
		t.Fatalf("EnsureManagedConfigs initial: %v", err)
	}

	refreshedSystemHints := managedConfigHints("gopls", "serve", "-rpc.trace")
	if err := EnsureManagedConfigs(internalDirectory, refreshedSystemHints, workspaceHints); err != nil {
		t.Fatalf("EnsureManagedConfigs refreshed: %v", err)
	}

	systemPath := filepath.Join(dataRoot, "system", "managed-configs", "lsp-bridge", "mcp-config.json")
	workspacePath := filepath.Join(internalDirectory, "lsp-bridge", "mcp-config.json")
	assertJSONCommand(t, systemPath, "languages.go.command", []string{"gopls", "serve", "-rpc.trace"})
	assertJSONCommand(t, workspacePath, "languages.go.command", []string{"custom-gopls"})
}

func managedConfigHints(command ...string) map[string]any {
	items := make([]any, 0, len(command))
	for _, item := range command {
		items = append(items, item)
	}
	return map[string]any{
		"managedWorkspaceConfigs": []any{
			map[string]any{
				"path":         "lsp-bridge/mcp-config.json",
				"format":       "json",
				"mergeMissing": true,
				"content": map[string]any{
					"languages": map[string]any{
						"go": map[string]any{"command": items},
					},
				},
			},
		},
	}
}

func assertJSONCommand(t *testing.T, path string, _ string, want []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	var payload struct {
		Languages map[string]struct {
			Command []string `json:"command"`
		} `json:"languages"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal %s: %v", path, err)
	}
	got := payload.Languages["go"].Command
	if len(got) != len(want) {
		t.Fatalf("%s command = %v, want %v", path, got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("%s command = %v, want %v", path, got, want)
		}
	}
}
