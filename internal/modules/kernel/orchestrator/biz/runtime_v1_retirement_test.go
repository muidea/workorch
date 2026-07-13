package orchestrator

import (
	"context"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReleaseRuntimeV1ResourcesClosesPendingRuntimeWorkIdempotently(t *testing.T) {
	ctx := context.Background()
	runs := memoryrepo.NewRunRepository()
	steps := memoryrepo.NewStepRepository()
	workspaces := memoryrepo.NewWorkspaceRepository()
	toolCalls := memoryrepo.NewToolCallRepository()
	continuations := memoryrepo.NewArtifactContinuationRepository()
	workspaceID := common.WorkspaceID("ws-v1-release")
	runID := common.RunID("run-v1-release")
	stepID := common.StepID("run-v1-release/step-1")
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, State: common.StepStateExecuting, Input: map[string]any{"validationResources": []map[string]any{{"id": "scratch", "kind": "scratch_dir", "status": "active"}}}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := toolCalls.Create(ctx, &runmodel.ToolCall{ID: "tool-v1-release", WorkspaceID: workspaceID, RunID: runID, StepID: stepID, State: common.ToolCallStateRunning}); err != nil {
		t.Fatalf("seed tool call: %v", err)
	}
	if err := continuations.Create(ctx, &runtimecontract.ArtifactContinuation{ID: "continuation-v1-release", WorkspaceID: workspaceID, RunID: runID, State: common.ArtifactContinuationStateChunking}); err != nil {
		t.Fatalf("seed continuation: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(runs, steps, workspaces, memoryrepo.NewDecisionTicketRepository(), memoryrepo.NewDecisionResolutionRepository(), memoryrepo.NewApprovalRepository(), RuntimeEvidenceRepositories{ToolCalls: toolCalls, ArtifactContinuations: continuations})
	first, err := orch.ReleaseRuntimeV1Resources(ctx, workspaceID, runID)
	if err != nil {
		t.Fatalf("ReleaseRuntimeV1Resources: %v", err)
	}
	if first.ReleasedValidationResources != 1 || first.ClosedSteps != 1 || first.ClosedToolCalls != 1 || first.ClosedArtifactContinuations != 1 || first.Total() != 4 {
		t.Fatalf("first release = %+v", first)
	}
	second, err := orch.ReleaseRuntimeV1Resources(ctx, workspaceID, runID)
	if err != nil || second.Total() != 0 {
		t.Fatalf("second release = %+v err=%v", second, err)
	}
}
