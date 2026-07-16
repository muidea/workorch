package workspacecore

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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
			"versionControl": map[string]any{
				"initializeRepository": true,
			},
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
	mode, _ := got.ContextHints.String("mode")
	if mode != "verify" {
		t.Fatalf("context hints were not cloned: %v", got.ContextHints)
	}
	versionControl, ok := got.ContextHints.Object("versionControl")
	if !ok {
		t.Fatalf("version control template declaration was removed: %v", got.ContextHints)
	}
	if initializeRepository, ok := versionControl.Bool("initializeRepository"); !ok || !initializeRepository {
		t.Fatalf("version control template declaration = %v, want initializeRepository=true", versionControl)
	}
	if _, ok := got.ContextHints.Field("managedWorkspaceConfigs"); ok {
		t.Fatalf("managedWorkspaceConfigs were copied into workspace context hints: %v", got.ContextHints)
	}
	semanticHints, _ := got.ContextHints.Object("semanticCodeNavigation")
	if _, ok := semanticHints.Field("defaultMCPServerIds"); ok {
		t.Fatalf("default MCP server IDs were copied into workspace context hints: %v", got.ContextHints)
	}
	kind, _ := got.EvidencePolicies[0].String("kind")
	if kind != "validation" {
		t.Fatalf("evidence policies were not cloned: %v", got.EvidencePolicies)
	}
}
