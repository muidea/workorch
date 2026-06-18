package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleStartRunCommandAcceptsStateAndSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-start")
	workspaceID := common.WorkspaceID("ws-module-background-start")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	err := module.handleCommand(
		common.ContextWithRuntimeLLMProviderID(ctx, "provider-test"),
		events.Envelope{
			Topic:       orchestratorevents.TopicOrchestratorCommandStartRun,
			SubjectType: "run",
			SubjectID:   string(runID),
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			Payload:     map[string]any{events.PayloadKeyRunID: string(runID)},
		},
	)
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("start command err = %v, want response", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled dispatch", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	run, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if run.State != common.RunStateRunning {
		t.Fatalf("run state = %s, want running before background dispatch executes", run.State)
	}
	workspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if workspace.State != common.WorkspaceStateRunning || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want running active run", workspace)
	}
}

func TestModuleResumeRunCommandSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-resume")
	workspaceID := common.WorkspaceID("ws-module-background-resume")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	err := module.handleCommand(
		common.ContextWithRuntimeLLMProviderID(ctx, "provider-test"),
		events.Envelope{
			Topic:       orchestratorevents.TopicOrchestratorCommandResumeRun,
			SubjectType: "run",
			SubjectID:   string(runID),
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			Payload: map[string]any{
				events.PayloadKeyCommand: runcontracts.ResumeRunCommand{
					WorkspaceID:           workspaceID,
					RunID:                 runID,
					ExecutionContextPatch: map[string]any{"decisionResolution": "replan"},
				},
			},
		},
	)
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("resume command err = %v, want response", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled resume", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
}

func TestModuleRecoverRuntimeGapCommandSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-runtime-gap-recovery")
	stepID := common.StepID("run-module-runtime-gap-recovery/step-001")
	workspaceID := common.WorkspaceID("ws-module-runtime-gap-recovery")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	err := module.handleCommand(ctx, events.Envelope{
		Topic:       orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap,
		SubjectType: "run",
		SubjectID:   string(runID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyCommand: runcontracts.RecoverRuntimeGapCommand{
				WorkspaceID:    workspaceID,
				RunID:          runID,
				StepID:         stepID,
				StepKind:       "llm/workspace_execution",
				Gap:            "worker_lost",
				RequestID:      "request-runtime-gap",
				QueueKey:       "queue-runtime-gap",
				LatestStepTime: "2026-06-17T14:00:00Z",
				Reason:         "stale dispatched runtime gap: worker_lost",
			},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("recover runtime gap command err = %v, want response", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled runtime gap recovery", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
}

func TestRuntimeGapRecoveryPatchContainsGapSpecificContext(t *testing.T) {
	cmd := runcontracts.RecoverRuntimeGapCommand{
		WorkspaceID:    "ws-runtime-gap-patch",
		RunID:          "run-runtime-gap-patch",
		StepID:         "run-runtime-gap-patch/step-001",
		StepKind:       "llm/workspace_execution",
		Gap:            "runtime_terminal_not_finalized",
		RequestID:      "request-runtime-gap-patch",
		QueueKey:       "queue-runtime-gap-patch",
		LatestStepTime: "2026-06-17T14:00:00Z",
		RecoveryPolicy: runtimemodel.RuntimeRecoveryPolicy{
			SideEffectPolicy:  runtimemodel.SideEffectPolicyExternalRead,
			IdempotencyPolicy: runtimemodel.IdempotencyPolicyIdempotent,
			WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
		},
		RecoverySafety:        "recoverable_idempotent",
		Reason:                "stale dispatched runtime gap: runtime_terminal_not_finalized",
		ExecutionContextPatch: map[string]any{"existing": "value"},
	}

	patch := runtimeGapRecoveryPatch(cmd)
	if patch["existing"] != "value" {
		t.Fatalf("patch = %+v, want existing patch preserved", patch)
	}
	recovery, ok := patch["runtimeGapRecovery"].(map[string]any)
	if !ok {
		t.Fatalf("runtimeGapRecovery = %T, want map", patch["runtimeGapRecovery"])
	}
	if recovery["mode"] != "runtime_gap_recovery" ||
		recovery["nextAction"] != "continue_run" ||
		recovery["workspaceId"] != string(cmd.WorkspaceID) ||
		recovery["runId"] != string(cmd.RunID) ||
		recovery["stepId"] != string(cmd.StepID) ||
		recovery["gap"] != cmd.Gap ||
		recovery["requestId"] != cmd.RequestID ||
		recovery["queueKey"] != cmd.QueueKey ||
		recovery["latestStepTime"] != cmd.LatestStepTime ||
		recovery["recoverySafety"] != cmd.RecoverySafety ||
		recovery["reason"] != cmd.Reason {
		t.Fatalf("runtimeGapRecovery = %+v, want gap-specific context", recovery)
	}
	policy, _ := recovery["runtimeRecoveryPolicy"].(map[string]any)
	if policy["watchdogRecovery"] != runtimemodel.WatchdogRecoveryRetryExternal {
		t.Fatalf("runtimeRecoveryPolicy = %+v, want retry_external", policy)
	}
}

func TestModuleResumeRunCommandRejectsTerminalRunBeforeBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-resume-terminal")
	workspaceID := common.WorkspaceID("ws-module-background-resume-terminal")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateFailed, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateFailed}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	err := module.handleCommand(ctx, events.Envelope{
		Topic:       orchestratorevents.TopicOrchestratorCommandResumeRun,
		SubjectType: "run",
		SubjectID:   string(runID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyCommand: runcontracts.ResumeRunCommand{WorkspaceID: workspaceID, RunID: runID},
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resume terminal err = %v, want conflict", err)
	}
	if bg.asyncFunctionCalls != 0 {
		t.Fatalf("background calls = %d, want terminal run rejected before scheduling", bg.asyncFunctionCalls)
	}
}

func TestModuleDispatchStepRequestedSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-dispatch-requested")
	stepID := common.StepID("run-module-dispatch-requested/step-1")
	workspaceID := common.WorkspaceID("ws-module-dispatch-requested")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	if err := module.handleDispatchStepRequested(ctx, events.Envelope{
		Topic:       orchestratorevents.TopicOrchestratorDispatchStepRequested,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
		Payload: map[string]any{
			events.PayloadKeyCommand: orchestratorcontracts.DispatchStepCommand{
				WorkspaceID: workspaceID,
				RunID:       runID,
				StepID:      stepID,
				Kind:        "llm/workspace_execution",
				Input:       map[string]any{"seed": "yes"},
			},
		},
	}); err != nil {
		t.Fatalf("handleDispatchStepRequested: %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one dispatch task", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	if _, err := stepRepo.Get(ctx, stepID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step before background dispatch err = %v, want not found", err)
	}
	bg.lastAsyncFunction()
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("step after background dispatch: %v", err)
	}
	if step.State != common.StepStateCompleted || step.Output["completionState"] != "completed" {
		t.Fatalf("step = %+v, want completed runtime dispatch", step)
	}
}

func TestModuleRuntimeCompletedEventSchedulesFinalize(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-runtime-completed")
	stepID := common.StepID("run-module-runtime-completed/step-1")
	workspaceID := common.WorkspaceID("ws-module-runtime-completed")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	toolCalls := memoryrepo.NewToolCallRepository()
	checkpoints := memoryrepo.NewRuntimeCheckpointRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched, Input: map[string]any{}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := toolCalls.Create(ctx, &runmodel.ToolCall{ID: common.ToolCallID(string(stepID) + "/tool-call/runtime-execute"), WorkspaceID: workspaceID, RunID: runID, StepID: stepID, ToolName: "llm/workspace_execution", State: common.ToolCallStateRunning}); err != nil {
		t.Fatalf("seed tool call: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		nil,
		memoryrepo.NewApprovalRepository(),
		orchestratorbiz.RuntimeEvidenceRepositories{
			ToolCalls:          toolCalls,
			ValidationResults:  memoryrepo.NewValidationResultRepository(),
			RuntimeCheckpoints: checkpoints,
		},
		events.NewMemoryHub(),
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	err := module.handleRuntimeResultEvent(ctx, events.Envelope{
		Topic:       runtimeevent.TopicRuntimeBlockExecuteCompleted,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeCompleted: runtimemodel.RuntimeStepExecutionCompleted{
				WorkspaceID: workspaceID,
				RunID:       runID,
				StepID:      stepID,
				StepKind:    "llm/workspace_execution",
				RequestID:   "request-1",
				Output:      map[string]any{"status": "ok"},
			},
		},
	})
	if err != nil {
		t.Fatalf("handleRuntimeResultEvent: %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one finalize task", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	bg.lastAsyncFunction()
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("read step: %v", err)
	}
	if step.State != common.StepStateCompleted || step.Output["status"] != "ok" {
		t.Fatalf("step = %+v, want completed status ok", step)
	}
	items, err := checkpoints.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasModuleRuntimeTerminalResultCheckpoint(items, "request-1", runtimeevent.TopicRuntimeBlockExecuteCompleted, "ok", "") {
		t.Fatalf("checkpoints = %+v, want durable completed runtime result", items)
	}
}

func TestModuleRuntimeFailedEventSchedulesFinalize(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-runtime-failed")
	stepID := common.StepID("run-module-runtime-failed/step-1")
	workspaceID := common.WorkspaceID("ws-module-runtime-failed")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	toolCalls := memoryrepo.NewToolCallRepository()
	checkpoints := memoryrepo.NewRuntimeCheckpointRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched, Input: map[string]any{}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := toolCalls.Create(ctx, &runmodel.ToolCall{ID: common.ToolCallID(string(stepID) + "/tool-call/runtime-execute"), WorkspaceID: workspaceID, RunID: runID, StepID: stepID, ToolName: "llm/workspace_execution", State: common.ToolCallStateRunning}); err != nil {
		t.Fatalf("seed tool call: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		nil,
		memoryrepo.NewApprovalRepository(),
		orchestratorbiz.RuntimeEvidenceRepositories{
			ToolCalls:          toolCalls,
			ValidationResults:  memoryrepo.NewValidationResultRepository(),
			RuntimeCheckpoints: checkpoints,
		},
		events.NewMemoryHub(),
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	err := module.handleRuntimeResultEvent(ctx, events.Envelope{
		Topic:       runtimeevent.TopicRuntimeBlockExecuteFailed,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeFailed: runtimemodel.RuntimeStepExecutionFailed{
				WorkspaceID: workspaceID,
				RunID:       runID,
				StepID:      stepID,
				StepKind:    "llm/workspace_execution",
				RequestID:   "request-1",
				Error:       "provider unavailable",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleRuntimeResultEvent: %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one finalize task", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	bg.lastAsyncFunction()
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("read step: %v", err)
	}
	if step.State != common.StepStateFailed {
		t.Fatalf("step = %+v, want failed", step)
	}
	items, err := checkpoints.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasModuleRuntimeTerminalResultCheckpoint(items, "request-1", runtimeevent.TopicRuntimeBlockExecuteFailed, "", "provider unavailable") {
		t.Fatalf("checkpoints = %+v, want durable failed runtime result", items)
	}
}

func hasModuleRuntimeTerminalResultCheckpoint(items []*runmodel.RuntimeCheckpoint, requestID string, terminalTopic string, outputStatus string, errorText string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["event"] != "runtime_step_result_available" ||
			item.Data["requestId"] != requestID ||
			item.Data["terminalTopic"] != terminalTopic {
			continue
		}
		if outputStatus != "" {
			output, _ := item.Data["output"].(map[string]any)
			if output["status"] != outputStatus {
				continue
			}
		}
		if errorText != "" && item.Data["error"] != errorText {
			continue
		}
		return true
	}
	return false
}

func TestModuleRuntimeProgressEventSchedulesCheckpoint(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-runtime-progress")
	stepID := common.StepID("run-module-runtime-progress/step-1")
	workspaceID := common.WorkspaceID("ws-module-runtime-progress")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	checkpoints := memoryrepo.NewRuntimeCheckpointRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched, Input: map[string]any{}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		nil,
		memoryrepo.NewApprovalRepository(),
		orchestratorbiz.RuntimeEvidenceRepositories{RuntimeCheckpoints: checkpoints},
		events.NewMemoryHub(),
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	err := module.handleRuntimeProgressEvent(ctx, events.Envelope{
		Topic:       runtimeevent.TopicRuntimeBlockExecuteProgress,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeProgress: runtimemodel.RuntimeStepExecutionProgressed{
				WorkspaceID: workspaceID,
				RunID:       runID,
				StepID:      stepID,
				StepKind:    "llm/workspace_execution",
				RequestID:   "request-progress",
				Event:       "provider_stream",
				Stage:       "streaming",
				Summary:     "tokens received",
				Data:        map[string]any{"tokens": 3},
				OccurredAt:  "2026-06-17T00:00:00Z",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleRuntimeProgressEvent: %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one progress task", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	bg.lastAsyncFunction()
	items, err := checkpoints.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(items) != 1 ||
		items[0].CurrentStepID != stepID ||
		items[0].Data["event"] != "provider_stream" ||
		items[0].Data["requestId"] != "request-progress" ||
		items[0].Data["tokens"] != 3 {
		t.Fatalf("checkpoints = %+v, want provider_stream progress checkpoint", items)
	}
}

func orchestratorServiceDependencies(orch *orchestratorbiz.RuntimeOrchestrator) orchestratorservice.Dependencies {
	return orchestratorservice.Dependencies{
		StartRun:         orch.StartRun,
		DispatchNextStep: orch.DispatchNextStep,
		BlockOnDecision:  orch.BlockOnDecision,
		BlockOnApproval:  orch.BlockOnApproval,
		ResumeRun:        orch.ResumeRun,
		ResumeRunPatch:   orch.ResumeRunWithExecutionContextPatch,
		ValidateRun:      orch.ValidateRun,
		CompleteRun:      orch.CompleteRun,
		CancelRun:        orch.CancelRun,
		SupersedeRun:     orch.SupersedeRun,
		FailRun:          orch.FailRun,
	}
}

type moduleNoopRuntimeAdapter struct{}

func (moduleNoopRuntimeAdapter) ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return runtimemodel.StepExecutionResult{Output: map[string]any{"completionState": "completed"}}, nil
}

type recordingBackgroundRoutine struct {
	asyncFunctionCalls int
	lastAsyncFunction  func()
	err                error
}

func (r *recordingBackgroundRoutine) AsyncTask(task magictask.Task) error {
	return r.AsyncFunction(task.Run)
}

func (r *recordingBackgroundRoutine) SyncTask(task magictask.Task) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) SyncTaskWithTimeOut(task magictask.Task, _ time.Duration) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) AsyncFunction(function func()) error {
	r.asyncFunctionCalls++
	r.lastAsyncFunction = function
	return r.err
}

func (r *recordingBackgroundRoutine) SyncFunction(function func()) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) SyncFunctionWithTimeOut(function func(), _ time.Duration) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) Timer(context.Context, magictask.Task, time.Duration, time.Duration) error {
	return nil
}

func (r *recordingBackgroundRoutine) Shutdown(context.Context) bool {
	return true
}
