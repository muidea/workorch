package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := runRepo.Update(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("block run: %v", err)
	}

	staleProgress := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
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
	if runRepo.getCalls.Load() != 1 {
		t.Fatalf("global Get calls = %d, want only final assertion read", runRepo.getCalls.Load())
	}
	if runRepo.getByWorkspaceIDCalls.Load() != 1 {
		t.Fatalf("GetByWorkspaceID calls = %d, want scoped owner read", runRepo.getByWorkspaceIDCalls.Load())
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
		State:       common.WorkspaceStateRunning,
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
		State:       common.WorkspaceStateRunning,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
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

func TestRunStateSubsystemRejectsIncompatibleActiveWorkspaceState(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	subsystem := newRunStateSubsystem(runRepo, workspaceRepo)
	workspaceID := common.WorkspaceID("ws-incompatible-active-state")
	runID := common.RunID("run-incompatible-active-state")
	ticketID := common.TicketID("ticket-incompatible-active-state")

	run := &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}
	workspace := &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateWaitingDecision,
		ActiveRunID: runID,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if _, err := subsystem.syncRunAndWorkspaceForActiveState(ctx, run, common.WorkspaceStateRunning, workspace); !errors.Is(err, common.ErrConflict) {
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
	if gotRun.State != common.RunStateBlockedOnDecision || gotRun.BlockingTicketID != ticketID {
		t.Fatalf("run = %+v, want unchanged blocked decision", gotRun)
	}
	if gotWorkspace.State != common.WorkspaceStateWaitingDecision || gotWorkspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want unchanged waiting decision", gotWorkspace)
	}
}

func TestWorkspaceStateCompatibleWithRun(t *testing.T) {
	cases := []struct {
		name           string
		runState       common.RunState
		workspaceState common.WorkspaceState
		want           bool
	}{
		{name: "created starts running", runState: common.RunStateCreated, workspaceState: common.WorkspaceStateRunning, want: true},
		{name: "running", runState: common.RunStateRunning, workspaceState: common.WorkspaceStateRunning, want: true},
		{name: "validating", runState: common.RunStateValidating, workspaceState: common.WorkspaceStateValidating, want: true},
		{name: "decision", runState: common.RunStateBlockedOnDecision, workspaceState: common.WorkspaceStateWaitingDecision, want: true},
		{name: "approval", runState: common.RunStateBlockedOnApproval, workspaceState: common.WorkspaceStateBlocked, want: true},
		{name: "decision cannot run workspace", runState: common.RunStateBlockedOnDecision, workspaceState: common.WorkspaceStateRunning, want: false},
		{name: "approval cannot run workspace", runState: common.RunStateBlockedOnApproval, workspaceState: common.WorkspaceStateRunning, want: false},
		{name: "terminal not active", runState: common.RunStateCompleted, workspaceState: common.WorkspaceStateRunning, want: false},
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
