package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.servicePtr != nil {
		t.Fatal("new module service pointer is non-nil before Configure")
	}

	deps := moduleContractDependencies()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.servicePtr == nil {
		t.Fatal("service pointer is nil after Configure")
	}
	if module.bizPtr == nil {
		t.Fatal("biz pointer is nil after Configure")
	}
	if module.rawEventHub == nil {
		t.Fatal("command hub is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.runtimeTopics) + len(module.rawTopics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe orchestrator commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.runtimeTopics)+len(module.rawTopics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.runtimeTopics)+len(module.rawTopics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.servicePtr != nil ||
		module.bizPtr != nil ||
		module.rawEventHub != nil ||
		module.steps != nil ||
		module.backgroundRoutine != nil ||
		len(module.runtimeTopics) != 0 ||
		len(module.rawTopics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := moduleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing runtime", deps: func() Dependencies { d := base; d.Runtime = nil; return d }()},
		{name: "missing steps", deps: func() Dependencies { d := base; d.Steps = nil; return d }()},
		{name: "missing tool calls", deps: func() Dependencies { d := base; d.ToolCalls = nil; return d }()},
		{name: "missing patches", deps: func() Dependencies { d := base; d.Patches = nil; return d }()},
		{name: "missing validation results", deps: func() Dependencies { d := base; d.ValidationResults = nil; return d }()},
		{name: "missing checkpoints", deps: func() Dependencies { d := base; d.RuntimeCheckpoints = nil; return d }()},
		{name: "missing evidence", deps: func() Dependencies { d := base; d.EvidenceRecords = nil; return d }()},
		{name: "missing coverage", deps: func() Dependencies { d := base; d.CoverageWorkItems = nil; return d }()},
		{name: "missing artifact continuations", deps: func() Dependencies { d := base; d.ArtifactContinuations = nil; return d }()},
		{name: "missing command hub", deps: func() Dependencies { d := base; d.CommandHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestModuleStartRunCommandAcceptsStateAndSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-start")
	workspaceID := common.WorkspaceID("ws-module-background-start")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(
		common.ContextWithRuntimeWorkspaceID(common.ContextWithRuntimeLLMProviderID(ctx, "provider-test"), workspaceID),
		orchestratorevents.TopicOrchestratorCommandStartRun,
		orchestratorevents.StartRunCommand{RunID: runID},
	)
	if err != nil {
		t.Fatalf("start command err = %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled dispatch", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	run, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if run.State != common.RunStateActive {
		t.Fatalf("run state = %s, want running before background dispatch executes", run.State)
	}
	workspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if workspace.State != common.WorkspaceStateActive || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want active workspace with active run", workspace)
	}
}

func TestModuleResumeRunCommandSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-resume")
	workspaceID := common.WorkspaceID("ws-module-background-resume")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(
		common.ContextWithRuntimeLLMProviderID(ctx, "provider-test"),
		orchestratorevents.TopicOrchestratorCommandResumeRun,
		orchestratorevents.ResumeRunCommand{
			WorkspaceID:           workspaceID,
			RunID:                 runID,
			ExecutionContextPatch: map[string]any{"decisionResolution": "replan"},
		},
	)
	if err != nil {
		t.Fatalf("resume command err = %v", err)
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(ctx, orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap, orchestratorevents.RecoverRuntimeGapCommand{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		StepKind:       "llm/workspace_execution",
		Gap:            "worker_lost",
		IdempotencyKey: "runtime-gap-recovery|ws-module-runtime-gap-recovery|run-module-runtime-gap-recovery|run-module-runtime-gap-recovery/step-001|worker_lost|request-runtime-gap|queue-runtime-gap",
		RequestID:      "request-runtime-gap",
		QueueKey:       "queue-runtime-gap",
		LatestStepTime: "2026-06-17T14:00:00Z",
		RecoveryPolicy: runtimemodel.RuntimeRecoveryPolicy{
			SideEffectPolicy:  runtimemodel.SideEffectPolicyExternalRead,
			IdempotencyPolicy: runtimemodel.IdempotencyPolicyIdempotent,
			WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
		},
		Reason: "stale dispatched runtime gap: worker_lost",
	})
	if err != nil {
		t.Fatalf("recover runtime gap command err = %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled runtime gap recovery", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
}

func TestModuleRecoverRuntimeGapCommandDeduplicatesInFlightIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-runtime-gap-recovery-dedupe")
	stepID := common.StepID("run-module-runtime-gap-recovery-dedupe/step-001")
	workspaceID := common.WorkspaceID("ws-module-runtime-gap-recovery-dedupe")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}
	key := "runtime-gap-recovery|ws-module-runtime-gap-recovery-dedupe|run-module-runtime-gap-recovery-dedupe|run-module-runtime-gap-recovery-dedupe/step-001|worker_lost|request-runtime-gap|queue-runtime-gap"
	cmd := orchestratorevents.RecoverRuntimeGapCommand{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		StepKind:       "llm/workspace_execution",
		Gap:            "worker_lost",
		IdempotencyKey: key,
		RequestID:      "request-runtime-gap",
		QueueKey:       "queue-runtime-gap",
		LatestStepTime: "2026-06-17T14:00:00Z",
		RecoveryPolicy: runtimemodel.RuntimeRecoveryPolicy{
			SideEffectPolicy:  runtimemodel.SideEffectPolicyExternalRead,
			IdempotencyPolicy: runtimemodel.IdempotencyPolicyIdempotent,
			WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
		},
		Reason: "stale dispatched runtime gap: worker_lost",
	}

	if _, err := module.executeMagicCommand(ctx, orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap, cmd); err != nil {
		t.Fatalf("first recover runtime gap command err = %v", err)
	}
	if _, err := module.executeMagicCommand(ctx, orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap, cmd); err != nil {
		t.Fatalf("duplicate recover runtime gap command err = %v", err)
	}
	if bg.asyncFunctionCalls != 1 {
		t.Fatalf("background calls = %d, want duplicate in-flight key to schedule once", bg.asyncFunctionCalls)
	}
}

func TestModuleRecoverRuntimeGapCommandRejectsUnsafeDirectCommand(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-runtime-gap-recovery-unsafe")
	stepID := common.StepID("run-module-runtime-gap-recovery-unsafe/step-001")
	workspaceID := common.WorkspaceID("ws-module-runtime-gap-recovery-unsafe")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(ctx, orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap, orchestratorevents.RecoverRuntimeGapCommand{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		StepKind:       "llm/workspace_execution",
		Gap:            "worker_lost",
		IdempotencyKey: "runtime-gap-recovery|unsafe",
		RequestID:      "request-runtime-gap",
		QueueKey:       "queue-runtime-gap",
		RecoveryPolicy: runtimemodel.RuntimeRecoveryPolicy{
			SideEffectPolicy:  runtimemodel.SideEffectPolicyWorkspaceRead,
			IdempotencyPolicy: runtimemodel.IdempotencyPolicyIdempotent,
			WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
		},
		Reason: "stale dispatched runtime gap: worker_lost",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("recover unsafe runtime gap err = %v, want ErrInvalidArgument", err)
	}
	if bg.asyncFunctionCalls != 0 || bg.lastAsyncFunction != nil {
		t.Fatalf("background calls = %d fn=%v, want unsafe command rejected before scheduling", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
}

func TestRuntimeGapRecoveryPatchContainsGapSpecificContext(t *testing.T) {
	cmd := runcontracts.RecoverRuntimeGapCommand{
		WorkspaceID:    "ws-runtime-gap-patch",
		RunID:          "run-runtime-gap-patch",
		StepID:         "run-runtime-gap-patch/step-001",
		StepKind:       "llm/workspace_execution",
		Gap:            "runtime_terminal_not_finalized",
		IdempotencyKey: "runtime-gap-recovery|ws-runtime-gap-patch|run-runtime-gap-patch|run-runtime-gap-patch/step-001|runtime_terminal_not_finalized|request-runtime-gap-patch|queue-runtime-gap-patch",
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
		recovery["nextAction"] != "resume_run" ||
		recovery["workspaceId"] != string(cmd.WorkspaceID) ||
		recovery["runId"] != string(cmd.RunID) ||
		recovery["stepId"] != string(cmd.StepID) ||
		recovery["gap"] != cmd.Gap ||
		recovery["idempotencyKey"] != cmd.IdempotencyKey ||
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
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
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(ctx, orchestratorevents.TopicOrchestratorCommandResumeRun, orchestratorevents.ResumeRunCommand{WorkspaceID: workspaceID, RunID: runID})
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	cmd := orchestratorevents.DispatchStepCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Kind:        "llm/workspace_execution",
		Input:       map[string]any{"seed": "yes"},
	}
	if err := module.dispatchRequestedStepCommand(ctx, cmd); err != nil {
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
	if step.State != common.StepStateSucceeded || step.Output["completionState"] != "completed" {
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateExecuting, Input: map[string]any{}}); err != nil {
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
		})

	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	err := module.handleRuntimeResultData(ctx, runtimeevent.TopicRuntimeBlockExecuteCompleted, runtimeevent.CompletedData{
		Completed: runtimemodel.RuntimeStepExecutionCompleted{
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      stepID,
			StepKind:    "llm/workspace_execution",
			RequestID:   "request-1",
			Output:      map[string]any{"status": "ok"},
		},
		Result: runtimemodel.StepExecutionResult{Output: map[string]any{"status": "ok"}},
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
	if step.State != common.StepStateSucceeded || step.Output["status"] != "ok" {
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateExecuting, Input: map[string]any{}}); err != nil {
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
		})

	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	err := module.handleRuntimeResultData(ctx, runtimeevent.TopicRuntimeBlockExecuteFailed, runtimeevent.FailedData{
		Failed: runtimemodel.RuntimeStepExecutionFailed{
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      stepID,
			StepKind:    "llm/workspace_execution",
			RequestID:   "request-1",
			Error:       "provider unavailable",
		},
		Error: "provider unavailable",
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

func TestModuleRuntimeResultEventRequiresWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	module := &Module{
		bizPtr: orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
			moduleNoopRuntimeAdapter{},
			memoryrepo.NewRunRepository(),
			memoryrepo.NewStepRepository(),
			memoryrepo.NewWorkspaceRepository(),
			memoryrepo.NewDecisionTicketRepository(),
			nil,
			memoryrepo.NewApprovalRepository(),
			orchestratorbiz.RuntimeEvidenceRepositories{RuntimeCheckpoints: memoryrepo.NewRuntimeCheckpointRepository()}),
	}

	err := module.handleRuntimeResultData(ctx, runtimeevent.TopicRuntimeBlockExecuteCompleted, runtimeevent.CompletedData{
		Completed: runtimemodel.RuntimeStepExecutionCompleted{
			RunID:  "run-terminal-missing-workspace",
			StepID: "run-terminal-missing-workspace/step-1",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("terminal result without workspace err = %v, want invalid argument", err)
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateExecuting, Input: map[string]any{}}); err != nil {
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
		orchestratorbiz.RuntimeEvidenceRepositories{RuntimeCheckpoints: checkpoints})

	bg := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: orch, backgroundRoutine: bg}

	err := module.handleRuntimeProgressData(ctx, runtimeevent.TopicRuntimeBlockExecuteProgress, runtimeevent.ProgressData{
		Progress: runtimemodel.RuntimeStepExecutionProgressed{
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

func moduleContractDependencies() Dependencies {
	rawHub := magicevent.NewHub(32)
	return Dependencies{
		Runtime:               moduleNoopRuntimeAdapter{},
		Steps:                 memoryrepo.NewStepRepository(),
		ToolCalls:             memoryrepo.NewToolCallRepository(),
		Patches:               memoryrepo.NewPatchRepository(),
		ValidationResults:     memoryrepo.NewValidationResultRepository(),
		RuntimeCheckpoints:    memoryrepo.NewRuntimeCheckpointRepository(),
		EvidenceRecords:       memoryrepo.NewEvidenceRecordRepository(),
		CoverageWorkItems:     memoryrepo.NewCoverageWorkItemRepository(),
		ArtifactContinuations: memoryrepo.NewArtifactContinuationRepository(),
		CommandHub:            rawHub,
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
