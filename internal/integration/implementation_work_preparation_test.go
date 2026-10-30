package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

// Only external SkillHub/protocol transports are fixtures. Preparation,
// source compilation, all-of checks, completion and activation remain real
// production owners and public HTTP. No receipt/readiness value is seeded.
func implementationWorkFullflowPreparationEnvironment(t *testing.T) (string, string, string) {
	t.Helper()
	source, err := filepath.Abs("../../scripts/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	base := "/tmp/workorch-validation"
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "public-fullflow-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if os.Getenv("WORKORCH_PUBLIC_WORK_KEEP_EVIDENCE") == "1" {
			// Preserve the complete isolated owner stores, including Shell raw
			// output. The Run export alone cannot prove those referenced bytes.
			// This affects cleanup only; all deadlines and exit assertions stay.
			t.Logf("restricted fullflow owner evidence retained: %s", root)
			return
		}
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{"data", "project", "bin", "home", "installed"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile(filepath.Join(source, "step04-skill-hub.py"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "skill-hub"), script, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKORCH_STEP04_FIXTURE_ROOT", root)
	t.Setenv("SKILL_HUB_HOME", filepath.Join(root, "home"))
	t.Setenv("SKILL_HUB_DISABLE_SERVICE_BRIDGE", "true")
	for _, name := range []string{"CODEX_SKILLS_DIR", "CLAUDE_SKILLS_DIR", "OPENCODE_SKILLS_DIR"} {
		t.Setenv(name, filepath.Join(root, "installed"))
	}
	t.Setenv("WORKORCH_STEP04_DROP_REGISTRATION_REPLY", "0")
	t.Chdir(root)
	return filepath.Join(root, "data"), filepath.Join(root, "project"), filepath.Join(source, "step04-mcp-protocol.py")
}

func implementationWorkFullflowPreparationMCP(t *testing.T, client *http.Client, base string, fixture string) {
	t.Helper()
	var envelope struct {
		Server management.MCPServer `json:"server"`
	}
	productionJSON(t, client, http.MethodGet, base+"/api/v1/workorch/mcp/servers/mcp-lsp-bridge", nil, &envelope)
	server := envelope.Server
	if server.ID != "mcp-lsp-bridge" {
		t.Fatal("missing default MCP declaration")
	}
	names, err := json.Marshal(server.AllowedTools)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKORCH_STEP04_FIXTURE_TOOLS", string(names))
	productionJSON(t, client, http.MethodPut, base+"/api/v1/workorch/mcp/servers/"+string(server.ID), map[string]any{
		"name": server.Name, "transport": "stdio", "command": "python3", "args": []string{fixture},
		"enabled": server.Enabled, "allowedTools": server.AllowedTools, "allowedResources": server.AllowedResources, "allowedPrompts": server.AllowedPrompts,
		"sessionPolicy": server.SessionPolicy, "timeoutMillis": server.TimeoutMillis,
	}, nil)
}

func implementationWorkFullflowPrepare(t *testing.T, ctx context.Context, client *http.Client, path string, repos workorchrepo.WorkspaceScopedFileRepositorySet, id common.WorkspaceID) {
	t.Helper()
	current, err := repos.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var initialized management.PreparationResourceInitializationOperation
	productionJSON(t, client, http.MethodPost, path+"/activation-preparation/initialize-resources", management.PreparationCheckRequest{RequestID: "fullflow-initialize-original", ExpectedWorkspaceRevision: current.Revision}, &initialized)
	if len(initialized.Preparation.ResourceInitializations) != 1 || initialized.Preparation.ResourceInitializations[0].State != "completed" {
		t.Fatalf("real preparation initialization incomplete: %+v", initialized)
	}
	current, err = repos.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var prepared management.PreparationOperationView
	productionJSON(t, client, http.MethodPost, path+"/activation-preparation/check", management.PreparationCheckRequest{RequestID: "fullflow-check-original", ExpectedWorkspaceRevision: current.Revision}, &prepared)
	if prepared.Preparation.OperationState != "completed" || prepared.Preparation.Record == nil || len(prepared.Preparation.Record.Completions) != 1 {
		t.Fatalf("real all-of preparation incomplete: %+v", prepared)
	}
	assertNoRun(t, repos)
}

// Separate activation test; it cannot satisfy the fullflow terminal/ten-minute
// gate, which still runs its unchanged assertions after the same public setup.
func TestImplementationWorkPublicPreparationAndFirstExecution(t *testing.T) {
	ctx, repos, _, id, _, _, _ := implementationWorkFullflowActivate(t)
	w, err := repos.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := repos.Runs.List(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatal("exactly one explicit Run required", err, len(runs))
	}
	if w.FirstExecutionPreparationEvidence == nil || w.PreparationFence == nil {
		t.Fatal("first activation lost complete consumption evidence")
	}
	var completion string
	if w.PreparationFence.Pending != nil {
		completion = w.PreparationFence.Pending.PreparationCompletionID
	}
	if len(w.PreparationFence.Settled) == 1 {
		completion = w.PreparationFence.Settled[0].Reservation.PreparationCompletionID
	}
	if completion == "" || w.StartupPreparation == nil || len(w.StartupPreparation.Completions) != 1 || completion != w.StartupPreparation.Completions[0].CompletionID {
		t.Fatal("activation did not consume exact completion")
	}
}
