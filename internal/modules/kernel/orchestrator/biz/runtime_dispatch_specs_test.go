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

func TestConvergenceNextActionOutcomeMatrix(t *testing.T) {
	tests := []struct {
		action       string
		wantOutcome  convergenceNextActionOutcome
		wantDispatch bool
	}{
		{action: runtimemodel.NextActionTypeStateChange, wantOutcome: convergenceNextActionOutcomeDispatch, wantDispatch: true},
		{action: runtimemodel.NextActionTypeValidate, wantOutcome: convergenceNextActionOutcomeDispatch, wantDispatch: true},
		{action: runtimemodel.NextActionTypeAssessment, wantOutcome: convergenceNextActionOutcomeDispatch, wantDispatch: true},
		{action: runtimemodel.NextActionTypeAcquireTargetSnapshot, wantOutcome: convergenceNextActionOutcomeDispatch, wantDispatch: true},
		{action: runtimemodel.NextActionTypeRequestDecision, wantOutcome: convergenceNextActionOutcomeGovernanceBlock},
		{action: runtimemodel.NextActionTypeRequestApproval, wantOutcome: convergenceNextActionOutcomeGovernanceBlock},
		{action: runtimemodel.NextActionTypeComplete, wantOutcome: convergenceNextActionOutcomeTerminalComplete},
		{action: runtimemodel.NextActionTypeFail, wantOutcome: convergenceNextActionOutcomeTerminalFail},
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			if got := classifyConvergenceNextActionOutcome(tc.action); got != tc.wantOutcome {
				t.Fatalf("classifyConvergenceNextActionOutcome(%q) = %q, want %q", tc.action, got, tc.wantOutcome)
			}
			if got := authorityContinuationActionIsDispatchable(tc.action); got != tc.wantDispatch {
				t.Fatalf("authorityContinuationActionIsDispatchable(%q) = %v, want %v", tc.action, got, tc.wantDispatch)
			}
		})
	}
}

func TestNextDispatchSpecsFromCompletedStepContinuesAssessmentAuthority(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	focus := map[string]any{
		"id":                    "cwi-assessment",
		"coverageWorkItemIds":   []string{"cwi-assessment"},
		"requiredEvidenceKinds": []string{"validation"},
		"acceptedEvidenceRefs":  []string{"step-validation"},
		"missingEvidenceKinds":  []string{},
		"status":                "evidence_ready",
	}
	run := &runmodel.Run{
		ID:              "run-assessment-continuation",
		WorkspaceID:     "ws-assessment-continuation",
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			coverageFocusKey: map[string]any{
				"id":                  "focus-cwi-assessment",
				"coverageWorkItemIds": []string{"cwi-assessment"},
			},
			convergenceAuthorityKey: map[string]any{
				"currentFocus": focus,
				"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeAssessment},
			},
			gapBaselineKey: map[string]any{
				"currentFocus": focus,
				"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeAssessment},
			},
		},
	}
	last := &runmodel.Step{
		ID:     "run-assessment-continuation/graph-001/001-validation",
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Input:  map[string]any{stepInputKeyGraphNodeID: "validation"},
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
		t.Fatalf("specs = %+v, want one assessment continuation", specs)
	}
	if got := convergenceNextActionKind(run, specs[0].Input); got != runtimemodel.NextActionTypeAssessment {
		t.Fatalf("next action = %q, want assessment; input=%+v", got, specs[0].Input)
	}
	if !strings.Contains(string(specs[0].ID), "authority-continuation") {
		t.Fatalf("spec ID = %s, want authority continuation", specs[0].ID)
	}
}

func TestNextDispatchSpecsFromFailedStepContinuesCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	focus := map[string]any{
		"id":                    "cwi-validation",
		"coverageWorkItemIds":   []string{"cwi-validation"},
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"acceptedEvidenceRefs":  []string{"step-state-change"},
		"missingEvidenceKinds":  []string{"validation"},
		"status":                "in_progress",
	}
	required := concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{
			"coverageWorkItemIds": []string{"cwi-validation"},
			"structuredTargetRefs": []string{
				"cwi-validation",
			},
		},
		nil,
		nil,
	)
	run := &runmodel.Run{
		ID:              "run-failed-authority-continuation",
		WorkspaceID:     "ws-failed-authority-continuation",
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			coverageFocusKey: map[string]any{
				"id":                  "focus-cwi-validation",
				"coverageWorkItemIds": []string{"cwi-validation"},
			},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus":    cloneStepPayload(focus),
				"unresolvedItems": []map[string]any{cloneStepPayload(focus)},
			},
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction": map[string]any{
					"kind":           runtimemodel.NextActionTypeValidate,
					"requiredOutput": required,
				},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction": map[string]any{
					"kind":           runtimemodel.NextActionTypeValidate,
					"requiredOutput": required,
				},
			},
		},
	}
	last := &runmodel.Step{
		ID:     "run-failed-authority-continuation/graph-001/001-validation",
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateFailed,
		Input:  map[string]any{stepInputKeyGraphNodeID: "validation"},
		Output: map[string]any{"status": "failed", "error": "validation failed"},
	}
	if err := stepRepo.Create(ctx, last); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, last)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want one authority continuation", specs)
	}
	if got := convergenceNextActionKind(run, specs[0].Input); got != runtimemodel.NextActionTypeValidate {
		t.Fatalf("next action = %q, want validate; input=%+v", got, specs[0].Input)
	}
	if !strings.Contains(string(specs[0].ID), "authority-continuation") {
		t.Fatalf("spec ID = %s, want authority continuation", specs[0].ID)
	}
}
