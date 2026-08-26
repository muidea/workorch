package workorchrepo

import (
	"context"
	"errors"
	"sync"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceScopedRunResourcesLinearizeRetirementAcrossRepositorySets(t *testing.T) {
	dataRoot := t.TempDir()
	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	second := NewWorkspaceScopedFileRepositorySet(dataRoot)
	workspaceID := common.WorkspaceID("ws-run-resource-retirement-race")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	run := &runmodel.Run{ID: "run-1", WorkspaceID: workspaceID, State: common.RunStateCompleted}
	if err := first.Runs.Create(ctx, run); err != nil {
		t.Fatalf("create Run: %v", err)
	}
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(3)
	var fence *runmodel.WorkspaceRetirementFence
	var fenceErr, stepErr, checkpointErr error
	go func() {
		defer wait.Done()
		<-start
		fence, fenceErr = first.Runs.PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
			WorkspaceID: workspaceID, DeletionID: "deletion-1", DeletionGeneration: 1,
			DataDispositionIntent: deletioncontract.ModePurge, WorkspaceIntentHash: "workspace-intent:" + string(workspaceID),
		})
	}()
	go func() {
		defer wait.Done()
		<-start
		stepErr = second.Steps.Create(ctx, &runmodel.Step{ID: "step-race", RunID: run.ID})
	}()
	go func() {
		defer wait.Done()
		<-start
		checkpointErr = second.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-race", WorkspaceID: workspaceID, RunID: run.ID})
	}()
	close(start)
	wait.Wait()
	if fenceErr != nil || fence == nil {
		t.Fatalf("prepare retirement: fence=%+v err=%v", fence, fenceErr)
	}
	for name, err := range map[string]error{"step": stepErr, "checkpoint": checkpointErr} {
		if err != nil && !errors.Is(err, common.ErrConflict) {
			t.Fatalf("%s race error = %v", name, err)
		}
	}
	if err := second.Steps.Create(ctx, &runmodel.Step{ID: "step-after-fence", RunID: run.ID}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("post-fence Step error = %v, want conflict", err)
	}
	if err := second.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-after-fence", WorkspaceID: workspaceID, RunID: run.ID}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("post-fence checkpoint error = %v, want conflict", err)
	}
	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	replayed, err := reopened.Runs.GetWorkspaceRetirement(ctx, workspaceID)
	if err != nil || !runmodel.SameWorkspaceRetirementFence(*replayed, *fence) {
		t.Fatalf("reopened retirement = %+v err=%v", replayed, err)
	}
	current, err := reopened.Runs.GetByWorkspaceID(ctx, workspaceID, run.ID)
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
	fence, err = reopened.Runs.FinalizeWorkspaceRetirement(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := reopened.RunOwner.WorkspacePurger().PurgeWorkspaceResources(ctx, *fence)
	if err != nil || receipt.ValidateFor(*fence) != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceScopedFileRepositorySet(dataRoot).Runs.GetWorkspaceRetirement(ctx, workspaceID); err != nil {
		t.Fatalf("retirement head was not preserved: %v", err)
	}
}
