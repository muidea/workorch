package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type listFailingStepRepository struct {
	inner *memory.StepRepository
	err   error
}

func TestConfigOnlyDependencyResolutionStillBlockedRequiresRecoveryEvidence(t *testing.T) {
	ticket := &decisionmodel.Ticket{
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
		},
	}
	tests := []struct {
		name    string
		typ     string
		payload map[string]any
		want    bool
	}{
		{name: "plain continue remains blocked", payload: map[string]any{"answer": "continue"}, want: true},
		{name: "payload only replan remains blocked until resolution type is normalized", payload: map[string]any{"recoveryAction": "replan"}, want: true},
		{name: "explicit replan resolution can create successor without recovery evidence", typ: decisioncommon.ResolutionTypeReplan, payload: map[string]any{"recoveryAction": "replan"}, want: false},
		{name: "retry remains blocked without recovery evidence", payload: map[string]any{"recoveryAction": "retry_same_step"}, want: true},
		{name: "external dependency recovered can resume", payload: map[string]any{"externalDependencyRecovered": true}, want: false},
		{name: "system config changed can resume", payload: map[string]any{"systemConfigChanged": true}, want: false},
		{name: "runtime dependency recovered can resume", payload: map[string]any{"runtimeDependencyRecovered": true}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configOnlyDependencyResolutionStillBlocked(ticket, &decisionmodel.Resolution{Type: tt.typ, Payload: tt.payload})
			if got != tt.want {
				t.Fatalf("blocked = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuntimeOrchestrator_ProviderConfigReplanCreatesSuccessorWithoutRecoveryEvidence(t *testing.T) {
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
			"summary":   "provider config replan successor completed",
			"items": []map[string]any{{
				"item":         "successor run",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-provider-config-replan-replan-001/step-1"},
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
		RuntimeEvidenceRepositories{Results: resultRepo})

	workspaceID := common.WorkspaceID("ws-provider-config-replan")
	runID := common.RunID("run-provider-config-replan")
	failedStepID := common.StepID(runID + "/provider-retry-002-of-planning-001")
	ticketID := common.TicketID(failedStepID + "-provider-retry-exhausted-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            "dod-provider-config-replan",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":                   "llm_provider_timeout",
			"error":                   "tls handshake timeout",
			failureOutputKeyClass:     failureClassProviderTimeout,
			failureOutputKeySignature: "class=provider_timeout|stage=llm_provider_timeout|error=tls handshake timeout",
		},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-provider-config-replan",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeReplan,
		Payload:  map[string]any{"operator": "tester", "recoveryAction": "replan"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume provider config replan: %v", err)
	}
	predecessor, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("load predecessor: %v", err)
	}
	if predecessor.State != common.RunStateSuperseded || predecessor.BlockingTicketID != "" {
		t.Fatalf("predecessor = %+v, want superseded without blocker", predecessor)
	}
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateResolved {
		t.Fatalf("ticket state = %q, want resolved", ticket.State)
	}
	successorID := common.RunID(runID + "-replan-001")
	successor, err := runRepo.Get(ctx, successorID)
	if err != nil {
		t.Fatalf("load successor %s: %v", successorID, err)
	}
	replan := mapFromValue(successor.ExecutionContext["replanContinuation"])
	if got := stringFromMap(replan, "predecessorRunId"); got != string(runID) {
		t.Fatalf("successor replan predecessor = %q, want %s", got, runID)
	}
}

func TestRunningTerminalStepRecoverySpecsAllowsCompletedStepContinuation(t *testing.T) {
	ctx := context.Background()
	steps := memory.NewStepRepository()
	runID := common.RunID("run-completed-terminal-continuation")
	run := &runmodel.Run{ID: runID, State: common.RunStateActive}
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		State: common.StepStateSucceeded,
	}
	if err := steps.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	called := false
	specs, err := runningTerminalStepRecoverySpecs(ctx, steps, func(context.Context, *runmodel.Run) ([]stepDispatchSpec, bool, error) {
		return nil, false, nil
	}, func(_ context.Context, gotRun *runmodel.Run, gotStep *runmodel.Step) ([]stepDispatchSpec, error) {
		called = true
		if gotRun.ID != runID || gotStep.ID != step.ID {
			t.Fatalf("continuation args run=%q step=%q, want %q %q", gotRun.ID, gotStep.ID, runID, step.ID)
		}
		return []stepDispatchSpec{{ID: common.StepID(runID + "/step-2"), Kind: "llm/workspace_execution"}}, nil
	}, run)
	if err != nil {
		t.Fatalf("runningTerminalStepRecoverySpecs: %v", err)
	}
	if !called || len(specs) != 1 || specs[0].ID != common.StepID(runID+"/step-2") {
		t.Fatalf("specs=%+v called=%v, want completed step continuation", specs, called)
	}
}

func TestRunningTerminalStepRecoverySpecsAllowsFailedStepContinuation(t *testing.T) {
	ctx := context.Background()
	steps := memory.NewStepRepository()
	runID := common.RunID("run-failed-terminal-continuation")
	run := &runmodel.Run{ID: runID, State: common.RunStateActive}
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status": "failed",
			"error":  "terminal diagnostic is recoverable by current authority",
		},
	}
	if err := steps.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	called := false
	specs, err := runningTerminalStepRecoverySpecs(ctx, steps, func(context.Context, *runmodel.Run) ([]stepDispatchSpec, bool, error) {
		return nil, false, nil
	}, func(_ context.Context, gotRun *runmodel.Run, gotStep *runmodel.Step) ([]stepDispatchSpec, error) {
		called = true
		if gotRun.ID != runID || gotStep.ID != step.ID {
			t.Fatalf("continuation args run=%q step=%q, want %q %q", gotRun.ID, gotStep.ID, runID, step.ID)
		}
		return []stepDispatchSpec{{ID: common.StepID(runID + "/authority-continuation"), Kind: "llm/workspace_execution"}}, nil
	}, run)
	if err != nil {
		t.Fatalf("runningTerminalStepRecoverySpecs: %v", err)
	}
	if !called || len(specs) != 1 || specs[0].ID != common.StepID(runID+"/authority-continuation") {
		t.Fatalf("specs=%+v called=%v, want failed step continuation", specs, called)
	}
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

func (r *listFailingStepRepository) ListByRunSummary(context.Context, common.RunID) ([]*runmodel.Step, error) {
	return nil, r.err
}

func (r *listFailingStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	return r.inner.Update(ctx, item)
}

func (r *listFailingStepRepository) Delete(ctx context.Context, id common.StepID) error {
	return r.inner.Delete(ctx, id)
}

type createFailingStepRepository struct {
	inner       *memory.StepRepository
	err         error
	createCalls int
}

func (r *createFailingStepRepository) Create(context.Context, *runmodel.Step) error {
	r.createCalls++
	if r.err != nil {
		return r.err
	}
	return common.ErrConflict
}

func (r *createFailingStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	return r.inner.Get(ctx, id)
}

func (r *createFailingStepRepository) ListByRun(ctx context.Context, id common.RunID) ([]*runmodel.Step, error) {
	return r.inner.ListByRun(ctx, id)
}

func (r *createFailingStepRepository) ListByRunSummary(ctx context.Context, id common.RunID) ([]*runmodel.Step, error) {
	return r.inner.ListByRunSummary(ctx, id)
}

func (r *createFailingStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	return r.inner.Update(ctx, item)
}

func (r *createFailingStepRepository) Delete(ctx context.Context, id common.StepID) error {
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	notifications := newRuntimeNotificationRecorder(t)
	notifications.attach(orch)

	workspaceID := common.WorkspaceID("ws-fail-evt")
	runID := common.RunID("run-fail-evt")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if _, ok := notifications.waitFor(time.Second, func(item runtimeNotificationRecord) bool {
		return item.Topic == workspaceevents.TopicWorkspaceStateChanged && item.WorkspaceID == string(workspaceID) && item.ActiveRunID == ""
	}); !ok {
		t.Fatalf("notifications = %+v, want workspace release event", notifications.snapshot())
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
			orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"ok": true}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

			workspaceID := common.WorkspaceID("ws-terminal-dismiss-" + tc.name)
			runID := common.RunID("run-terminal-dismiss-" + tc.name)
			ticketID := common.TicketID("ticket-terminal-dismiss-" + tc.name)
			if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
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
			if ticket.State != common.DecisionTicketStateSuperseded || ticket.UpdatedAt == "" {
				t.Fatalf("ticket = %+v, want dismissed with updatedAt", ticket)
			}
		})
	}
}

func TestRuntimeOrchestrator_SupersedeCreatedRunReleasesWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"ok": true}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-supersede-created")
	runID := common.RunID("run-supersede-created")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.SupersedeRun(ctx, runID); err != nil {
		t.Fatalf("supersede created run: %v", err)
	}
	run, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.State != common.RunStateSuperseded {
		t.Fatalf("run state = %s, want superseded", run.State)
	}
	workspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want activeRunId released", workspace)
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

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
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-stale-terminal-active")
	staleRunID := common.RunID("run-stale-terminal-active")
	runID := common.RunID("run-start-after-stale-terminal")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateSuperseded}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
	if ws.State != common.WorkspaceStateActive || ws.ActiveRunID != "" {
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-start-initial-spec-fails")
	runID := common.RunID("run-start-initial-spec-fails")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
	if ws.State != common.WorkspaceStateActive || ws.ActiveRunID != "" {
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-running-empty-resume")
	runID := common.RunID("run-running-empty-resume")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
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
	if step.State != common.StepStateSucceeded {
		t.Fatalf("step state = %q, want completed", step.State)
	}
}

func TestRuntimeOrchestrator_ResumeRunningRunWithTerminalValidateAuthorityDispatchesContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &requestOnlyRuntimeAdapter{}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-terminal-validate-continuation")
	runID := common.RunID("run-terminal-validate-continuation")
	terminalStepID := common.StepID(runID + "/graph-001/state-change")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := runWithAuthorityFocusForTest("validate")
	run.ID = runID
	run.WorkspaceID = workspaceID
	run.State = common.RunStateActive
	updateAuthorityScopeForTest(run, workspaceID, runID)
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    terminalStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"status": "completed",
		},
	}); err != nil {
		t.Fatalf("seed terminal step: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume running terminal run: %v", err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("runtime requests = %d, want one authority continuation dispatch", len(runtime.requests))
	}
	request := runtime.requests[0]
	if request.RunID != runID || request.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("request = %+v, want scoped llm workspace execution", request)
	}
	nextAction := currentActionNextActionFromInput(request.Input)
	if got := stringFromMap(nextAction, "kind"); got != "validate" {
		t.Fatalf("request nextAction = %+v, want validate", nextAction)
	}
	required := mapFromValue(request.Input["requiredOutput"])
	if len(required) == 0 {
		t.Fatalf("request requiredOutput missing: %+v", request.Input)
	}
	acceptedKinds := stringSliceFromValue(required["acceptedKinds"])
	if !stringSliceContains(acceptedKinds, runtimemodel.StepKindShellCommand) ||
		!stringSliceContains(acceptedKinds, managedValidationSessionNodeKind) ||
		stringSliceContains(acceptedKinds, runtimemodel.RuntimeBlockKindPatchApply) {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want bounded validation-only producers", acceptedKinds)
	}
	example := mapFromValue(required["validationEvidenceNodeExample"])
	exampleInput := mapFromValue(example["input"])
	exampleEvidence := mapFromValue(exampleInput["validationEvidence"])
	if stringFromMap(exampleEvidence, "mode") == "" ||
		stringFromMap(exampleEvidence, "observationType") == "" ||
		len(stringSliceFromValue(exampleEvidence["observedClaims"])) == 0 ||
		!stringSliceContains(stringSliceFromValue(exampleEvidence["coverageWorkItemIds"]), "cwi-current") {
		t.Fatalf("validationEvidenceNodeExample = %+v, want concrete coverage-bound validation evidence", example)
	}
	assertRequiredOutputMirrorForTest(t, "repairActionPackage.requiredOutput", required, mapFromValue(mapFromValue(request.Input["repairActionPackage"])["requiredOutput"]))
	stagePacket := mapFromValue(request.Input[stepInputKeyStagePacket])
	assertRequiredOutputMirrorForTest(t, "stagePacket.requiredOutput", required, mapFromValue(stagePacket["requiredOutput"]))
	assertRequiredOutputMirrorForTest(t, "stagePacket.nextAction.requiredOutput", required, mapFromValue(mapFromValue(stagePacket["nextAction"])["requiredOutput"]))
	assertRequiredOutputMirrorForTest(t, "currentActionContract.requiredOutput", required, mapFromValue(mapFromValue(request.Input[currentActionContractKey])["requiredOutput"]))
	continuation, err := stepRepo.Get(ctx, request.StepID)
	if err != nil {
		t.Fatalf("get continuation step: %v", err)
	}
	if continuation.State != common.StepStateExecuting {
		t.Fatalf("continuation step state = %q, want executing", continuation.State)
	}
}

func TestRuntimeOrchestrator_ResumeRunningRunWithTerminalStateChangeAuthorityDispatchesContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &requestOnlyRuntimeAdapter{}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-terminal-statechange-continuation")
	runID := common.RunID("run-terminal-statechange-continuation")
	terminalStepID := common.StepID(runID + "/validation-diagnostic")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := stateChangeAuthorityRunForTest(workspaceID, runID, stateChangeAuthorityRunOptions{
		TargetRef: "target.txt",
		TargetSnapshots: []map[string]any{{
			"targetRef":         "target.txt",
			"exists":            true,
			"content":           "current target content\n",
			"contentHash":       "hash-target-current",
			"contentComplete":   true,
			"safeForFileWrite":  true,
			"safeForExactPatch": true,
			"runtimeManaged":    true,
		}},
	})
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    terminalStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"status": "completed",
		},
	}); err != nil {
		t.Fatalf("seed terminal step: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume running terminal run: %v", err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("runtime requests = %d, want one state-changing authority continuation dispatch", len(runtime.requests))
	}
	request := runtime.requests[0]
	nextAction := currentActionNextActionFromInput(request.Input)
	if got := stringFromMap(nextAction, "kind"); got != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("request nextAction = %+v, want state_change", nextAction)
	}
	contract := mapFromValue(request.Input["convergenceContract"])
	if got := stringFromMap(contract, "state"); got != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	if targetSnapshotPrerequisiteActive(request.Input) {
		t.Fatalf("request input unexpectedly requires target snapshot acquisition: %+v", request.Input)
	}
}

func TestRuntimeOrchestrator_TerminalStateChangeAuthorityRebuildsTargetSnapshotPrerequisite(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo, workspaces: workspaceRepo}

	workspaceID := common.WorkspaceID("ws-terminal-statechange-snapshot")
	runID := common.RunID("run-terminal-statechange-snapshot")
	terminalStepID := common.StepID(runID + "/validation-diagnostic")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := stateChangeAuthorityRunForTest(workspaceID, runID, stateChangeAuthorityRunOptions{
		TargetRef:                     "target.txt",
		RequiresCurrentTargetSnapshot: true,
	})
	terminalStep := &runmodel.Step{
		ID:    terminalStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"status": "completed",
		},
	}
	if err := stepRepo.Create(ctx, terminalStep); err != nil {
		t.Fatalf("seed terminal step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, terminalStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want one authority continuation spec", specs)
	}
	input, err := rebuildDispatchInputFromConvergenceAuthority(run, specs[0].ID, specs[0].Input)
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	if !targetSnapshotAcquisitionDispatchActive(input, state, nextAction) {
		t.Fatalf("targetSnapshotAcquisitionDispatchActive = false, state=%q nextAction=%q input=%+v", state, nextAction, input)
	}
	if !inputRequiresRuntimeManagedTargetSnapshotAcquisition(input) {
		t.Fatalf("inputRequiresRuntimeManagedTargetSnapshotAcquisition = false, input=%+v", input)
	}
	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(input)
	if !ok {
		t.Fatalf("runtimeManagedTargetSnapshotAcquisitionOutput ok = false, input=%+v", input)
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) != 1 || graph.Nodes[0].Kind != runtimemodel.StepKindFileRead || graph.Nodes[0].Input["path"] != "target.txt" {
		t.Fatalf("target snapshot graph = %+v/%v, want one file/read target.txt node", graph, ok)
	}
}

func TestRuntimeOrchestrator_ResumeRunningRunDispatchCreateFailureFailsRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	innerStepRepo := memory.NewStepRepository()
	stepRepo := &createFailingStepRepository{
		inner: innerStepRepo,
		err:   errors.New("dispatch step create failed"),
	}
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &requestOnlyRuntimeAdapter{}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-terminal-dispatch-failure")
	runID := common.RunID("run-terminal-dispatch-failure")
	terminalStepID := common.StepID(runID + "/graph-001/state-change")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := runWithAuthorityFocusForTest("validate")
	run.ID = runID
	run.WorkspaceID = workspaceID
	run.State = common.RunStateActive
	updateAuthorityScopeForTest(run, workspaceID, runID)
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := innerStepRepo.Create(ctx, &runmodel.Step{
		ID:    terminalStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"status": "completed",
		},
	}); err != nil {
		t.Fatalf("seed terminal step: %v", err)
	}

	err := orch.ResumeRun(ctx, runID)
	if err != nil && !strings.Contains(err.Error(), "dispatch step create failed") {
		t.Fatalf("ResumeRun err = %v, want nil or dispatch create failure", err)
	}
	if stepRepo.createCalls != 1 {
		t.Fatalf("dispatch create calls = %d, want one continuation create attempt", stepRepo.createCalls)
	}
	storedRun, getErr := runRepo.Get(ctx, runID)
	if getErr != nil {
		t.Fatalf("get run: %v", getErr)
	}
	if storedRun.State != common.RunStateFailed || !strings.Contains(storedRun.FailureReason, "dispatch step create failed") {
		t.Fatalf("run state/reason = %q/%q, want failed structured dispatch failure", storedRun.State, storedRun.FailureReason)
	}
	workspace, getErr := workspaceRepo.Get(ctx, workspaceID)
	if getErr != nil {
		t.Fatalf("get workspace: %v", getErr)
	}
	if workspace.ActiveRunID != "" {
		t.Fatalf("workspace active run = %q, want cleared after dispatch failure", workspace.ActiveRunID)
	}
	if len(runtime.requests) != 0 {
		t.Fatalf("runtime requests = %d, want no direct runtime dispatch after event publish failure", len(runtime.requests))
	}
}

type stateChangeAuthorityRunOptions struct {
	TargetRef                     string
	RequiresCurrentTargetSnapshot bool
	TargetSnapshots               []map[string]any
}

func stateChangeAuthorityRunForTest(workspaceID common.WorkspaceID, runID common.RunID, opts stateChangeAuthorityRunOptions) *runmodel.Run {
	targetRef := strings.TrimSpace(opts.TargetRef)
	if targetRef == "" {
		targetRef = "target.txt"
	}
	focus := map[string]any{
		"id":                   "cwi-state-change-current",
		"coverageWorkItemIds":  []string{"cwi-state-change-current"},
		"missingEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange},
		"requiredEvidenceKinds": []string{
			runtimemodel.EvidenceKindStateChange,
			runtimemodel.EvidenceKindValidation,
		},
		"status":               "open",
		"targetRefs":           []string{targetRef},
		"structuredTargetRefs": []map[string]any{{"targetRef": targetRef}},
	}
	authority := map[string]any{
		"schemaVersion": convergenceAuthoritySchema,
		"workspaceId":   string(workspaceID),
		"runId":         string(runID),
		"currentFocus":  cloneStepPayload(focus),
		"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
	}
	pkg := map[string]any{
		"repairTargetSet": []string{targetRef},
		"requiredOutput": map[string]any{
			"coverageWorkItemIds":           []string{"cwi-state-change-current"},
			"structuredTargetRefs":          []string{targetRef},
			"repairTargetSet":               []string{targetRef},
			"requiredEvidenceKinds":         []string{runtimemodel.EvidenceKindStateChange},
			"mustIncludeStateChangingNode":  true,
			"mustBindCoverageFocus":         true,
			"mustReferenceCoverageWorkItem": true,
			"acceptedKinds": []string{
				runtimemodel.RuntimeBlockKindPatchApply,
				runtimemodel.StepKindShellCommand,
				runtimemodel.StepKindMCPToolCall,
				runtimemodel.StepKindGitWrite,
			},
		},
	}
	if opts.RequiresCurrentTargetSnapshot {
		pkg["requiresCurrentTargetSnapshotForStateChange"] = true
		required := mapFromValue(pkg["requiredOutput"])
		required["mustUseCurrentTargetSnapshot"] = true
		pkg["requiredOutput"] = required
	}
	if len(opts.TargetSnapshots) > 0 {
		pkg["targetSnapshots"] = cloneStepInputValue(opts.TargetSnapshots)
	}
	return &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			coverageFocusKey:        cloneStepPayload(focus),
			convergenceAuthorityKey: cloneStepPayload(authority),
			gapBaselineKey:          cloneStepPayload(authority),
			"repairActionPackage":   pkg,
		},
	}
}

func assertRequiredOutputMirrorForTest(t *testing.T, label string, want map[string]any, got map[string]any) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("%s missing, want requiredOutput mirror", label)
	}
	if stableCoverageJSON(got) != stableCoverageJSON(want) {
		t.Fatalf("%s = %+v, want mirror %+v", label, got, want)
	}
}

func TestRuntimeOrchestrator_ReconcileWorkspaceActiveRunClearsTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-terminal-active-run")
	runID := common.RunID("run-terminal-active-run")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
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
	if ws.State != common.WorkspaceStateSuperseded || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want superseded with no terminal active run", ws)
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-start-state-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name  string
		state common.RunState
	}{
		{name: "running", state: common.RunStateActive},
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
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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

	orch := NewRuntimeOrchestrator(nil, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	if err := orch.DispatchNextStep(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty runID err = %v, want invalid argument", err)
	}

	runID := common.RunID("run-no-runtime")
	workspaceID := common.WorkspaceID("ws-no-runtime")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
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
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

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
		State:       common.RunStatePending,
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
	if decisionWorkspace.State != common.WorkspaceStateActive || decisionWorkspace.ActiveRunID != decisionRunID {
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
		State:       common.RunStateActive,
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
	if approvalWorkspace.State != common.WorkspaceStateBlockedOnApproval || approvalWorkspace.ActiveRunID != approvalRunID {
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
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

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
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	notifications := newRuntimeNotificationRecorder(t)
	notifications.attach(orch)

	workspaceID := common.WorkspaceID("ws-validating")
	runID := common.RunID("run-validating")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.ValidateRun(ctx, runID); err != nil {
		t.Fatalf("validate run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateActive || ws.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want validating with active run", ws)
	}
	if _, ok := notifications.waitFor(time.Second, func(item runtimeNotificationRecord) bool {
		return item.Topic == workspaceevents.TopicWorkspaceStateChanged && item.WorkspaceID == string(workspaceID) && item.RunID == string(runID) && item.State == string(common.WorkspaceStateActive)
	}); !ok {
		t.Fatalf("notifications = %+v, want workspace validating event", notifications.snapshot())
	}
}

func TestRuntimeOrchestrator_BlockRunCanReplaceStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)

	workspaceID := common.WorkspaceID("ws-stale-terminal-active")
	staleRunID := common.RunID("run-stale-terminal-active")
	currentRunID := common.RunID("run-current-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: staleRunID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: currentRunID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed current run: %v", err)
	}

	if err := orch.BlockOnApproval(ctx, currentRunID, common.ApprovalID("approval-current-block")); err != nil {
		t.Fatalf("block on approval with stale terminal active run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateBlockedOnApproval || ws.ActiveRunID != currentRunID {
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
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo)

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
		RuntimeEvidenceRepositories{Results: resultRepo})

	workspaceID := common.WorkspaceID("ws-provider-retry-continue")
	runID := common.RunID("run-provider-retry-continue")
	rootStepID := common.StepID(string(runID) + "/repair-001")
	failedStepID := common.StepID(string(runID) + "/provider-retry-002-of-repair-001")
	ticketID := common.TicketID(string(failedStepID) + "-provider-retry-exhausted-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
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
		Payload:  map[string]any{"answer": "继续处理", "recoveryAction": "retry_same_step", "externalDependencyRecovered": true},
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
