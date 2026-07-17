package orchestrator

import (
	"context"
	"errors"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestActivateRuntimeV2RunOnlyProjectsOwnerState(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	steps := memory.NewStepRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-v2",
	}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	if err := orchestrator.ActivateRuntimeV2Run(ctx, "ws-v2", "run-v2"); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateActive || run.BlockingTicketID != "" {
		t.Fatalf("activated run = %+v", run)
	}
	items, err := steps.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 0 {
		t.Fatalf("Runtime V2 activation created legacy steps: %+v err=%v", items, err)
	}
}

func TestProjectRuntimeV2CompleteBypassesLegacyResultGate(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "全部 coverage focus 已满足"}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatalf("terminal replay: %v", err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateCompleted || run.EndedAt == "" || workspace.ActiveRunID != "" {
		t.Fatalf("run=%+v workspace=%+v", run, workspace)
	}
}

func TestProjectRuntimeV2FailurePreservesTerminalCodeForObservation(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	terminal := &runtimecontract.TerminalOutcome{Code: "no_progress_exhausted", Summary: "patch input remains invalid"}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionFail, terminal); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateFailed || run.FailureReason != "no_progress_exhausted: patch input remains invalid" {
		t.Fatalf("failed run = %+v", run)
	}
}

func TestActivateRuntimeV2RunRepairsStaleWorkspaceProjection(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := NewRuntimeOrchestrator(runs, workspaces, nil, nil).ActivateRuntimeV2Run(ctx, "ws-v2", "run-v2"); err != nil {
		t.Fatal(err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	if workspace.State != common.WorkspaceStateActive || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("repaired workspace = %+v", workspace)
	}
}

func TestProjectRuntimeV2TerminalConvergesAfterRunUpdateFailure(t *testing.T) {
	ctx := context.Background()
	baseRuns := memory.NewRunRepository()
	runUpdateErr := errors.New("run update unavailable")
	runs := &failRunUpdateRepository{RunRepository: baseRuns, failures: 1, err: runUpdateErr}
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := baseRuns.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "done"}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionComplete, terminal); !errors.Is(err, runUpdateErr) {
		t.Fatalf("first terminal projection err = %v", err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	run, err := baseRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ActiveRunID != "" || run.State != common.RunStateActive {
		t.Fatalf("partial projection workspace=%+v run=%+v", workspace, run)
	}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatalf("retry terminal projection: %v", err)
	}
	run, err = baseRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateCompleted || run.EndedAt == "" {
		t.Fatalf("converged run = %+v", run)
	}
}

type failRunUpdateRepository struct {
	repository.RunRepository
	failures int
	err      error
}

func (r *failRunUpdateRepository) Update(ctx context.Context, run *runmodel.Run) error {
	if r.failures > 0 {
		r.failures--
		return r.err
	}
	return r.RunRepository.Update(ctx, run)
}

// RVC-14: terminal projection for run R must not steal ActiveRun owned by a different live run,
// and must not rewrite already-completed Run authority on retry.
func TestProjectRuntimeV2TerminalDoesNotStealLiveActiveRun(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{
		ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-live",
	}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-old", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-live", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "old run done"}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-old", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatalf("terminal projection for superseded slot: %v", err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ActiveRunID != "run-live" {
		t.Fatalf("ActiveRunID = %q, want live run preserved", workspace.ActiveRunID)
	}
	old, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-old")
	if err != nil {
		t.Fatal(err)
	}
	if old.State != common.RunStateCompleted {
		t.Fatalf("old run state = %s, want completed", old.State)
	}
	// Retry must remain idempotent without conflict or rewrite.
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-old", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatalf("terminal retry: %v", err)
	}
	workspace, err = workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ActiveRunID != "run-live" {
		t.Fatalf("retry stole ActiveRun: %q", workspace.ActiveRunID)
	}
}

func TestProjectRuntimeV2TerminalClearsStaleTerminalActiveRunSlot(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{
		ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-stale",
	}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-stale", WorkspaceID: "ws-v2", State: common.RunStateFailed}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-new", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "done"}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-new", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatal(err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ActiveRunID != "" {
		t.Fatalf("ActiveRunID = %q after terminal, want cleared", workspace.ActiveRunID)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-new")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateCompleted {
		t.Fatalf("run state = %s", run.State)
	}
}
