package activationports

import (
	"context"
	"errors"
	"testing"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSelectedWorkspaceMemoryItemsLoadsDefaultProviderOnlyForImplicitSelection(t *testing.T) {
	ctx := context.Background()
	calls := 0
	loader := func(context.Context) (string, error) {
		calls++
		return "", errors.New("config unavailable")
	}

	if _, _, err := SelectedWorkspaceMemoryItems(ctx, WorkspaceMemorySelectionRequest{
		WorkspaceID:                    "ws-001",
		IDs:                            []string{"mem-001"},
		DefaultEmbeddingProviderLoader: loader,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("explicit selection err = %v, want invalid argument without hub", err)
	}
	if calls != 0 {
		t.Fatalf("loader calls for explicit memory selection = %d, want 0", calls)
	}

	if _, _, err := SelectedWorkspaceMemoryItems(ctx, WorkspaceMemorySelectionRequest{
		WorkspaceID:                    "ws-001",
		DefaultEmbeddingProviderLoader: loader,
	}); err != nil {
		t.Fatalf("implicit selection err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("loader calls for implicit memory selection = %d, want 1", calls)
	}
}

func TestSelectedWorkspaceMemoryItemsReadsExplicitIDsWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	defer runtime.Shutdown(ctx)
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-memory-selection-scope")
	if err := exports.Repositories.MemoryItems.Create(ctx, &memorymodel.MemoryItem{ID: "memory-selection-scope", WorkspaceID: workspaceID, Summary: "scoped memory", Visibility: "llm"}); err != nil {
		t.Fatal(err)
	}

	items, evidence, err := SelectedWorkspaceMemoryItems(ctx, WorkspaceMemorySelectionRequest{
		WorkspaceID: workspaceID,
		IDs:         []string{"memory-selection-scope"},
		Hub:         exports.Infrastructure.CommandHub,
	})
	if err != nil {
		t.Fatalf("SelectedWorkspaceMemoryItems err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "memory-selection-scope" {
		t.Fatalf("items = %+v, want scoped explicit memory", items)
	}
	if evidence["policy"] != "explicit_memory_selection" {
		t.Fatalf("evidence = %+v, want explicit policy", evidence)
	}
}

func TestSelectedWorkspaceMCPAllowsEmptySelectionWithoutHub(t *testing.T) {
	result, err := SelectedWorkspaceMCP(context.Background(), WorkspaceMCPSelectionRequest{})
	if err != nil {
		t.Fatalf("SelectedWorkspaceMCP err = %v", err)
	}
	if len(result.MCPServerIDs) != 0 || len(result.CapabilityIDs) != 0 {
		t.Fatalf("selection IDs = servers:%#v capabilities:%#v, want empty", result.MCPServerIDs, result.CapabilityIDs)
	}
	if result.Context()["policy"] != "template_default_and_skill_required_mcp_auto_bind" {
		t.Fatalf("selection context = %+v", result.Context())
	}
}

func TestSelectWorkspaceExecutionSkillsAutoSelectsRepositorySkill(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	if err := exports.Repositories.Skills.Create(ctx, &skillmodel.Skill{
		ID:           "skill-auto-runports",
		Name:         "Auto Runports Skill",
		TriggerRules: []string{"auto runports"},
	}); err != nil {
		t.Fatalf("seed skill: %v", err)
	}

	result, err := SelectWorkspaceExecutionSkills(ctx, WorkspaceSkillSelectionRequest{
		Hub:         exports.Infrastructure.CommandHub,
		WorkspaceID: "ws-auto-runports",
		Goal:        &workspacemodel.Goal{Text: "complete auto runports verification"},
		DoD:         &workspacemodel.DoD{Items: []string{"auto runports evidence"}},
		Soul:        &soulmodel.Soul{AllowedSkillRefs: []string{"skill-auto-runports"}},
	})
	if err != nil {
		t.Fatalf("SelectWorkspaceExecutionSkills: %v", err)
	}
	if len(result.Skills) != 1 || result.Skills[0].ID != "skill-auto-runports" {
		t.Fatalf("skills = %+v, want auto-selected repository skill", result.Skills)
	}
	if len(result.IDs) != 1 || result.IDs[0] != "skill-auto-runports" || result.Evidence.Status != "selected" {
		t.Fatalf("selection = ids:%+v evidence:%+v, want selected skill", result.IDs, result.Evidence)
	}
}

func TestSelectWorkspaceExecutionSkillsResolvesExplicitRepositorySkill(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	if err := exports.Repositories.Skills.Create(ctx, &skillmodel.Skill{
		ID:   "skill-explicit-runports",
		Name: "Explicit Runports Skill",
	}); err != nil {
		t.Fatalf("seed skill: %v", err)
	}

	result, err := SelectWorkspaceExecutionSkills(ctx, WorkspaceSkillSelectionRequest{
		Hub:          exports.Infrastructure.CommandHub,
		WorkspaceID:  "ws-explicit-runports",
		RequestedIDs: []string{" skill-explicit-runports "},
	})
	if err != nil {
		t.Fatalf("SelectWorkspaceExecutionSkills explicit: %v", err)
	}
	if len(result.Skills) != 1 || result.Skills[0].ID != "skill-explicit-runports" {
		t.Fatalf("skills = %+v, want explicit repository skill", result.Skills)
	}
	if len(result.IDs) != 1 || result.IDs[0] != "skill-explicit-runports" || result.Evidence.Policy != "explicit_skill_selection" {
		t.Fatalf("selection = ids:%+v evidence:%+v, want explicit evidence", result.IDs, result.Evidence)
	}
}
