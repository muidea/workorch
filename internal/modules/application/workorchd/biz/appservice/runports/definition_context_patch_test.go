package runports

import (
	"context"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceDefinitionContextPatchUsesEventHubOwnerReads(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()

	workspaceID := common.WorkspaceID("ws-runports-definition")
	goalID := common.GoalID("goal-runports-definition")
	dodID := common.DoDID("dod-runports-definition")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "runports definition",
		DeliverableDirectory: "workspaces/ws-runports-definition",
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, &workspacemodel.Goal{
		ID:          goalID,
		WorkspaceID: workspaceID,
		Text:        "current goal",
	}); err != nil {
		t.Fatalf("seed goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, &workspacemodel.DoD{
		ID:          dodID,
		WorkspaceID: workspaceID,
		Items:       []string{"definition context patch"},
		State:       common.DoDStateReady,
	}); err != nil {
		t.Fatalf("seed dod: %v", err)
	}

	patch, err := WorkspaceDefinitionContextPatch(ctx, exports.Infrastructure.CommandHub, &runmodel.Run{
		ID:          "run-runports-definition",
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		DoDID:       dodID,
	})
	if err != nil {
		t.Fatalf("WorkspaceDefinitionContextPatch: %v", err)
	}
	if patch["goalId"] != string(goalID) || patch["doDId"] != string(dodID) {
		t.Fatalf("patch identity = %+v, want selected goal/dod", patch)
	}
	goal, _ := patch["goal"].(map[string]any)
	if goal["id"] != string(goalID) || goal["text"] != "current goal" {
		t.Fatalf("goal context = %+v, want seeded goal", goal)
	}
	dod, _ := patch["dod"].(map[string]any)
	if dod["id"] != string(dodID) {
		t.Fatalf("dod context = %+v, want seeded DoD", dod)
	}
	revision, _ := patch["definitionRevision"].(map[string]any)
	if revision["policy"] != "workspace_goal_dod_cumulative" ||
		revision["effectiveGoalId"] != string(goalID) ||
		revision["effectiveDoDId"] != string(dodID) {
		t.Fatalf("definitionRevision = %+v, want effective definition ids", revision)
	}
}

func TestWorkspaceDefinitionContextPatchSkipsIncompleteRun(t *testing.T) {
	patch, err := WorkspaceDefinitionContextPatch(context.Background(), nil, &runmodel.Run{})
	if err != nil {
		t.Fatalf("WorkspaceDefinitionContextPatch: %v", err)
	}
	if patch != nil {
		t.Fatalf("patch = %+v, want nil for incomplete run", patch)
	}
}
