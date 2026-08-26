package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMemoryWorkspaceRetirementDrainingAllowsPendingTerminalAckCAS(t *testing.T) {
	runs, ctx, workspaceID := newMemoryTwoPhaseRetirementFixture(t, "pending-ack")
	fence := prepareMemoryTwoPhaseRetirement(t, runs, ctx, workspaceID, deletioncontract.ModeRetain)
	if !fence.IsDraining() {
		t.Fatalf("prepare phase = %q, want draining", fence.Phase)
	}

	current := getMemoryTwoPhaseRun(t, runs, ctx, workspaceID)
	updated := cloneMemoryTwoPhaseRun(current)
	advanceMemoryTwoPhaseRuntime(updated)
	if err := runs.CompareAndSwapRuntimeState(ctx, updated, current.Revision, current.RuntimeState.StateRevision); err != nil {
		t.Fatalf("terminal-delivery acknowledgement CAS while draining: %v", err)
	}
	afterAck := getMemoryTwoPhaseRun(t, runs, ctx, workspaceID)
	if afterAck.RuntimeState.StateRevision != current.RuntimeState.StateRevision+1 {
		t.Fatalf("Runtime revision = %d, want %d", afterAck.RuntimeState.StateRevision, current.RuntimeState.StateRevision+1)
	}

	token := memoryTwoPhaseQuiescenceToken(*fence, afterAck)
	retired, err := runs.FinalizeWorkspaceRetirement(ctx, token)
	if err != nil {
		t.Fatalf("finalize retirement: %v", err)
	}
	if !retired.IsRetired() || retired.RuntimeInventoryHash != token.RuntimeInventoryHash || retired.QuiescenceID != token.QuiescenceID {
		t.Fatalf("retired fence = %+v, token = %+v", retired, token)
	}
}

func TestMemoryWorkspaceRetirementCASAndFinalizeLinearize(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		runs, ctx, workspaceID := newMemoryTwoPhaseRetirementFixture(t, fmt.Sprintf("linearize-%d", iteration))
		fence := prepareMemoryTwoPhaseRetirement(t, runs, ctx, workspaceID, deletioncontract.ModeRetain)
		current := getMemoryTwoPhaseRun(t, runs, ctx, workspaceID)
		token := memoryTwoPhaseQuiescenceToken(*fence, current)
		updated := cloneMemoryTwoPhaseRun(current)
		advanceMemoryTwoPhaseRuntime(updated)

		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var casErr, finalizeErr error
		go func() {
			defer wait.Done()
			<-start
			casErr = runs.CompareAndSwapRuntimeState(ctx, updated, current.Revision, current.RuntimeState.StateRevision)
		}()
		go func() {
			defer wait.Done()
			<-start
			_, finalizeErr = runs.FinalizeWorkspaceRetirement(ctx, token)
		}()
		close(start)
		wait.Wait()

		if (casErr == nil) == (finalizeErr == nil) {
			t.Fatalf("iteration %d CAS err=%v finalize err=%v, want exactly one winner", iteration, casErr, finalizeErr)
		}
		if casErr != nil && !errors.Is(casErr, common.ErrConflict) {
			t.Fatalf("iteration %d CAS error = %v, want conflict", iteration, casErr)
		}
		if finalizeErr != nil && !errors.Is(finalizeErr, common.ErrConflict) {
			t.Fatalf("iteration %d finalize error = %v, want conflict", iteration, finalizeErr)
		}
		if casErr == nil {
			latest := getMemoryTwoPhaseRun(t, runs, ctx, workspaceID)
			freshToken := memoryTwoPhaseQuiescenceToken(*fence, latest)
			if _, err := runs.FinalizeWorkspaceRetirement(ctx, freshToken); err != nil {
				t.Fatalf("iteration %d finalize fresh token after CAS: %v", iteration, err)
			}
		}
		latest := getMemoryTwoPhaseRun(t, runs, ctx, workspaceID)
		latestUpdate := cloneMemoryTwoPhaseRun(latest)
		advanceMemoryTwoPhaseRuntime(latestUpdate)
		if err := runs.CompareAndSwapRuntimeState(ctx, latestUpdate, latest.Revision, latest.RuntimeState.StateRevision); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("iteration %d post-retirement CAS error = %v, want conflict", iteration, err)
		}
	}
}

func TestMemoryWorkspaceRetirementTokenIsScopedWithDuplicateRunID(t *testing.T) {
	runs := NewRunRepository()
	leftWorkspace := common.WorkspaceID("ws-memory-left")
	rightWorkspace := common.WorkspaceID("ws-memory-right")
	leftCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), leftWorkspace)
	rightCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), rightWorkspace)
	for _, fixture := range []struct {
		ctx         context.Context
		workspaceID common.WorkspaceID
	}{
		{ctx: leftCtx, workspaceID: leftWorkspace},
		{ctx: rightCtx, workspaceID: rightWorkspace},
	} {
		item := newMemoryTwoPhaseRun(fixture.workspaceID, 7)
		if err := runs.Create(fixture.ctx, item); err != nil {
			t.Fatalf("create %s: %v", fixture.workspaceID, err)
		}
	}
	leftFence := prepareMemoryTwoPhaseRetirement(t, runs, leftCtx, leftWorkspace, deletioncontract.ModeRetain)
	left := getMemoryTwoPhaseRun(t, runs, leftCtx, leftWorkspace)
	leftToken := memoryTwoPhaseQuiescenceToken(*leftFence, left)
	if _, err := runs.FinalizeWorkspaceRetirement(leftCtx, leftToken); err != nil {
		t.Fatalf("left finalize: %v", err)
	}
	right := getMemoryTwoPhaseRun(t, runs, rightCtx, rightWorkspace)
	rightUpdate := cloneMemoryTwoPhaseRun(right)
	advanceMemoryTwoPhaseRuntime(rightUpdate)
	if err := runs.CompareAndSwapRuntimeState(rightCtx, rightUpdate, right.Revision, right.RuntimeState.StateRevision); err != nil {
		t.Fatalf("right Workspace CAS was affected by left retirement: %v", err)
	}
}

func TestMemoryRunOwnerBundleRejectsMixedRepositorySets(t *testing.T) {
	first := NewRunOwnerRepositorySet()
	second := NewRunOwnerRepositorySet()
	for name, build := range map[string]func() (repository.RunOwnerRepositories, error){
		"foreign steps": func() (repository.RunOwnerRepositories, error) {
			return repository.NewRunOwnerRepositories(first.Runs(), second.Steps(), first.Checkpoints(), first.WorkspacePurger())
		},
		"foreign checkpoints": func() (repository.RunOwnerRepositories, error) {
			return repository.NewRunOwnerRepositories(first.Runs(), first.Steps(), second.Checkpoints(), first.WorkspacePurger())
		},
		"foreign purger": func() (repository.RunOwnerRepositories, error) {
			return repository.NewRunOwnerRepositories(first.Runs(), first.Steps(), first.Checkpoints(), second.WorkspacePurger())
		},
	} {
		t.Run(name, func(t *testing.T) {
			if mixed, err := build(); !errors.Is(err, common.ErrConflict) || mixed.Present() {
				t.Fatalf("mixed Run-owner bundle = %+v err=%v, want empty conflict", mixed, err)
			}
		})
	}
}

func TestMemoryRunOwnerPurgeCountsOrphansAndNoopRetryCannotHideResidue(t *testing.T) {
	owner := NewRunOwnerRepositorySet()
	runs := owner.Runs()
	steps := owner.Steps()
	checkpoints := owner.Checkpoints()
	workspaceID := common.WorkspaceID("ws-memory-purge-orphans")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	orphan := &runmodel.Run{ID: "run-orphan", WorkspaceID: workspaceID, State: common.RunStateCompleted}
	if err := runs.Create(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	if err := steps.Create(ctx, &runmodel.Step{ID: "step-orphan", RunID: orphan.ID}); err != nil {
		t.Fatal(err)
	}
	if err := checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-orphan", WorkspaceID: workspaceID, RunID: orphan.ID}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Delete(ctx, orphan.ID); err != nil {
		t.Fatalf("delete parent Run while retaining orphan child fixtures: %v", err)
	}
	current := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStateCompleted}
	if err := runs.CreateNextWorkspaceRun(ctx, current); err != nil {
		t.Fatal(err)
	}
	if current.WorkspaceRunGeneration != 2 {
		t.Fatalf("current generation = %d, want legal gap at generation 1", current.WorkspaceRunGeneration)
	}
	if err := steps.Create(ctx, &runmodel.Step{ID: "step-current", RunID: current.ID}); err != nil {
		t.Fatal(err)
	}
	if err := checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-current", WorkspaceID: workspaceID, RunID: current.ID}); err != nil {
		t.Fatal(err)
	}
	fence := prepareMemoryTwoPhaseRetirement(t, runs.(*RunRepository), ctx, workspaceID, deletioncontract.ModePurge)
	persisted, err := runs.GetByWorkspaceID(ctx, workspaceID, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	retired, err := runs.FinalizeWorkspaceRetirement(ctx, memoryNoRuntimeQuiescenceToken(*fence, persisted))
	if err != nil {
		t.Fatalf("finalize retirement with legal generation gap: %v", err)
	}
	receipt, err := owner.WorkspacePurger().PurgeWorkspaceResources(ctx, *retired)
	if err != nil || receipt.ValidateFor(*retired) != nil {
		t.Fatalf("atomic Run-owner purge receipt=%+v err=%v", receipt, err)
	}
	want := []runmodel.WorkspaceRetirementPurgeObservation{
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceRuns, ObservedBefore: 1, DeletedCount: 1},
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceSteps, ObservedBefore: 2, DeletedCount: 2},
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceCheckpoints, ObservedBefore: 2, DeletedCount: 2},
	}
	if fmt.Sprint(receipt.Observations) != fmt.Sprint(want) {
		t.Fatalf("purge observations = %+v, want %+v", receipt.Observations, want)
	}
	for _, runID := range []common.RunID{orphan.ID, current.ID} {
		if items, listErr := steps.ListByRun(ctx, runID); listErr != nil || len(items) != 0 {
			t.Fatalf("post-purge steps run=%s items=%+v err=%v", runID, items, listErr)
		}
		if items, listErr := checkpoints.ListByRun(ctx, runID); listErr != nil || len(items) != 0 {
			t.Fatalf("post-purge checkpoints run=%s items=%+v err=%v", runID, items, listErr)
		}
	}
	if _, err := runs.GetByWorkspaceID(ctx, workspaceID, current.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("post-purge current Run error=%v, want not found", err)
	}
	if replayed, err := runs.GetWorkspaceRetirement(ctx, workspaceID); err != nil || !runmodel.SameWorkspaceRetirementFence(*replayed, *retired) {
		t.Fatalf("post-purge retirement head=%+v err=%v", replayed, err)
	}

	// A response-loss retry must replay the original owner-authored receipt. A
	// newly invented all-zero receipt would conceal the orphan residue that the
	// first transaction actually observed and deleted.
	retry, err := owner.WorkspacePurger().PurgeWorkspaceResources(ctx, *retired)
	if err != nil || retry.ValidateFor(*retired) != nil {
		t.Fatalf("response-loss retry receipt=%+v err=%v", retry, err)
	}
	if retry.ProofHash != receipt.ProofHash || fmt.Sprint(retry.Observations) != fmt.Sprint(receipt.Observations) || !runmodel.SameWorkspaceRetirementFence(retry.Fence, receipt.Fence) {
		t.Fatalf("response-loss retry invented a new receipt: first=%+v retry=%+v", receipt, retry)
	}
	corrupt := receipt
	corrupt.Observations = append([]runmodel.WorkspaceRetirementPurgeObservation(nil), receipt.Observations...)
	corrupt.Observations[1].RemainingCount = 1
	if err := corrupt.ValidateFor(*retired); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("corrupt non-empty receipt error=%v, want conflict", err)
	}
	corrupt = receipt
	corrupt.ProofHash += ":forged"
	if err := corrupt.ValidateFor(*retired); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged proof hash error=%v, want conflict", err)
	}
}

func newMemoryTwoPhaseRetirementFixture(t *testing.T, suffix string) (*RunRepository, context.Context, common.WorkspaceID) {
	t.Helper()
	runs := NewRunRepository()
	workspaceID := common.WorkspaceID("ws-memory-two-phase-" + suffix)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	item := newMemoryTwoPhaseRun(workspaceID, 7)
	if err := runs.Create(ctx, item); err != nil {
		t.Fatalf("create Run: %v", err)
	}
	return runs, ctx, workspaceID
}

func prepareMemoryTwoPhaseRetirement(t *testing.T, runs *RunRepository, ctx context.Context, workspaceID common.WorkspaceID, mode deletioncontract.Mode) *runmodel.WorkspaceRetirementFence {
	t.Helper()
	intent := runmodel.WorkspaceRetirementIntent{
		WorkspaceID: workspaceID, DeletionID: "deletion-" + string(workspaceID), DeletionGeneration: 1,
		DataDispositionIntent: mode, WorkspaceIntentHash: "workspace-intent-hash:" + string(workspaceID),
	}
	fence, err := runs.PrepareWorkspaceRetirement(ctx, intent)
	if err != nil {
		t.Fatalf("prepare retirement: %v", err)
	}
	return fence
}

func getMemoryTwoPhaseRun(t *testing.T, runs *RunRepository, ctx context.Context, workspaceID common.WorkspaceID) *runmodel.Run {
	t.Helper()
	item, err := runs.GetByWorkspaceID(ctx, workspaceID, "run-001")
	if err != nil {
		t.Fatalf("get Run: %v", err)
	}
	return item
}

func cloneMemoryTwoPhaseRun(item *runmodel.Run) *runmodel.Run {
	return runmodel.CloneRun(item)
}

func newMemoryTwoPhaseRun(workspaceID common.WorkspaceID, runtimeRevision uint64) *runmodel.Run {
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

func advanceMemoryTwoPhaseRuntime(item *runmodel.Run) {
	item.RuntimeState.StateRevision++
	item.RuntimeLifecycleFence = &runtimecontract.RuntimeOpaqueLifecycleFence{
		WorkspaceID: item.WorkspaceID, RunID: item.ID, RuntimeStateRevision: item.RuntimeState.StateRevision,
		RuntimeStateRef:  fmt.Sprintf("runtime-state/%s/%s/%d", item.WorkspaceID, item.ID, item.RuntimeState.StateRevision),
		RuntimeStateHash: runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState), BootstrapHash: "bootstrap-hash:current",
		TerminalFenceID: item.TerminalDelivery.TerminalDeliveryID,
	}
}

func memoryTwoPhaseQuiescenceToken(fence runmodel.WorkspaceRetirementFence, item *runmodel.Run) deletioncontract.WorkspaceRuntimeQuiescenceToken {
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

func memoryNoRuntimeQuiescenceToken(fence runmodel.WorkspaceRetirementFence, item *runmodel.Run) deletioncontract.WorkspaceRuntimeQuiescenceToken {
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
