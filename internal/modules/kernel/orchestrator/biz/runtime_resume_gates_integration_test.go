package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_ResumeRunFinalizesDurableRuntimeTerminalResult(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "unexpected-runtime-call"}}
	hub, _ := newRecordingDispatchMagicHub(t)
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		hub,
	)
	orch.EnableEventDrivenDispatch()
	workspaceID := common.WorkspaceID("ws-resume-terminal-result")
	runID := common.RunID("run-resume-terminal-result")
	stepID := common.StepID("run-resume-terminal-result/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: llmcommon.StepKindWorkspaceExecution, State: common.StepStateExecuting, Input: map[string]any{}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := toolCallRepo.Create(ctx, &runmodel.ToolCall{
		ID:          common.ToolCallID(string(stepID) + "/tool-call/runtime-execute"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    llmcommon.StepKindWorkspaceExecution,
		State:       common.ToolCallStateRunning,
	}); err != nil {
		t.Fatalf("seed tool call: %v", err)
	}
	if err := orch.RecordRuntimeStepTerminalResult(ctx, runID, stepID, RuntimeStepTerminalResultCheckpoint{
		RequestID:     "request-resume-terminal",
		TerminalTopic: "/runtime/block/execute/completed",
		StepKind:      llmcommon.StepKindWorkspaceExecution,
		Output: map[string]any{
			"status":   "ok",
			"nextStep": "next-from-terminal-result",
			"nextKind": llmcommon.StepKindWorkspaceExecution,
		},
		CompletedAt: "2026-06-17T01:00:00Z",
	}); err != nil {
		t.Fatalf("record terminal result: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("read step: %v", err)
	}
	if step.State != common.StepStateSucceeded || step.Output["status"] != "ok" {
		t.Fatalf("step = %+v, want completed from durable terminal result", step)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %+v, want no provider redispatch", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesBlockExecutionGraphAfterApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "prepare", Kind: "graph/mock"},
		{ID: "apply", Kind: "graph/mock", DependsOn: []string{"prepare"}},
	}}
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-graph-approval/block/prepare":
				return map[string]any{
					"status":              "completed",
					"executionGraph":      graph,
					"blockApprovalReason": "requires approval",
				}, nil
			case "run-graph-approval/graph-001/002-apply":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph step")
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-approval")
	runID := common.RunID("run-graph-approval")
	prepareStepID := common.StepID("run-graph-approval/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateExecuting,
		Input: map[string]any{
			"executionGraph": graph,
			"graphNodeId":    "prepare",
		},
	}); err != nil {
		t.Fatalf("seed graph start step: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}
	blockedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run: %v", err)
	}
	if blockedRun.State != common.RunStateBlockedOnApproval || blockedRun.BlockingApprovalID == "" {
		t.Fatalf("blocked run = %+v, want approval block", blockedRun)
	}
	approval, err := approvalRepo.Get(ctx, blockedRun.BlockingApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("approve item: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume graph run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	applyStep, err := stepRepo.Get(ctx, "run-graph-approval/graph-001/002-apply")
	if err != nil {
		t.Fatalf("get apply step: %v", err)
	}
	if applyStep.State != common.StepStateSucceeded || applyStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("apply step = %+v, want resumed graph node after prepare", applyStep)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesConditionalBranchStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-branch/step-1":
				return map[string]any{
					"status":   "completed",
					"stage":    "route",
					"branch":   "fast-track",
					"nextStep": "run-branch/step-fast",
					"nextKind": "runtime/test",
				}, nil
			case "run-branch/step-fast":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-branch")
	runID := common.RunID("run-branch")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start branch run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step1, err := stepRepo.Get(ctx, "run-branch/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.Output["branch"] != "fast-track" {
		t.Fatalf("step1 branch = %v, want fast-track", step1.Output["branch"])
	}

	stepFast, err := stepRepo.Get(ctx, "run-branch/step-fast")
	if err != nil {
		t.Fatalf("get branch step: %v", err)
	}
	if stepFast.State != common.StepStateSucceeded {
		t.Fatalf("branch step state = %s, want completed", stepFast.State)
	}
	if stepFast.Input["previousStepId"] != "run-branch/step-1" {
		t.Fatalf("branch step previousStepId = %v, want run-branch/step-1", stepFast.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-branch/step-1" || runtime.calls[1] != "run-branch/step-fast" {
		t.Fatalf("runtime calls = %#v, want [run-branch/step-1 run-branch/step-fast]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesConditionalReviewStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-branch-review/step-1":
				return map[string]any{
					"status":   "completed",
					"stage":    "route",
					"branch":   "review-path",
					"nextStep": "run-branch-review/step-review",
					"nextKind": "runtime/test",
				}, nil
			case "run-branch-review/step-review":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-branch-review")
	runID := common.RunID("run-branch-review")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start branch review run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step1, err := stepRepo.Get(ctx, "run-branch-review/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.Output["branch"] != "review-path" {
		t.Fatalf("step1 branch = %v, want review-path", step1.Output["branch"])
	}

	stepReview, err := stepRepo.Get(ctx, "run-branch-review/step-review")
	if err != nil {
		t.Fatalf("get review step: %v", err)
	}
	if stepReview.State != common.StepStateSucceeded {
		t.Fatalf("review step state = %s, want completed", stepReview.State)
	}
	if stepReview.Input["previousStepId"] != "run-branch-review/step-1" {
		t.Fatalf("review step previousStepId = %v, want run-branch-review/step-1", stepReview.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-branch-review/step-1" || runtime.calls[1] != "run-branch-review/step-review" {
		t.Fatalf("runtime calls = %#v, want [run-branch-review/step-1 run-branch-review/step-review]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunBlocksOnDecisionWhenRuntimeRequestsDecisionGate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		output: map[string]any{
			"status":                           "completed",
			stepOutputKeyBlockDecisionReason:   "runtime decision gate",
			stepOutputKeyBlockDecisionTicketID: "ticket-runtime-block",
			stepOutputKeyNextStep:              "run-runtime-block/step-2",
			stepOutputKeyNextKind:              "runtime/test",
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-block")
	runID := common.RunID("run-runtime-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision {
		t.Fatalf("run state = %s, want blocked_on_decision", runItem.State)
	}
	if runItem.BlockingTicketID != "ticket-runtime-block" {
		t.Fatalf("blocking ticket = %s, want ticket-runtime-block", runItem.BlockingTicketID)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-block")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %s, want open", ticket.State)
	}
	if ticket.StepID != "run-runtime-block/step-1" {
		t.Fatalf("ticket step id = %s, want run-runtime-block/step-1", ticket.StepID)
	}

	step1, err := stepRepo.Get(ctx, "run-runtime-block/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.State != common.StepStateSucceeded {
		t.Fatalf("step1 state = %s, want completed", step1.State)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-block/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step2 err = %v, want not found", err)
	}

}

func TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeDecisionResolved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-resume/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "runtime decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-resume",
					stepOutputKeyNextStep:              "run-runtime-resume/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-runtime-resume/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-resume")
	runID := common.RunID("run-runtime-resume")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-resume")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-resume"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step2, err := stepRepo.Get(ctx, "run-runtime-resume/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateSucceeded {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-runtime-resume/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-runtime-resume/step-1", step2.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-runtime-resume/step-1" || runtime.calls[1] != "run-runtime-resume/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-resume/step-1 run-runtime-resume/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunResolvedDecisionUsesRunScopedTicketRead(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := newCountingDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if !strings.Contains(string(request.StepID), "/repair-") {
				return nil, fmt.Errorf("unexpected step %s", request.StepID)
			}
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair completed after operator guidance",
					"items": []map[string]any{
						{
							"item":         "repair completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-runtime-ticket-scope")
	runID := common.RunID("run-runtime-ticket-scope")
	failedStepID := common.StepID("run-runtime-ticket-scope/block/write_config")
	ticketID := common.TicketID("ticket-runtime-ticket-scope")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-runtime-ticket-scope"),
		DoDID:           common.DoDID("dod-runtime-ticket-scope"),
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		State:           common.RunStateActive,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{"targetRef": "llm-proxy/config.yaml"},
		Output: map[string]any{
			"error": "workspace patch path conflict",
		},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		Reason:      "operator guidance required",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-other-run"),
		WorkspaceID: workspaceID,
		RunID:       common.RunID("run-other"),
		StepID:      common.StepID("run-other/step-1"),
		Reason:      "other run ticket",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed unrelated ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-ticket-scope"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if got := ticketRepo.listCount.Load(); got != 0 {
		t.Fatalf("ticket List count = %d, want no full ticket scan", got)
	}
	if got := ticketRepo.listByRunIDCount.Load(); got < 1 {
		t.Fatalf("ticket ListByRunID count = %d, want at least one run scoped ticket read", got)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesFromFailedStepAfterRepairDecisionResolved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	var capturedInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if !strings.Contains(string(request.StepID), "/repair-") {
				return nil, fmt.Errorf("unexpected step %s", request.StepID)
			}
			capturedInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair completed after operator guidance",
					"items": []map[string]any{
						{
							"item":         "repair completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-repair-decision-resume")
	runID := common.RunID("run-repair-decision-resume")
	failedStepID := common.StepID("run-repair-decision-resume/block/write_config")
	ticketID := common.TicketID("ticket-repair-decision-resume")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           common.GoalID("goal-repair-decision-resume"),
		DoDID:            common.DoDID("dod-repair-decision-resume"),
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{"targetRef": "llm-proxy/config.yaml"},
		Output: map[string]any{
			"error": "workspace patch path conflict",
			"runtimeDiagnostics": map[string]any{
				"kind":        "workspace_path_conflict",
				"targetRef":   "llm-proxy/config.yaml",
				"conflictRef": "llm-proxy",
			},
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
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-repair-decision-resume"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"resumeInstruction": "continue from the root project"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if capturedInput == nil {
		t.Fatalf("captured input is nil")
	}
	if capturedInput["failedStepId"] != string(failedStepID) {
		t.Fatalf("failedStepId = %v, want %s", capturedInput["failedStepId"], failedStepID)
	}
	if _, ok := capturedInput["decisionResolution"].(map[string]any); !ok {
		t.Fatalf("decisionResolution missing from resume input: %#v", capturedInput)
	}
}

func TestRuntimeOrchestrator_ResumeRunRecoversRunningRunWithResolvedFailedStepDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if request.Input["decisionResolution"] == nil {
				return nil, errors.New("decision resolution input is required")
			}
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "stale running run recovered",
					"items": []map[string]any{
						{
							"item":         "stale running run recovered",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-running-recovery")
	runID := common.RunID("run-running-recovery")
	failedStepID := common.StepID("run-running-recovery/block/write_config")
	ticketID := common.TicketID("ticket-running-recovery")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-running-recovery"),
		DoDID:           common.DoDID("dod-running-recovery"),
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		State:           common.RunStateActive,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     failedStepID,
		RunID:  runID,
		Kind:   "patch/apply",
		State:  common.StepStateFailed,
		Output: map[string]any{"error": "workspace patch path conflict"},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-running-recovery"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"resumeInstruction": "continue from existing files"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume running run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_ResumeRunCreatesUniqueStepForMultiRoundDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runID := common.RunID("run-runtime-multi-decision")
	firstResumeStepID := common.StepID("run-runtime-multi-decision/llm-after-decision")
	secondResumeStepID := common.StepID("run-runtime-multi-decision/llm-after-decision-2")
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch request.StepID {
			case "run-runtime-multi-decision/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "first decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-multi-decision-first",
					stepOutputKeyNextStep:              string(firstResumeStepID),
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case firstResumeStepID:
				resolution := mapFromValue(request.Input["decisionResolution"])
				payload := mapFromValue(resolution["resolutionPayload"])
				if payload["answer"] != "first answer is incomplete" {
					return nil, fmt.Errorf("first resume payload = %+v, want first answer", payload)
				}
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockDecisionReason: "second decision gate",
					stepOutputKeyNextStep:            string(firstResumeStepID),
					stepOutputKeyNextKind:            "runtime/test",
				}, nil
			case secondResumeStepID:
				resolution := mapFromValue(request.Input["decisionResolution"])
				payload := mapFromValue(resolution["resolutionPayload"])
				if payload["answer"] != "second answer is enough" {
					return nil, fmt.Errorf("second resume payload = %+v, want second answer", payload)
				}
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, fmt.Errorf("unexpected step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-runtime-multi-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	resolveRuntimeTicket(t, ctx, ticketRepo, resolutionRepo, "ticket-runtime-multi-decision-first", "resolution-runtime-multi-decision-first", map[string]any{"answer": "first answer is incomplete"})
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("first resume run: %v", err)
	}
	secondTicketID := common.TicketID("run-runtime-multi-decision-llm-after-decision-decision")
	resolveRuntimeTicket(t, ctx, ticketRepo, resolutionRepo, secondTicketID, "resolution-runtime-multi-decision-second", map[string]any{"answer": "second answer is enough"})
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("second resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if _, err := stepRepo.Get(ctx, firstResumeStepID); err != nil {
		t.Fatalf("get first resume step: %v", err)
	}
	secondStep, err := stepRepo.Get(ctx, secondResumeStepID)
	if err != nil {
		t.Fatalf("get second resume step: %v", err)
	}
	if secondStep.Input["previousStepId"] != string(firstResumeStepID) {
		t.Fatalf("second previousStepId = %v, want %s", secondStep.Input["previousStepId"], firstResumeStepID)
	}
	if len(runtime.calls) != 3 ||
		runtime.calls[0] != "run-runtime-multi-decision/step-1" ||
		runtime.calls[1] != firstResumeStepID ||
		runtime.calls[2] != secondResumeStepID {
		t.Fatalf("runtime calls = %#v, want unique multi-round decision calls", runtime.calls)
	}
}

func resolveRuntimeTicket(
	t *testing.T,
	ctx context.Context,
	ticketRepo *memory.DecisionTicketRepository,
	resolutionRepo *memory.DecisionResolutionRepository,
	ticketID common.TicketID,
	resolutionID common.ResolutionID,
	payload map[string]any,
) {
	t.Helper()
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket %s: %v", ticketID, err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket %s: %v", ticketID, err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticket.ID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  payload,
	}); err != nil {
		t.Fatalf("create resolution %s: %v", resolutionID, err)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsRuntimeDecisionWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-decision-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "runtime decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-decision-reject",
					stepOutputKeyNextStep:              "run-runtime-decision-reject/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-decision-reject")
	runID := common.RunID("run-runtime-decision-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision reject run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-decision-reject")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-decision-reject"),
		TicketID: ticket.ID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester", "outcome": "reject"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected runtime decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" {
		t.Fatalf("blocking ticket = %q, want empty", runItem.BlockingTicketID)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-decision-reject/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-2, got %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-decision-reject/step-1" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-decision-reject/step-1]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeApprovalApproved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-approval/step-1":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "risky operation requires approval",
					stepOutputKeyBlockApprovalID:     "approval-runtime-approval",
					stepOutputKeyBlockApprovalAction: "remove public API",
					stepOutputKeyBlockApprovalContext: map[string]any{
						"risk":       "breaking_change",
						"targetFile": "internal/api.go",
					},
					stepOutputKeyNextStep: "run-runtime-approval/step-2",
					stepOutputKeyNextKind: "runtime/test",
				}, nil
			case "run-runtime-approval/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-approval")
	runID := common.RunID("run-runtime-approval")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-approval run: %v", err)
	}

	approval, err := approvalRepo.Get(ctx, "approval-runtime-approval")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.Reason != "risky operation requires approval" ||
		approval.Action != "remove public API" ||
		approval.Context["risk"] != "breaking_change" {
		t.Fatalf("approval = %+v, want persisted risk operation metadata", approval)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-approval/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step2 before approval err = %v, want not found", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-approval/step-1" {
		t.Fatalf("runtime calls before approval = %#v, want only step-1", runtime.calls)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step2, err := stepRepo.Get(ctx, "run-runtime-approval/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateSucceeded {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-runtime-approval/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-runtime-approval/step-1", step2.Input["previousStepId"])
	}
	approvalResolution, ok := step2.Input["approvalResolution"].(map[string]any)
	if !ok {
		t.Fatalf("step2 approvalResolution = %#v, want map", step2.Input["approvalResolution"])
	}
	if approvalResolution["action"] != "remove public API" {
		t.Fatalf("approvalResolution action = %v, want remove public API", approvalResolution["action"])
	}
	approvalContext, ok := approvalResolution["context"].(map[string]any)
	if !ok || approvalContext["risk"] != "breaking_change" || approvalContext["targetFile"] != "internal/api.go" {
		t.Fatalf("approvalResolution context = %#v, want risk metadata", approvalResolution["context"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-runtime-approval/step-1" || runtime.calls[1] != "run-runtime-approval/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-approval/step-1 run-runtime-approval/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsRuntimeApprovalWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-approval-reject/step-1":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "risky operation requires approval",
					stepOutputKeyBlockApprovalID:     "approval-runtime-approval-reject",
					stepOutputKeyBlockApprovalAction: "delete production data",
					stepOutputKeyBlockApprovalContext: map[string]any{
						"risk": "data_loss",
					},
					stepOutputKeyNextStep: "run-runtime-approval-reject/step-2",
					stepOutputKeyNextKind: "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-approval-reject")
	runID := common.RunID("run-runtime-approval-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-approval reject run: %v", err)
	}

	approval, err := approvalRepo.Get(ctx, "approval-runtime-approval-reject")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.Action != "delete production data" || approval.Context["risk"] != "data_loss" {
		t.Fatalf("approval = %+v, want persisted risky action metadata", approval)
	}
	approval.State = common.ApprovalStateRejected
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected runtime approval: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", runItem.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-approval-reject/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-2, got %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-approval-reject/step-1" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-approval-reject/step-1]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsMultipleDecisionGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi-gate/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 1",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-1",
					stepOutputKeyNextStep:              "run-multi-gate/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-multi-gate/step-2":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 2",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-2",
					stepOutputKeyNextStep:              "run-multi-gate/step-3",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-multi-gate/step-3":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi-gate")
	runID := common.RunID("run-multi-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	resolveTicket := func(ticketID common.TicketID, resolutionID common.ResolutionID) {
		t.Helper()
		ticket, err := ticketRepo.Get(ctx, ticketID)
		if err != nil {
			t.Fatalf("get ticket %s: %v", ticketID, err)
		}
		ticket.State = common.DecisionTicketStateResolved
		if err := ticketRepo.Update(ctx, ticket); err != nil {
			t.Fatalf("update ticket %s: %v", ticketID, err)
		}
		if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
			ID:       resolutionID,
			TicketID: ticketID,
			Type:     "provide_missing_info",
			Payload:  map[string]any{"operator": "tester"},
		}); err != nil {
			t.Fatalf("create resolution for %s: %v", ticketID, err)
		}
	}

	resolveTicket("ticket-multi-gate-1", "resolution-multi-gate-1")
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after first gate: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after first resume: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID != "ticket-multi-gate-2" {
		t.Fatalf("run after first resume = %+v, want blocked on second gate", runItem)
	}

	resolveTicket("ticket-multi-gate-2", "resolution-multi-gate-2")
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after second gate: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step3, err := stepRepo.Get(ctx, "run-multi-gate/step-3")
	if err != nil {
		t.Fatalf("get step3: %v", err)
	}
	if step3.State != common.StepStateSucceeded {
		t.Fatalf("step3 state = %s, want completed", step3.State)
	}
	if step3.Input["previousStepId"] != "run-multi-gate/step-2" {
		t.Fatalf("step3 previousStepId = %v, want run-multi-gate/step-2", step3.Input["previousStepId"])
	}
	if len(runtime.calls) != 3 || runtime.calls[0] != "run-multi-gate/step-1" || runtime.calls[1] != "run-multi-gate/step-2" || runtime.calls[2] != "run-multi-gate/step-3" {
		t.Fatalf("runtime calls = %#v, want [run-multi-gate/step-1 run-multi-gate/step-2 run-multi-gate/step-3]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsSecondDecisionGateWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi-gate-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 1",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-reject-1",
					stepOutputKeyNextStep:              "run-multi-gate-reject/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-multi-gate-reject/step-2":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 2",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-reject-2",
					stepOutputKeyNextStep:              "run-multi-gate-reject/step-3",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi-gate-reject")
	runID := common.RunID("run-multi-gate-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket1, err := ticketRepo.Get(ctx, "ticket-multi-gate-reject-1")
	if err != nil {
		t.Fatalf("get ticket1: %v", err)
	}
	ticket1.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket1); err != nil {
		t.Fatalf("update ticket1: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-multi-gate-reject-1"),
		TicketID: ticket1.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution1: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after first gate: %v", err)
	}

	ticket2, err := ticketRepo.Get(ctx, "ticket-multi-gate-reject-2")
	if err != nil {
		t.Fatalf("get ticket2: %v", err)
	}
	ticket2.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket2); err != nil {
		t.Fatalf("update ticket2: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-multi-gate-reject-2"),
		TicketID: ticket2.ID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution2: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected second gate: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" || runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", runItem.BlockingTicketID, runItem.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, "run-multi-gate-reject/step-3"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-3, got %v", err)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-multi-gate-reject/step-1" || runtime.calls[1] != "run-multi-gate-reject/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-multi-gate-reject/step-1 run-multi-gate-reject/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsDecisionThenApprovalGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-mixed-gate/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "mixed decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-mixed-gate-1",
					stepOutputKeyNextStep:              "run-mixed-gate/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-mixed-gate/step-2":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "mixed approval",
					stepOutputKeyBlockApprovalID:     "approval-mixed-gate-1",
					stepOutputKeyNextStep:            "run-mixed-gate/step-3",
					stepOutputKeyNextKind:            "runtime/test",
				}, nil
			case "run-mixed-gate/step-3":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-mixed-gate")
	runID := common.RunID("run-mixed-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-mixed-gate-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-mixed-gate-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after decision: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID != "approval-mixed-gate-1" {
		t.Fatalf("run after decision resume = %+v, want blocked on approval", runItem)
	}

	approval, err := approvalRepo.Get(ctx, "approval-mixed-gate-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step3, err := stepRepo.Get(ctx, "run-mixed-gate/step-3")
	if err != nil {
		t.Fatalf("get step3: %v", err)
	}
	if step3.State != common.StepStateSucceeded {
		t.Fatalf("step3 state = %s, want completed", step3.State)
	}
	if step3.Input["previousStepId"] != "run-mixed-gate/step-2" {
		t.Fatalf("step3 previousStepId = %v, want run-mixed-gate/step-2", step3.Input["previousStepId"])
	}
	if len(runtime.calls) != 3 || runtime.calls[0] != "run-mixed-gate/step-1" || runtime.calls[1] != "run-mixed-gate/step-2" || runtime.calls[2] != "run-mixed-gate/step-3" {
		t.Fatalf("runtime calls = %#v, want [run-mixed-gate/step-1 run-mixed-gate/step-2 run-mixed-gate/step-3]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsLLMNamedDecisionThenApprovalGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-llm-mixed/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "llm decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-llm-mixed-1",
					stepOutputKeyNextStep:              "run-llm-mixed/llm-after-decision",
					stepOutputKeyNextKind:              "llm/workspace-plan",
				}, nil
			case "run-llm-mixed/llm-after-decision":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "llm approval",
					stepOutputKeyBlockApprovalID:     "approval-llm-mixed-1",
					stepOutputKeyNextStep:            "run-llm-mixed/llm-after-approval",
					stepOutputKeyNextKind:            "llm/workspace-plan",
				}, nil
			case "run-llm-mixed/llm-after-approval":
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "multi gate completed",
						"items": []map[string]any{{
							"item":         "multi gate completed",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-llm-mixed/llm-after-approval"},
						}},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected step %s", stepID)
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-llm-mixed")
	runID := common.RunID("run-llm-mixed")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending, DoDID: "dod-llm-mixed"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-llm-mixed-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-llm-mixed-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}
	afterDecision, err := stepRepo.Get(ctx, "run-llm-mixed/llm-after-decision")
	if err != nil {
		t.Fatalf("get after decision step: %v", err)
	}
	if afterDecision.Input[stepInputKeySoulStage] != soulStageDecision {
		t.Fatalf("after decision soulStage = %v, want decision", afterDecision.Input[stepInputKeySoulStage])
	}
	decisionResolution, ok := afterDecision.Input["decisionResolution"].(map[string]any)
	if !ok {
		t.Fatalf("decisionResolution input = %#v, want map", afterDecision.Input["decisionResolution"])
	}
	if decisionResolution["ticketId"] != string(ticket.ID) || decisionResolution["resolutionType"] != "provide_missing_info" {
		t.Fatalf("decisionResolution = %#v, want ticket and provide_missing_info", decisionResolution)
	}
	resolutionPayload, ok := decisionResolution["resolutionPayload"].(map[string]any)
	if !ok || resolutionPayload["operator"] != "tester" {
		t.Fatalf("decisionResolution payload = %#v, want operator tester", decisionResolution["resolutionPayload"])
	}

	approval, err := approvalRepo.Get(ctx, "approval-llm-mixed-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	afterApproval, err := stepRepo.Get(ctx, "run-llm-mixed/llm-after-approval")
	if err != nil {
		t.Fatalf("get after approval step: %v", err)
	}
	if afterApproval.Input["previousStepId"] != "run-llm-mixed/llm-after-decision" {
		t.Fatalf("after approval previousStepId = %v, want run-llm-mixed/llm-after-decision", afterApproval.Input["previousStepId"])
	}
	if afterApproval.Input[stepInputKeySoulStage] != soulStageApproval {
		t.Fatalf("after approval soulStage = %v, want approval", afterApproval.Input[stepInputKeySoulStage])
	}
	approvalResolution, ok := afterApproval.Input["approvalResolution"].(map[string]any)
	if !ok {
		t.Fatalf("approvalResolution input = %#v, want map", afterApproval.Input["approvalResolution"])
	}
	if approvalResolution["approvalId"] != string(approval.ID) || approvalResolution["state"] != string(common.ApprovalStateApproved) {
		t.Fatalf("approvalResolution = %#v, want approved approval", approvalResolution)
	}
	if len(runtime.calls) != 3 ||
		runtime.calls[0] != "run-llm-mixed/step-1" ||
		runtime.calls[1] != "run-llm-mixed/llm-after-decision" ||
		runtime.calls[2] != "run-llm-mixed/llm-after-approval" {
		t.Fatalf("runtime calls = %#v, want llm named decision then approval flow", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsDecisionThenRejectedApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-mixed-gate-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "mixed decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-mixed-gate-reject-1",
					stepOutputKeyNextStep:              "run-mixed-gate-reject/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-mixed-gate-reject/step-2":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "mixed approval",
					stepOutputKeyBlockApprovalID:     "approval-mixed-gate-reject-1",
					stepOutputKeyNextStep:            "run-mixed-gate-reject/step-3",
					stepOutputKeyNextKind:            "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-mixed-gate-reject")
	runID := common.RunID("run-mixed-gate-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-mixed-gate-reject-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-mixed-gate-reject-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after decision: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID != "approval-mixed-gate-reject-1" {
		t.Fatalf("run after decision resume = %+v, want blocked on approval", runItem)
	}

	approval, err := approvalRepo.Get(ctx, "approval-mixed-gate-reject-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateRejected
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after rejected approval: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" || runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", runItem.BlockingTicketID, runItem.BlockingApprovalID)
	}

	step2, err := stepRepo.Get(ctx, "run-mixed-gate-reject/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateSucceeded {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-mixed-gate-reject/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-mixed-gate-reject/step-1", step2.Input["previousStepId"])
	}
	if _, err := stepRepo.Get(ctx, "run-mixed-gate-reject/step-3"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-3, got %v", err)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-mixed-gate-reject/step-1" || runtime.calls[1] != "run-mixed-gate-reject/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-mixed-gate-reject/step-1 run-mixed-gate-reject/step-2]", runtime.calls)
	}
}
