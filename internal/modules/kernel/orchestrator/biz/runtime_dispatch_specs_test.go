package orchestrator

import (
	"context"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestNextDispatchSpecsFromCompletedStepContinuesCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	focus := map[string]any{
		"id":                  "focus-cwi-030",
		"coverageWorkItemIds": []string{"cwi-030"},
		"targetRefs":          []string{"cwi-030", "deliverable:source"},
	}
	run := &runmodel.Run{
		ID:              "run-authority-continuation",
		WorkspaceID:     "ws-authority-continuation",
		DoDID:           "dod-authority-continuation",
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			coverageFocusKey: focus,
			convergenceAuthorityKey: map[string]any{
				"currentFocus": focus,
				"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			gapBaselineKey: map[string]any{
				"currentFocus": focus,
				"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
		},
	}
	last := &runmodel.Step{
		ID:    "run-authority-continuation/graph-005/001-completed-node",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphNodeID: "completed-node",
		},
		Output: map[string]any{"status": "completed"},
	}
	if err := stepRepo.Create(ctx, last); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, last)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want one authority continuation spec", specs)
	}
	if !strings.Contains(string(specs[0].ID), "authority-continuation") {
		t.Fatalf("spec ID = %s, want authority continuation", specs[0].ID)
	}
	if specs[0].Kind != llmWorkspaceExecutionStepKind {
		t.Fatalf("spec kind = %q, want %q", specs[0].Kind, llmWorkspaceExecutionStepKind)
	}
	if specs[0].Input["previousStepId"] != string(last.ID) {
		t.Fatalf("previousStepId = %v, want %s", specs[0].Input["previousStepId"], last.ID)
	}
	if got := convergenceNextActionKind(run, specs[0].Input); got != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("next action = %q, want state_change; input=%+v", got, specs[0].Input)
	}
}
