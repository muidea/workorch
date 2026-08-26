package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestRunOwnerMemoryResourcesLinearizeWithWorkspaceRetirement(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		runs := NewRunRepository()
		steps := NewRunGuardedStepRepository(runs)
		checkpoints := NewRunGuardedRuntimeCheckpointRepository(runs)
		workspaceID := common.WorkspaceID(fmt.Sprintf("ws-retirement-memory-%d", iteration))
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		item := &runmodel.Run{ID: "run-1", WorkspaceID: workspaceID, State: common.RunStateCompleted}
		if err := runs.Create(ctx, item); err != nil {
			t.Fatalf("iteration %d create Run: %v", iteration, err)
		}
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(3)
		var fence *runmodel.WorkspaceRetirementFence
		var fenceErr, stepErr, checkpointErr error
		go func() {
			defer wait.Done()
			<-start
			fence, fenceErr = runs.PrepareWorkspaceRetirement(ctx, memoryRetirementIntent(workspaceID))
		}()
		go func() {
			defer wait.Done()
			<-start
			stepErr = steps.Create(ctx, &runmodel.Step{ID: "step-race", RunID: item.ID})
		}()
		go func() {
			defer wait.Done()
			<-start
			checkpointErr = checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-race", WorkspaceID: workspaceID, RunID: item.ID})
		}()
		close(start)
		wait.Wait()
		if fenceErr != nil || fence == nil {
			t.Fatalf("iteration %d prepare retirement: fence=%+v err=%v", iteration, fence, fenceErr)
		}
		for name, err := range map[string]error{"step": stepErr, "checkpoint": checkpointErr} {
			if err != nil && !errors.Is(err, common.ErrConflict) {
				t.Fatalf("iteration %d %s race error = %v", iteration, name, err)
			}
		}
		if err := steps.Create(ctx, &runmodel.Step{ID: "step-after-fence", RunID: item.ID}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("iteration %d post-fence Step error = %v, want conflict", iteration, err)
		}
		if err := checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-after-fence", WorkspaceID: workspaceID, RunID: item.ID}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("iteration %d post-fence checkpoint error = %v, want conflict", iteration, err)
		}
	}
}

func TestRunOwnerMemoryWorkspacePurgeUsesExactFenceAndPreservesHead(t *testing.T) {
	owner := NewRunOwnerRepositorySet()
	runs := owner.Runs()
	steps := owner.Steps()
	checkpoints := owner.Checkpoints()
	workspaceID := common.WorkspaceID("ws-retirement-memory-purge")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	item := &runmodel.Run{ID: "run-1", WorkspaceID: workspaceID, State: common.RunStateCompleted}
	if err := runs.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err := steps.Create(ctx, &runmodel.Step{ID: "step-1", RunID: item.ID}); err != nil {
		t.Fatal(err)
	}
	if err := checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-1", WorkspaceID: workspaceID, RunID: item.ID}); err != nil {
		t.Fatal(err)
	}
	fence, err := runs.PrepareWorkspaceRetirement(ctx, memoryRetirementIntent(workspaceID))
	if err != nil {
		t.Fatal(err)
	}
	current, err := runs.GetByWorkspaceID(ctx, workspaceID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: workspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash,
		RunDrainingRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
		Runs:                           []deletioncontract.WorkspaceRuntimeQuiescenceRun{{RunID: current.ID, WorkspaceRunGeneration: current.WorkspaceRunGeneration, RunRevision: current.Revision, RunState: current.State}},
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	fence, err = runs.FinalizeWorkspaceRetirement(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	wrong := *fence
	wrong.DeletionID = "deletion-2"
	if _, err := owner.WorkspacePurger().PurgeWorkspaceResources(ctx, wrong); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong-fence purge error = %v, want conflict", err)
	}
	if receipt, err := owner.WorkspacePurger().PurgeWorkspaceResources(ctx, *fence); err != nil || receipt.ValidateFor(*fence) != nil {
		t.Fatal(err)
	}
	if _, err := runs.GetByWorkspaceID(ctx, workspaceID, item.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("purged Run error = %v, want not found", err)
	}
	replayed, err := runs.GetWorkspaceRetirement(ctx, workspaceID)
	if err != nil || !runmodel.SameWorkspaceRetirementFence(*replayed, *fence) {
		t.Fatalf("retirement head after purge = %+v err=%v", replayed, err)
	}
	if err := runs.CreateNextWorkspaceRun(ctx, &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStatePending}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("post-purge create error = %v, want conflict", err)
	}
}

func TestRunOwnerCanonicalBundleDoesNotExposePartialPurgeLanes(t *testing.T) {
	owner := NewRunOwnerRepositorySet()
	if _, ok := owner.Runs().(interface {
		PurgeWorkspaceRuns(context.Context, runmodel.WorkspaceRetirementFence) error
	}); ok {
		t.Fatal("Run repository exposes a partial Workspace purge lane")
	}
	if _, ok := owner.Steps().(interface {
		PurgeWorkspaceSteps(context.Context, runmodel.WorkspaceRetirementFence, common.RunID) error
	}); ok {
		t.Fatal("Step repository exposes a partial Workspace purge lane")
	}
	if _, ok := owner.Checkpoints().(interface {
		PurgeWorkspaceCheckpoints(context.Context, runmodel.WorkspaceRetirementFence, common.RunID) error
	}); ok {
		t.Fatal("Runtime checkpoint repository exposes a partial Workspace purge lane")
	}
}

func memoryRetirementIntent(workspaceID common.WorkspaceID) runmodel.WorkspaceRetirementIntent {
	return runmodel.WorkspaceRetirementIntent{
		WorkspaceID: workspaceID, DeletionID: "deletion-1", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModePurge, WorkspaceIntentHash: "workspace-intent:" + string(workspaceID),
	}
}
