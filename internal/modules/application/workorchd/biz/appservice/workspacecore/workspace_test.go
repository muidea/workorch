package workspacecore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestApplyTemplateToWorkspace(t *testing.T) {
	t.Parallel()

	item := &workspacemodel.Workspace{ID: common.WorkspaceID("ws-001")}
	template := WorkspaceTemplate{
		ID:            " template-001 ",
		DefaultSoulID: " soul-execution ",
		SoulBindings: map[string]string{
			"execution": " soul-execution ",
		},
		ContextHints: map[string]any{
			"mode": "verify",
			"semanticCodeNavigation": map[string]any{
				"defaultMCPServerIds": []any{"mcp-lsp-bridge"},
			},
			"managedWorkspaceConfigs": []any{
				map[string]any{"path": "lsp-bridge/mcp-config.json"},
			},
		},
		EvidencePolicies: []map[string]any{
			{"kind": "validation"},
		},
	}

	got, err := ApplyTemplateToWorkspace(item, template)
	if err != nil {
		t.Fatalf("ApplyTemplateToWorkspace: %v", err)
	}
	if got.TemplateID != "template-001" || got.ProfileID != "template-001" {
		t.Fatalf("template/profile IDs = %q/%q, want template-001/template-001", got.TemplateID, got.ProfileID)
	}
	if got.DefaultSoulID != common.SoulID("soul-execution") {
		t.Fatalf("default soul = %q, want soul-execution", got.DefaultSoulID)
	}
	if got.SoulBindings["execution"] != common.SoulID("soul-execution") {
		t.Fatalf("execution soul binding = %q, want soul-execution", got.SoulBindings["execution"])
	}

	template.ContextHints["mode"] = "mutated"
	template.EvidencePolicies[0]["kind"] = "mutated"
	if got.ContextHints["mode"] != "verify" {
		t.Fatalf("context hints were not cloned: %v", got.ContextHints)
	}
	if _, ok := got.ContextHints["managedWorkspaceConfigs"]; ok {
		t.Fatalf("managedWorkspaceConfigs were copied into workspace context hints: %v", got.ContextHints)
	}
	semanticHints, _ := got.ContextHints["semanticCodeNavigation"].(map[string]any)
	if _, ok := semanticHints["defaultMCPServerIds"]; ok {
		t.Fatalf("default MCP server IDs were copied into workspace context hints: %v", got.ContextHints)
	}
	if got.EvidencePolicies[0]["kind"] != "validation" {
		t.Fatalf("evidence policies were not cloned: %v", got.EvidencePolicies)
	}
}

func TestInitializeGitRepositorySkipsExistingGitDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	if err := InitializeGitRepository(context.Background(), dir); err != nil {
		t.Fatalf("InitializeGitRepository existing git err = %v, old PATH was %q", err, oldPath)
	}
}

func TestInitializeGitRepositoryUsesGitBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell git shim")
	}
	dir := t.TempDir()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	gitPath := filepath.Join(binDir, "git")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + shellQuote(logPath) + "\n" +
		"if [ \"$1\" = \"init\" ]; then /bin/mkdir -p .git; fi\n"
	if err := os.WriteFile(gitPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", binDir)

	if err := InitializeGitRepository(context.Background(), dir); err != nil {
		t.Fatalf("InitializeGitRepository err = %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read git log: %v", err)
	}
	log := string(raw)
	for _, expected := range []string{
		"init -q",
		"config user.name WorkOrch",
		"config user.email workorch@example.local",
		"config core.hooksPath /dev/null",
	} {
		if !strings.Contains(log, expected) {
			t.Fatalf("git log missing %q in:\n%s", expected, log)
		}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
