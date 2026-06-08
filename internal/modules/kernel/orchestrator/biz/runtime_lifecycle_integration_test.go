package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type listFailingStepRepository struct {
	inner *memory.StepRepository
	err   error
}

func (r *listFailingStepRepository) Create(ctx context.Context, item *runmodel.Step) error {
	return r.inner.Create(ctx, item)
}

func (r *listFailingStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	return r.inner.Get(ctx, id)
}

func (r *listFailingStepRepository) ListByRun(context.Context, common.RunID) ([]*runmodel.Step, error) {
	return nil, r.err
}

func (r *listFailingStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	return r.inner.Update(ctx, item)
}

func (r *listFailingStepRepository) Delete(ctx context.Context, id common.StepID) error {
	return r.inner.Delete(ctx, id)
}

func TestRuntimeOrchestrator_RuntimeFailurePublishesWorkspaceReleaseEvent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime exploded")}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-fail-evt")
	runID := common.RunID("run-fail-evt")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	published := hub.Published()
	if len(published) < 5 {
		t.Fatalf("published events = %d, want at least 5", len(published))
	}
	last := published[len(published)-1]
	if last.Topic != workspaceevents.TopicWorkspaceStateChanged || last.Payload["activeRunId"] != "" {
		t.Fatalf("last event = %#v, want workspace release event", last)
	}
}

func TestRuntimeOrchestrator_TerminalRunDismissesOpenDecisionBlocker(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		close func(context.Context, *RuntimeOrchestrator, common.RunID) error
		want  common.RunState
	}{
		{
			name: "failed",
			close: func(ctx context.Context, orch *RuntimeOrchestrator, runID common.RunID) error {
				return orch.FailRun(ctx, runID, "manual failure")
			},
			want: common.RunStateFailed,
		},
		{
			name: "superseded",
			close: func(ctx context.Context, orch *RuntimeOrchestrator, runID common.RunID) error {
				return orch.SupersedeRun(ctx, runID)
			},
			want: common.RunStateSuperseded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspaceRepo := memory.NewWorkspaceRepository()
			runRepo := memory.NewRunRepository()
			stepRepo := memory.NewStepRepository()
			ticketRepo := memory.NewDecisionTicketRepository()
			approvalRepo := memory.NewApprovalRepository()
			orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"ok": true}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

			workspaceID := common.WorkspaceID("ws-terminal-dismiss-" + tc.name)
			runID := common.RunID("run-terminal-dismiss-" + tc.name)
			ticketID := common.TicketID("ticket-terminal-dismiss-" + tc.name)
			if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
				t.Fatalf("seed workspace: %v", err)
			}
			if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen}); err != nil {
				t.Fatalf("seed ticket: %v", err)
			}

			if err := tc.close(ctx, orch, runID); err != nil {
				t.Fatalf("close run: %v", err)
			}

			run, err := runRepo.Get(ctx, runID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if run.State != tc.want || run.BlockingTicketID != "" {
				t.Fatalf("run = %+v, want %s without blocker", run, tc.want)
			}
			ticket, err := ticketRepo.Get(ctx, ticketID)
			if err != nil {
				t.Fatalf("get ticket: %v", err)
			}
			if ticket.State != common.DecisionTicketStateDismissed || ticket.UpdatedAt == "" {
				t.Fatalf("ticket = %+v, want dismissed with updatedAt", ticket)
			}
		})
	}
}

func TestRuntimeOrchestrator_StartRunRejectsDifferentActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-active-conflict")
	runID := common.RunID("run-active-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("other-run"),
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("start run err = %v, want conflict", err)
	}
}

func TestRuntimeOrchestrator_StartRunAllowsStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-stale-terminal-active")
	staleRunID := common.RunID("run-stale-terminal-active")
	runID := common.RunID("run-start-after-stale-terminal")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateFailed,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateSuperseded}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run with stale terminal active run: %v", err)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID == staleRunID {
		t.Fatalf("workspace active run id = %q, want stale terminal run replaced", ws.ActiveRunID)
	}
	if ws.State != common.WorkspaceStateCompleted || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed with no stale active run", ws)
	}
}

func TestRuntimeOrchestrator_StartRunFailsRunWhenInitialDispatchSpecBuildFails(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	listErr := errors.New("step list unavailable")
	stepRepo := &listFailingStepRepository{inner: memory.NewStepRepository(), err: listErr}
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-start-initial-spec-fails")
	runID := common.RunID("run-start-initial-spec-fails")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateFailed || !strings.Contains(r.FailureReason, listErr.Error()) {
		t.Fatalf("run = %+v, want failed with initial dispatch error", r)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateFailed || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want failed with active run released", ws)
	}
}

func TestRuntimeOrchestrator_ResumeRunningRunWithNoStepsDispatchesInitialStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-running-empty-resume")
	runID := common.RunID("run-running-empty-resume")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get initial step: %v", err)
	}
	if step.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", step.State)
	}
}

func TestRuntimeOrchestrator_ReconcileWorkspaceActiveRunClearsTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-terminal-active-run")
	runID := common.RunID("run-terminal-active-run")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateSuperseded}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.reconcileWorkspaceActiveRun(ctx, workspaceID); err != nil {
		t.Fatalf("reconcile workspace active run: %v", err)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateFailed || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want failed with no terminal active run", ws)
	}
}

func TestRuntimeOrchestrator_StartRunRejectsNonCreatedState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-start-state-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name  string
		state common.RunState
	}{
		{name: "running", state: common.RunStateRunning},
		{name: "blocked decision", state: common.RunStateBlockedOnDecision},
		{name: "blocked approval", state: common.RunStateBlockedOnApproval},
		{name: "completed", state: common.RunStateCompleted},
		{name: "failed", state: common.RunStateFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runID := common.RunID("run-start-state-conflict-" + tc.name)
			if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: tc.state}); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.StartRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("start run err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_StartRun_RuntimeFailureMarksRunAndStepFailed(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime exploded")}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close failure state in-repo, got err: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}

	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed {
		t.Fatalf("step state = %q, want failed", step.State)
	}
	if step.Output["reason"] != "runtime exploded" {
		t.Fatalf("step output reason = %v, want runtime exploded", step.Output["reason"])
	}
}

func TestRuntimeOrchestrator_DispatchNextStep_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()

	orch := NewRuntimeOrchestrator(nil, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)
	if err := orch.DispatchNextStep(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty runID err = %v, want invalid argument", err)
	}

	runID := common.RunID("run-no-runtime")
	workspaceID := common.WorkspaceID("ws-no-runtime")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := orch.DispatchNextStep(ctx, runID); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil runtime err = %v, want invalid argument", err)
	}
}

func TestRuntimeOrchestrator_BlockCommands_ValidateArgumentsAndSetExpectedRefs(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	if err := orch.BlockOnDecision(ctx, "", common.TicketID("ticket-1")); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision empty run err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnDecision(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision empty ticket err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnApproval(ctx, "", common.ApprovalID("approval-1")); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval empty run err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnApproval(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval empty approval err = %v, want invalid argument", err)
	}

	decisionWorkspaceID := common.WorkspaceID("ws-block-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: decisionWorkspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	decisionRunID := common.RunID("run-block-decision")
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          decisionRunID,
		WorkspaceID: decisionWorkspaceID,
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("seed decision run: %v", err)
	}
	if err := orch.BlockOnDecision(ctx, decisionRunID, common.TicketID("ticket-1")); err != nil {
		t.Fatalf("block on decision: %v", err)
	}
	blockedDecision, err := runRepo.Get(ctx, decisionRunID)
	if err != nil {
		t.Fatalf("get decision-blocked run: %v", err)
	}
	if blockedDecision.BlockingTicketID != common.TicketID("ticket-1") || blockedDecision.BlockingApprovalID != "" {
		t.Fatalf("unexpected decision block refs: %+v", blockedDecision)
	}
	decisionWorkspace, err := workspaceRepo.Get(ctx, decisionWorkspaceID)
	if err != nil {
		t.Fatalf("get decision-blocked workspace: %v", err)
	}
	if decisionWorkspace.State != common.WorkspaceStateWaitingDecision || decisionWorkspace.ActiveRunID != decisionRunID {
		t.Fatalf("decision workspace = %+v, want waiting_decision with active run", decisionWorkspace)
	}

	approvalWorkspaceID := common.WorkspaceID("ws-block-approval")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: approvalWorkspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed approval workspace: %v", err)
	}
	approvalRunID := common.RunID("run-block-approval")
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          approvalRunID,
		WorkspaceID: approvalWorkspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed approval run: %v", err)
	}
	if err := orch.BlockOnApproval(ctx, approvalRunID, common.ApprovalID("approval-1")); err != nil {
		t.Fatalf("block on approval: %v", err)
	}
	blockedApproval, err := runRepo.Get(ctx, approvalRunID)
	if err != nil {
		t.Fatalf("get approval-blocked run: %v", err)
	}
	if blockedApproval.BlockingApprovalID != common.ApprovalID("approval-1") || blockedApproval.BlockingTicketID != "" {
		t.Fatalf("unexpected approval block refs: %+v", blockedApproval)
	}
	approvalWorkspace, err := workspaceRepo.Get(ctx, approvalWorkspaceID)
	if err != nil {
		t.Fatalf("get approval-blocked workspace: %v", err)
	}
	if approvalWorkspace.State != common.WorkspaceStateBlocked || approvalWorkspace.ActiveRunID != approvalRunID {
		t.Fatalf("approval workspace = %+v, want blocked with active run", approvalWorkspace)
	}
}

func TestRuntimeOrchestrator_BlockCommandsRejectBlockedOrTerminalRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-block-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{
			name: "already blocked on decision",
			run: &runmodel.Run{
				ID:               common.RunID("run-block-conflict-decision"),
				WorkspaceID:      workspaceID,
				State:            common.RunStateBlockedOnDecision,
				BlockingTicketID: common.TicketID("ticket-existing"),
			},
		},
		{
			name: "already blocked on approval",
			run: &runmodel.Run{
				ID:                 common.RunID("run-block-conflict-approval"),
				WorkspaceID:        workspaceID,
				State:              common.RunStateBlockedOnApproval,
				BlockingApprovalID: common.ApprovalID("approval-existing"),
			},
		},
		{
			name: "completed run",
			run: &runmodel.Run{
				ID:          common.RunID("run-block-conflict-completed"),
				WorkspaceID: workspaceID,
				State:       common.RunStateCompleted,
			},
		},
		{
			name: "failed run",
			run: &runmodel.Run{
				ID:          common.RunID("run-block-conflict-failed"),
				WorkspaceID: workspaceID,
				State:       common.RunStateFailed,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.BlockOnDecision(ctx, tc.run.ID, common.TicketID("ticket-next")); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("block on decision err = %v, want conflict", err)
			}
			if err := orch.BlockOnApproval(ctx, tc.run.ID, common.ApprovalID("approval-next")); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("block on approval err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_ValidateRunProjectsWorkspaceState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-validating")
	runID := common.RunID("run-validating")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.ValidateRun(ctx, runID); err != nil {
		t.Fatalf("validate run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateValidating || ws.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want validating with active run", ws)
	}
	published := hub.Published()
	if len(published) < 2 {
		t.Fatalf("published events = %d, want run and workspace state events", len(published))
	}
	last := published[len(published)-1]
	if last.Topic != workspaceevents.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyState] != string(common.WorkspaceStateValidating) {
		t.Fatalf("last event = %#v, want workspace validating event", last)
	}
}

func TestRuntimeOrchestrator_BlockRunCanReplaceStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-stale-terminal-active")
	staleRunID := common.RunID("run-stale-terminal-active")
	currentRunID := common.RunID("run-current-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: staleRunID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: currentRunID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed current run: %v", err)
	}

	if err := orch.BlockOnApproval(ctx, currentRunID, common.ApprovalID("approval-current-block")); err != nil {
		t.Fatalf("block on approval with stale terminal active run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateBlocked || ws.ActiveRunID != currentRunID {
		t.Fatalf("workspace = %+v, want current blocked run projection", ws)
	}
}

func TestRuntimeOrchestrator_ResumeRunRequiresResolvedTicket(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	ticketID := common.TicketID("ticket-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID})
	_ = ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen})

	if err := orch.ResumeRun(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("resume blocked by open ticket err = %v, want blocked", err)
	}

	ticket, _ := ticketRepo.Get(ctx, ticketID)
	ticket.State = common.DecisionTicketStateResolved
	_ = ticketRepo.Update(ctx, ticket)
	_ = resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-1"),
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	})

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after ticket resolved: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
	if r.BlockingTicketID != "" {
		t.Fatalf("blocking ticket = %q, want empty", r.BlockingTicketID)
	}
}

func TestRuntimeOrchestrator_ResumeProviderConfigRecoveryContinueDispatchesExplicitRetry(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":          "completed",
		"completionState": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "provider retry resumed",
			"items": []map[string]any{{
				"item":         "provider retry explicit continue",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-provider-retry-continue/provider-retry-003-of-repair-001"},
			}},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-provider-retry-continue")
	runID := common.RunID("run-provider-retry-continue")
	rootStepID := common.StepID(string(runID) + "/repair-001")
	failedStepID := common.StepID(string(runID) + "/provider-retry-002-of-repair-001")
	ticketID := common.TicketID(string(failedStepID) + "-provider-retry-exhausted-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            "dod-provider-retry-continue",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		stepID := common.StepID(fmt.Sprintf("%s/provider-retry-%03d-of-repair-001", runID, attempt))
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
				"stateChangingOutputRequired":     true,
				"repairObjective":                 repairObjectiveStateChangingOutput,
				stepInputKeySoulStage:             soulStageExecution,
				stepInputKeyRuntimeStage:          runtimeStageRepair,
				stepInputKeyRuntimeStageMode:      "strict_convergence",
			},
			Output: map[string]any{
				"status":                  "failed",
				"stage":                   "llm_provider_unavailable",
				"error":                   "provider unavailable",
				failureOutputKeyClass:     failureClassProviderUnavailable,
				failureOutputKeySignature: "class=provider_unavailable|stage=llm_provider_unavailable|error=provider unavailable",
			},
		}); err != nil {
			t.Fatalf("seed provider retry step %d: %v", attempt, err)
		}
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-provider-retry-continue",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "继续处理", "recoveryAction": "retry_same_step"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume provider recovery continue: %v", err)
	}
	if len(runtime.calls) != 1 || !strings.Contains(string(runtime.calls[0]), "provider-retry-003-of-repair-001") {
		t.Fatalf("runtime calls = %#v, want explicit provider retry attempt 3", runtime.calls)
	}
	retry, err := stepRepo.Get(ctx, runtime.calls[0])
	if err != nil {
		t.Fatalf("get retry step: %v", err)
	}
	if retry.Input["decisionAuthorizedRetry"] != true {
		t.Fatalf("retry input = %+v, want decisionAuthorizedRetry", retry.Input)
	}
	if retry.Input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		retry.Input[stepInputKeyRuntimeStageMode] != "strict_convergence" ||
		retry.Input[stepInputKeySoulStage] != soulStageExecution {
		t.Fatalf("retry stage input = %+v, want source repair stage contract preserved", retry.Input)
	}
	if retry.Input["repairObjective"] != repairObjectiveStateChangingOutput || retry.Input["stateChangingOutputRequired"] != true {
		t.Fatalf("retry repair input = %+v, want state-changing repair contract preserved", retry.Input)
	}
}
