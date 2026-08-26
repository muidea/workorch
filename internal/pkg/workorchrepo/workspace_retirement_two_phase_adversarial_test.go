package workorchrepo

import (
	"context"
	"errors"
	"fmt"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceScopedFileRetirementReopensDrainingAndCompletesTerminalAckCAS(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-file-two-phase-reopen")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)

	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	item := newFileTwoPhaseRun(workspaceID, 7)
	if err := first.Runs.Create(ctx, item); err != nil {
		t.Fatalf("create Run: %v", err)
	}
	fence := prepareFileTwoPhaseRetirement(t, first, ctx, workspaceID)

	second := NewWorkspaceScopedFileRepositorySet(dataRoot)
	reopenedFence, err := second.Runs.GetWorkspaceRetirement(ctx, workspaceID)
	if err != nil || !runmodel.SameWorkspaceRetirementFence(*reopenedFence, *fence) || !reopenedFence.IsDraining() {
		t.Fatalf("reopened draining fence = %+v err=%v", reopenedFence, err)
	}
	current := getFileTwoPhaseRun(t, second, ctx, workspaceID)
	updated := cloneFileTwoPhaseRun(current)
	advanceFileTwoPhaseRuntime(updated)
	if err := second.Runs.CompareAndSwapRuntimeState(ctx, updated, current.Revision, current.RuntimeState.StateRevision); err != nil {
		t.Fatalf("terminal-delivery acknowledgement CAS after reopen: %v", err)
	}

	third := NewWorkspaceScopedFileRepositorySet(dataRoot)
	afterAck := getFileTwoPhaseRun(t, third, ctx, workspaceID)
	token := fileTwoPhaseQuiescenceToken(*fence, afterAck)
	retired, err := third.Runs.FinalizeWorkspaceRetirement(ctx, token)
	if err != nil {
		t.Fatalf("finalize after reopen: %v", err)
	}
	if !retired.IsRetired() {
		t.Fatalf("finalized phase = %q, want retired", retired.Phase)
	}

	fourth := NewWorkspaceScopedFileRepositorySet(dataRoot)
	reopenedRetired, err := fourth.Runs.GetWorkspaceRetirement(ctx, workspaceID)
	if err != nil || !runmodel.SameWorkspaceRetirementFence(*reopenedRetired, *retired) {
		t.Fatalf("reopened retired fence = %+v err=%v", reopenedRetired, err)
	}
	latest := getFileTwoPhaseRun(t, fourth, ctx, workspaceID)
	postRetirement := cloneFileTwoPhaseRun(latest)
	advanceFileTwoPhaseRuntime(postRetirement)
	if err := fourth.Runs.CompareAndSwapRuntimeState(ctx, postRetirement, latest.Revision, latest.RuntimeState.StateRevision); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("post-retirement Runtime CAS error = %v, want conflict", err)
	}
}

func TestWorkspaceScopedFileRetirementStaleTokenCannotWinAcrossRepositorySets(t *testing.T) {
	for iteration := 0; iteration < 20; iteration++ {
		dataRoot := t.TempDir()
		workspaceID := common.WorkspaceID(fmt.Sprintf("ws-file-two-phase-stale-%d", iteration))
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		first := NewWorkspaceScopedFileRepositorySet(dataRoot)
		item := newFileTwoPhaseRun(workspaceID, 11)
		if err := first.Runs.Create(ctx, item); err != nil {
			t.Fatalf("iteration %d create Run: %v", iteration, err)
		}
		fence := prepareFileTwoPhaseRetirement(t, first, ctx, workspaceID)
		before := getFileTwoPhaseRun(t, first, ctx, workspaceID)
		staleToken := fileTwoPhaseQuiescenceToken(*fence, before)

		second := NewWorkspaceScopedFileRepositorySet(dataRoot)
		updated := cloneFileTwoPhaseRun(before)
		advanceFileTwoPhaseRuntime(updated)
		if err := second.Runs.CompareAndSwapRuntimeState(ctx, updated, before.Revision, before.RuntimeState.StateRevision); err != nil {
			t.Fatalf("iteration %d Runtime CAS: %v", iteration, err)
		}
		if _, err := first.Runs.FinalizeWorkspaceRetirement(ctx, staleToken); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("iteration %d stale finalize error = %v, want conflict", iteration, err)
		}

		third := NewWorkspaceScopedFileRepositorySet(dataRoot)
		latest := getFileTwoPhaseRun(t, third, ctx, workspaceID)
		freshToken := fileTwoPhaseQuiescenceToken(*fence, latest)
		if _, err := third.Runs.FinalizeWorkspaceRetirement(ctx, freshToken); err != nil {
			t.Fatalf("iteration %d fresh finalize: %v", iteration, err)
		}
	}
}

func TestWorkspaceScopedFileRunOwnerBundleRejectsMixedStores(t *testing.T) {
	first := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	second := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	if mixed, err := repository.NewRunOwnerRepositories(
		first.RunOwner.Runs(), second.RunOwner.Steps(), first.RunOwner.Checkpoints(), first.RunOwner.WorkspacePurger(),
	); !errors.Is(err, common.ErrConflict) || mixed.Present() {
		t.Fatalf("mixed file-store Run-owner bundle=%+v err=%v, want empty conflict", mixed, err)
	}
}

func TestWorkspaceScopedFileRunOwnerPurgeReopensAndCountsOrphans(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-file-purge-orphans")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	orphan := &runmodel.Run{ID: "run-orphan", WorkspaceID: workspaceID, State: common.RunStateCompleted}
	if err := first.Runs.Create(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	if err := first.Steps.Create(ctx, &runmodel.Step{ID: "step-orphan", RunID: orphan.ID}); err != nil {
		t.Fatal(err)
	}
	if err := first.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-orphan", WorkspaceID: workspaceID, RunID: orphan.ID}); err != nil {
		t.Fatal(err)
	}
	if err := first.Runs.Delete(ctx, orphan.ID); err != nil {
		t.Fatalf("delete parent Run while retaining orphan child fixtures: %v", err)
	}
	current := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStateCompleted}
	if err := first.Runs.CreateNextWorkspaceRun(ctx, current); err != nil {
		t.Fatal(err)
	}
	if current.WorkspaceRunGeneration != 2 {
		t.Fatalf("current generation=%d, want legal gap at generation 1", current.WorkspaceRunGeneration)
	}
	if err := first.Steps.Create(ctx, &runmodel.Step{ID: "step-current", RunID: current.ID}); err != nil {
		t.Fatal(err)
	}
	if err := first.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-current", WorkspaceID: workspaceID, RunID: current.ID}); err != nil {
		t.Fatal(err)
	}
	fence, err := first.Runs.PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
		WorkspaceID: workspaceID, DeletionID: "deletion-file-purge-orphans", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModePurge, WorkspaceIntentHash: "workspace-intent:file-purge-orphans",
	})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := first.Runs.GetByWorkspaceID(ctx, workspaceID, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	retired, err := first.Runs.FinalizeWorkspaceRetirement(ctx, fileNoRuntimeQuiescenceToken(*fence, persisted))
	if err != nil {
		t.Fatalf("finalize retirement with legal generation gap: %v", err)
	}

	// Execute through a freshly reopened repository set so the purger must scan
	// durable roots, including the deleted parent Run's orphan child surfaces.
	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	receipt, err := reopened.RunOwner.WorkspacePurger().PurgeWorkspaceResources(ctx, *retired)
	if err != nil || receipt.ValidateFor(*retired) != nil {
		t.Fatalf("reopened atomic purge receipt=%+v err=%v", receipt, err)
	}
	want := []runmodel.WorkspaceRetirementPurgeObservation{
		// File storage counts the deleted parent's still-present run.json
		// container as physical residue even though logical lookup is empty.
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceRuns, ObservedBefore: 2, DeletedCount: 2},
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceSteps, ObservedBefore: 2, DeletedCount: 2},
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceCheckpoints, ObservedBefore: 2, DeletedCount: 2},
	}
	if fmt.Sprint(receipt.Observations) != fmt.Sprint(want) {
		t.Fatalf("purge observations=%+v, want %+v", receipt.Observations, want)
	}
	third := NewWorkspaceScopedFileRepositorySet(dataRoot)
	for _, runID := range []common.RunID{orphan.ID, current.ID} {
		if items, listErr := third.Steps.ListByRun(ctx, runID); listErr != nil || len(items) != 0 {
			t.Fatalf("post-reopen steps run=%s items=%+v err=%v", runID, items, listErr)
		}
		if items, listErr := third.RuntimeCheckpoints.ListByRun(ctx, runID); listErr != nil || len(items) != 0 {
			t.Fatalf("post-reopen checkpoints run=%s items=%+v err=%v", runID, items, listErr)
		}
	}
	if _, err := third.Runs.GetByWorkspaceID(ctx, workspaceID, current.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("post-reopen current Run error=%v, want not found", err)
	}
	if replayed, err := third.Runs.GetWorkspaceRetirement(ctx, workspaceID); err != nil || !runmodel.SameWorkspaceRetirementFence(*replayed, *retired) {
		t.Fatalf("post-reopen retirement head=%+v err=%v", replayed, err)
	}
	retry, err := third.RunOwner.WorkspacePurger().PurgeWorkspaceResources(ctx, *retired)
	if err != nil || retry.ValidateFor(*retired) != nil {
		t.Fatalf("reopened response-loss retry receipt=%+v err=%v", retry, err)
	}
	if retry.ProofHash != receipt.ProofHash || fmt.Sprint(retry.Observations) != fmt.Sprint(receipt.Observations) || !runmodel.SameWorkspaceRetirementFence(retry.Fence, receipt.Fence) {
		t.Fatalf("response-loss retry invented a new receipt: first=%+v retry=%+v", receipt, retry)
	}
}

func prepareFileTwoPhaseRetirement(t *testing.T, repositories WorkspaceScopedFileRepositorySet, ctx context.Context, workspaceID common.WorkspaceID) *runmodel.WorkspaceRetirementFence {
	t.Helper()
	intent := runmodel.WorkspaceRetirementIntent{
		WorkspaceID: workspaceID, DeletionID: "deletion-" + string(workspaceID), DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent-hash:" + string(workspaceID),
	}
	fence, err := repositories.Runs.PrepareWorkspaceRetirement(ctx, intent)
	if err != nil {
		t.Fatalf("prepare retirement: %v", err)
	}
	return fence
}

func getFileTwoPhaseRun(t *testing.T, repositories WorkspaceScopedFileRepositorySet, ctx context.Context, workspaceID common.WorkspaceID) *runmodel.Run {
	t.Helper()
	item, err := repositories.Runs.GetByWorkspaceID(ctx, workspaceID, "run-001")
	if err != nil {
		t.Fatalf("get Run: %v", err)
	}
	return item
}

func cloneFileTwoPhaseRun(item *runmodel.Run) *runmodel.Run {
	return runmodel.CloneRun(item)
}

func newFileTwoPhaseRun(workspaceID common.WorkspaceID, runtimeRevision uint64) *runmodel.Run {
	terminalID := "terminal-delivery:" + string(workspaceID) + ":run-001"
	state := &runtimecontract.RuntimeStateRecord{StateRevision: runtimeRevision}
	return &runmodel.Run{
		ID: "run-001", WorkspaceID: workspaceID, State: common.RunStateCompleted, RuntimeState: state,
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{
			WorkspaceID: workspaceID, RunID: "run-001", RuntimeStateRevision: runtimeRevision,
			RuntimeStateRef:  fmt.Sprintf("runtime-state/%s/run-001/%d", workspaceID, runtimeRevision),
			RuntimeStateHash: runtimecontract.StableRuntimeStateRecordHash(*state), BootstrapHash: "bootstrap-hash:current", TerminalFenceID: terminalID,
		},
		TerminalDelivery: &runtimecontract.RuntimeTerminalDeliveryIdentity{TerminalDeliveryID: terminalID},
	}
}

func advanceFileTwoPhaseRuntime(item *runmodel.Run) {
	item.RuntimeState.StateRevision++
	item.RuntimeLifecycleFence = &runtimecontract.RuntimeOpaqueLifecycleFence{
		WorkspaceID: item.WorkspaceID, RunID: item.ID, RuntimeStateRevision: item.RuntimeState.StateRevision,
		RuntimeStateRef:  fmt.Sprintf("runtime-state/%s/%s/%d", item.WorkspaceID, item.ID, item.RuntimeState.StateRevision),
		RuntimeStateHash: runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState), BootstrapHash: "bootstrap-hash:current",
		TerminalFenceID: item.TerminalDelivery.TerminalDeliveryID,
	}
}

func fileTwoPhaseQuiescenceToken(fence runmodel.WorkspaceRetirementFence, item *runmodel.Run) deletioncontract.WorkspaceRuntimeQuiescenceToken {
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash,
		RunDrainingRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
		Runs: []deletioncontract.WorkspaceRuntimeQuiescenceRun{{
			RunID: item.ID, WorkspaceRunGeneration: item.WorkspaceRunGeneration, RunRevision: item.Revision, RunState: item.State,
			RuntimeStatePresent: true, RuntimeStateRevision: item.RuntimeState.StateRevision,
			RuntimeStateRef:    fmt.Sprintf("runtime-state/%s/%s/%d", item.WorkspaceID, item.ID, item.RuntimeState.StateRevision),
			RuntimeStateHash:   runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState),
			TerminalDeliveryID: item.TerminalDelivery.TerminalDeliveryID, TerminalDeliveryAcked: true,
		}},
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	return token
}

func fileNoRuntimeQuiescenceToken(fence runmodel.WorkspaceRetirementFence, item *runmodel.Run) deletioncontract.WorkspaceRuntimeQuiescenceToken {
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash,
		RunDrainingRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
		Runs: []deletioncontract.WorkspaceRuntimeQuiescenceRun{{
			RunID: item.ID, WorkspaceRunGeneration: item.WorkspaceRunGeneration, RunRevision: item.Revision, RunState: item.State,
		}},
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	return token
}
