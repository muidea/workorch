package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type runStateCountingRunRepository struct {
	inner                 *memory.RunRepository
	getCalls              atomic.Int32
	getByWorkspaceIDCalls atomic.Int32
}

func newRunStateCountingRunRepository() *runStateCountingRunRepository {
	return &runStateCountingRunRepository{inner: memory.NewRunRepository()}
}

func (r *runStateCountingRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Create(ctx, item)
}

func (r *runStateCountingRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	r.getCalls.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *runStateCountingRunRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	r.getByWorkspaceIDCalls.Add(1)
	return r.inner.GetByWorkspaceID(ctx, workspaceID, id)
}

func (r *runStateCountingRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.inner.List(ctx)
}

func (r *runStateCountingRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *runStateCountingRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Update(ctx, item)
}

func (r *runStateCountingRunRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.inner.Delete(ctx, id)
}

func TestRuntimeOrchestratorPersistRunProgressUpdatePreservesBlockedDecisionLifecycle(t *testing.T) {
	ctx := context.Background()
	runRepo := newRunStateCountingRunRepository()
	orch := &RuntimeOrchestrator{runs: runRepo}
	runID := common.RunID("run-progress-preserve-blocked-decision")
	workspaceID := common.WorkspaceID("ws-progress-preserve-blocked-decision")
	ticketID := common.TicketID("ticket-progress-preserve-blocked-decision")

	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := runRepo.Update(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		ExecutionContext: map[string]any{
			"ownerContinuation": map[string]any{"kind": "decision"},
		},
	}); err != nil {
		t.Fatalf("block run: %v", err)
	}

	staleProgress := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			"coverageProgress": "updated",
		},
	}
	if err := orch.persistRunProgressUpdate(ctx, staleProgress); err != nil {
		t.Fatalf("persist progress: %v", err)
	}

	got, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.State != common.RunStateBlockedOnDecision || got.BlockingTicketID != ticketID {
		t.Fatalf("run lifecycle = state:%s ticket:%s, want blocked on %s", got.State, got.BlockingTicketID, ticketID)
	}
	if got.ExecutionContext["coverageProgress"] != "updated" {
		t.Fatalf("execution context = %+v, want progress update preserved", got.ExecutionContext)
	}
	if owner := mapFromValue(got.ExecutionContext["ownerContinuation"]); stringFromMap(owner, "kind") != "decision" {
		t.Fatalf("execution context = %+v, want current owner continuation preserved", got.ExecutionContext)
	}
	if runRepo.getCalls.Load() != 1 {
		t.Fatalf("global Get calls = %d, want only final assertion read", runRepo.getCalls.Load())
	}
	if runRepo.getByWorkspaceIDCalls.Load() != 1 {
		t.Fatalf("GetByWorkspaceID calls = %d, want scoped owner read", runRepo.getByWorkspaceIDCalls.Load())
	}
}

func TestRuntimeOrchestratorPersistRunProgressUpdateAppliesManagedProgressDeletion(t *testing.T) {
	ctx := context.Background()
	runRepo := newRunStateCountingRunRepository()
	orch := &RuntimeOrchestrator{runs: runRepo}
	runID := common.RunID("run-progress-delete-managed")
	workspaceID := common.WorkspaceID("ws-progress-delete-managed")

	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			gapBaselineKey:      map[string]any{"schemaVersion": "old"},
			"ownerContinuation": map[string]any{"kind": "dispatch"},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	progress := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			"coverageProgress": "updated",
		},
	}
	if err := orch.persistRunProgressUpdate(ctx, progress); err != nil {
		t.Fatalf("persist progress: %v", err)
	}

	got, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if _, ok := got.ExecutionContext[gapBaselineKey]; ok {
		t.Fatalf("execution context = %+v, want managed gap baseline deletion applied", got.ExecutionContext)
	}
	if got.ExecutionContext["coverageProgress"] != "updated" {
		t.Fatalf("execution context = %+v, want progress update preserved", got.ExecutionContext)
	}
	if owner := mapFromValue(got.ExecutionContext["ownerContinuation"]); stringFromMap(owner, "kind") != "dispatch" {
		t.Fatalf("execution context = %+v, want unmanaged owner continuation preserved", got.ExecutionContext)
	}
}

func TestCanCloseRunAsUnsuccessfulTerminalDoesNotOwnCompletion(t *testing.T) {
	for _, target := range []common.RunState{
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded,
	} {
		if !canCloseRunAsUnsuccessfulTerminal(common.RunStateActive, target) {
			t.Fatalf("canCloseRunAsUnsuccessfulTerminal(running, %s) = false, want true", target)
		}
	}
	if canCloseRunAsUnsuccessfulTerminal(common.RunStateActive, common.RunStateCompleted) {
		t.Fatal("canCloseRunAsUnsuccessfulTerminal(running, completed) = true, want completion to stay on CompleteRun path")
	}
}

func TestRunStateSubsystemReconcileWorkspaceActiveRunClearsMissingActiveRun(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	subsystem := newRunStateSubsystem(runRepo, workspaceRepo)
	workspaceID := common.WorkspaceID("ws-missing-active-run")

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: "run-missing-active",
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	ws, activeRunID, changed, err := subsystem.reconcileWorkspaceActiveRun(ctx, workspaceID)
	if err != nil {
		t.Fatalf("reconcile workspace active run: %v", err)
	}
	if !changed || activeRunID != "" {
		t.Fatalf("changed=%v activeRunID=%q, want changed with empty event run id", changed, activeRunID)
	}
	if ws.ActiveRunID != "" || ws.State != common.WorkspaceStateActive {
		t.Fatalf("workspace = %+v, want active with no active run", ws)
	}
	persisted, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if persisted.ActiveRunID != "" || persisted.State != common.WorkspaceStateActive {
		t.Fatalf("persisted workspace = %+v, want active with no active run", persisted)
	}
}

func TestRunStateSubsystemReconcileWorkspaceActiveRunUsesScopedGetter(t *testing.T) {
	ctx := context.Background()
	runRepo := newRunStateCountingRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	subsystem := newRunStateSubsystem(runRepo, workspaceRepo)
	workspaceID := common.WorkspaceID("ws-active-run-scoped-read")
	runID := common.RunID("run-active")

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	ws, activeRunID, changed, err := subsystem.reconcileWorkspaceActiveRun(ctx, workspaceID)
	if err != nil {
		t.Fatalf("reconcile workspace active run: %v", err)
	}
	if changed || activeRunID != runID || ws.ActiveRunID != runID {
		t.Fatalf("changed=%v activeRunID=%q workspace=%+v, want unchanged active run", changed, activeRunID, ws)
	}
	if runRepo.getCalls.Load() != 0 {
		t.Fatalf("global Get calls = %d, want 0", runRepo.getCalls.Load())
	}
	if runRepo.getByWorkspaceIDCalls.Load() != 1 {
		t.Fatalf("GetByWorkspaceID calls = %d, want 1", runRepo.getByWorkspaceIDCalls.Load())
	}
}

func TestRunStateSubsystemRejectsActiveWorkspaceStateForApprovalBlock(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	subsystem := newRunStateSubsystem(runRepo, workspaceRepo)
	workspaceID := common.WorkspaceID("ws-incompatible-active-state")
	runID := common.RunID("run-incompatible-active-state")
	approvalID := common.ApprovalID("approval-incompatible-active-state")

	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: approvalID,
	}
	workspace := &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if _, err := subsystem.syncRunAndWorkspaceForActiveState(ctx, run, common.WorkspaceStateActive, workspace); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("sync incompatible state error = %v, want conflict", err)
	}
	gotRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	gotWorkspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if gotRun.State != common.RunStateBlockedOnApproval || gotRun.BlockingApprovalID != approvalID {
		t.Fatalf("run = %+v, want unchanged blocked approval", gotRun)
	}
	if gotWorkspace.State != common.WorkspaceStateActive || gotWorkspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want unchanged active workspace", gotWorkspace)
	}
}

func TestWorkspaceStateCompatibleWithRun(t *testing.T) {
	cases := []struct {
		name           string
		runState       common.RunState
		workspaceState common.WorkspaceState
		want           bool
	}{
		{name: "created starts running", runState: common.RunStatePending, workspaceState: common.WorkspaceStateActive, want: true},
		{name: "running", runState: common.RunStateActive, workspaceState: common.WorkspaceStateActive, want: true},
		{name: "validating", runState: common.RunStateActive, workspaceState: common.WorkspaceStateActive, want: true},
		{name: "decision", runState: common.RunStateBlockedOnDecision, workspaceState: common.WorkspaceStateActive, want: true},
		{name: "approval", runState: common.RunStateBlockedOnApproval, workspaceState: common.WorkspaceStateBlockedOnApproval, want: true},
		{name: "approval cannot run workspace", runState: common.RunStateBlockedOnApproval, workspaceState: common.WorkspaceStateActive, want: false},
		{name: "terminal not active", runState: common.RunStateCompleted, workspaceState: common.WorkspaceStateActive, want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			run := &runmodel.Run{ID: "run", WorkspaceID: "ws", State: tt.runState}
			if got := workspaceStateCompatibleWithRun(run, tt.workspaceState); got != tt.want {
				t.Fatalf("compatible = %v, want %v", got, tt.want)
			}
		})
	}
}
