package orchestrator

import (
	"context"
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestratorRecordRuntimeStepProgressRequiresWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		nil,
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository()},
		events.NewMemoryHub(),
	)

	err := orch.RecordRuntimeStepProgress(ctx, runtimemodel.RuntimeStepExecutionProgressed{
		RunID:  common.RunID("run-progress-missing-workspace"),
		StepID: common.StepID("run-progress-missing-workspace/step-1"),
		Event:  "runtime_step_progress",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("progress without workspace err = %v, want invalid argument", err)
	}
}

func TestRuntimeOrchestratorRecordRuntimeStepProgressUsesScopedRunLookup(t *testing.T) {
	ctx := context.Background()
	runRepo := newRunStateCountingRunRepository()
	stepRepo := memory.NewStepRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		nil,
		runRepo,
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{RuntimeCheckpoints: checkpointRepo},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-progress-scoped-lookup")
	runID := common.RunID("run-progress-scoped-lookup")
	stepID := common.StepID("run-progress-scoped-lookup/step-1")
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: llmWorkspaceExecutionStepKind, State: common.StepStateExecuting}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	err := orch.RecordRuntimeStepProgress(ctx, runtimemodel.RuntimeStepExecutionProgressed{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Event:       "runtime_step_progress",
		Stage:       "streaming",
		Summary:     "progress",
		Data:        map[string]any{"chunks": 1},
	})
	if err != nil {
		t.Fatalf("record progress: %v", err)
	}
	if runRepo.getCalls.Load() != 0 {
		t.Fatalf("global Get calls = %d, want 0", runRepo.getCalls.Load())
	}
	if runRepo.getByWorkspaceIDCalls.Load() != 1 {
		t.Fatalf("GetByWorkspaceID calls = %d, want 1", runRepo.getByWorkspaceIDCalls.Load())
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 || checkpoints[0].WorkspaceID != workspaceID {
		t.Fatalf("checkpoints = %+v, want one workspace-scoped checkpoint", checkpoints)
	}
}

func TestRuntimeOrchestratorTerminalResultUsesScopedRunLookup(t *testing.T) {
	ctx := context.Background()
	runRepo := newRunStateCountingRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		nil,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{RuntimeCheckpoints: checkpointRepo},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-terminal-scoped-lookup")
	runID := common.RunID("run-terminal-scoped-lookup")
	stepID := common.StepID("run-terminal-scoped-lookup/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: llmWorkspaceExecutionStepKind, State: common.StepStateExecuting}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if err := orch.RecordRuntimeStepTerminalResult(scopedCtx, runID, stepID, RuntimeStepTerminalResultCheckpoint{
		RequestID:     "request-terminal-scoped",
		TerminalTopic: runtimeStepResultAvailableEvent,
		Output:        map[string]any{"status": "ok"},
	}); err != nil {
		t.Fatalf("record terminal result: %v", err)
	}
	if err := orch.FinalizeRuntimeStepResult(scopedCtx, runID, stepID, runtimemodel.StepExecutionResult{Output: map[string]any{"status": "ok"}}, nil); err != nil {
		t.Fatalf("finalize terminal result: %v", err)
	}
	if runRepo.getCalls.Load() != 0 {
		t.Fatalf("global Get calls = %d, want 0", runRepo.getCalls.Load())
	}
	if runRepo.getByWorkspaceIDCalls.Load() != 3 {
		t.Fatalf("GetByWorkspaceID calls = %d, want 3", runRepo.getByWorkspaceIDCalls.Load())
	}
}
