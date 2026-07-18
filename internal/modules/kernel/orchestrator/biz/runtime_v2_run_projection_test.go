package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeV2ProjectionStateRepository struct {
	aggregate runtimev2.RuntimeAggregate
}

func (r *runtimeV2ProjectionStateRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtimev2.RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *runtimeV2ProjectionStateRepository) Save(_ context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if r.aggregate.StateRevision != expectedRevision {
		return fmt.Errorf("%w: revision changed", common.ErrConflict)
	}
	r.aggregate = aggregate
	return nil
}

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

func TestRuntimeV2BlockedDecisionRequiresCommittedAggregateResolution(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2-gate", State: common.WorkspaceStateActive, ActiveRunID: "run-v2-gate"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID:                     "run-v2-gate",
		WorkspaceID:            "ws-v2-gate",
		State:                  common.RunStateBlockedOnDecision,
		BlockingTicketID:       "ticket-v2-gate",
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2,
	}); err != nil {
		t.Fatal(err)
	}
	state := &runtimeV2ProjectionStateRepository{aggregate: runtimev2.RuntimeAggregate{
		StateRevision: 1,
		Facts:         runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2-gate", RunID: "run-v2-gate"},
		GovernanceRequests: []runtimecontract.RuntimeGovernanceRequestRecord{{
			TicketID: "ticket-v2-gate", AuthoritySnapshotID: "snapshot-v2-gate", DispatchID: "dispatch-v2-gate",
			DependencyKind: "configuration", SubjectRef: "provider/ai-proxy",
		}},
	}}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	if err := orchestrator.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatalf("configure runtime state repository: %v", err)
	}
	if err := orchestrator.CanResumeRun(ctx, "run-v2-gate"); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("CanResumeRun without aggregate resolution err = %v, want blocked", err)
	}
	if err := orchestrator.ActivateRuntimeV2Run(ctx, "ws-v2-gate", "run-v2-gate"); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("ActivateRuntimeV2Run without aggregate resolution err = %v, want blocked", err)
	}
	state.aggregate.GovernanceResolutions = []runtimecontract.RuntimeGovernanceResolutionRecord{{
		ResolutionID: "resolution-v2-gate", TicketID: "ticket-v2-gate", AuthoritySnapshotID: "snapshot-v2-gate",
		DispatchID: "dispatch-v2-gate", Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue,
	}}
	if err := orchestrator.CanResumeRun(ctx, "run-v2-gate"); err != nil {
		t.Fatalf("CanResumeRun with committed aggregate resolution: %v", err)
	}
	if err := orchestrator.ActivateRuntimeV2Run(ctx, "ws-v2-gate", "run-v2-gate"); err != nil {
		t.Fatalf("ActivateRuntimeV2Run with committed aggregate resolution: %v", err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2-gate", "run-v2-gate")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateActive || run.BlockingTicketID != "" {
		t.Fatalf("activated run = %+v", run)
	}
}

func TestProjectRuntimeV2CompleteBypassesLegacyResultGate(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "全部 coverage focus 已满足"}
	if err := runs.Create(ctx, terminalRuntimeV2Run(t, "run-v2", runtimecontract.RuntimeActionComplete, terminal)); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
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
	terminal := &runtimecontract.TerminalOutcome{Code: "no_progress_exhausted", Summary: "patch input remains invalid"}
	if err := runs.Create(ctx, terminalRuntimeV2Run(t, "run-v2", runtimecontract.RuntimeActionFail, terminal)); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
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

func TestFailRunRejectsActiveRuntimeV2AggregateBypass(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	state := runtimecontract.RuntimeStateRecord{StateRevision: 1}
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeState: &state,
	}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	if err := orchestrator.FailRun(ctx, "run-v2", "adapter failed"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("FailRun active Runtime V2 err = %v, want conflict", err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateActive || run.FailureReason != "" {
		t.Fatalf("FailRun bypass changed active Runtime V2 run: %+v", run)
	}
}

func TestProjectRuntimeV2TerminalRejectsUnsealedProjection(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive, RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	terminal := &runtimecontract.TerminalOutcome{Code: "runtime_v2_drive_failed", Summary: "adapter failed"}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionFail, terminal); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unsealed terminal projection err = %v, want conflict", err)
	}
}

func TestFailRunAllowsRuntimeV2ActivationFailureBeforeAggregateExists(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStatePending,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2,
	}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	if err := orchestrator.FailRun(ctx, "run-v2", "activation failed"); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateFailed || run.FailureReason != "activation failed" {
		t.Fatalf("activation failure run = %+v", run)
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
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "done"}
	if err := baseRuns.Create(ctx, terminalRuntimeV2Run(t, "run-v2", runtimecontract.RuntimeActionComplete, terminal)); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
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
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "old run done"}
	if err := runs.Create(ctx, terminalRuntimeV2Run(t, "run-old", runtimecontract.RuntimeActionComplete, terminal)); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-live", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
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
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "done"}
	if err := runs.Create(ctx, terminalRuntimeV2Run(t, "run-new", runtimecontract.RuntimeActionComplete, terminal)); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, workspaces, nil, nil)
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

func terminalRuntimeV2Run(t *testing.T, runID common.RunID, action runtimecontract.RuntimeAction, terminal *runtimecontract.TerminalOutcome) *runmodel.Run {
	t.Helper()
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-v2", RunID: runID, DefinitionRevision: "definition-v2", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus: runtimecontract.CoverageFocus{ID: "focus-v2"},
	}
	stateValue := runtimecontract.ConvergenceStateComplete
	if action == runtimecontract.RuntimeActionFail {
		stateValue = runtimecontract.ConvergenceStateFailed
	}
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: stateValue, Action: action, Focus: facts.Focus, Terminal: terminal,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimecontract.RuntimeStateRecord{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent, StateRevision: 1,
		Definition: runtimecontract.RuntimeDefinitionContext{
			Revision: "definition-v2", GoalID: "goal-v2", GoalDefinition: "goal", DoDID: "dod-v2", DoDDefinition: "dod",
			CapabilityContract: []string{"file/write"},
		},
		Facts: facts.Normalized(), Decision: decision,
	}
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	return &runmodel.Run{
		ID: runID, WorkspaceID: "ws-v2", State: common.RunStateActive,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeState: &state,
	}
}
