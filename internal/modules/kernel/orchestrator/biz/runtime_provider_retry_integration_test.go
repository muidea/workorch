package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeOrchestrator_ResumeDispatchForProviderFailureUsesProviderRetryWithinBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-provider-retry")
	failedStepID := common.StepID(runID + "/llm-provider-step")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-provider-retry"),
		DoDID:           common.DoDID("dod-resume-provider-retry"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want one provider retry spec", specs)
	}
	if specs[0].Input[stepInputKeyProviderRetryFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryAttempt] != 1 ||
		specs[0].Input[stepInputKeyRuntimeRepairFollowUp] == true {
		t.Fatalf("spec input = %+v, want provider retry within budget", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_ResumeDispatchForProviderFailureStopsAfterRetryBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-provider-retry-exhausted")
	failedStepID := common.StepID(runID + "/llm-provider-step")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-provider-retry-exhausted"),
		DoDID:           common.DoDID("dod-resume-provider-retry-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	for attempt := 1; attempt <= maxProviderRetryAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    providerRetryStepID(runID, failedStepID, attempt),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(failedStepID),
				"failedStepId":                    string(failedStepID),
			},
			Output: map[string]any{
				"stage":               "llm_provider_timeout",
				failureOutputKeyClass: failureClassProviderTimeout,
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("nextDispatchSpecsFromStep err = %v, want blocked after provider retry budget", err)
	}
	if len(specs) != 0 {
		t.Fatalf("specs = %+v, want no max+1 provider retry or runtime repair", specs)
	}
}

func TestRuntimeOrchestrator_DispatchNextBlocksProviderFailureAfterRetryBudget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-dispatch-provider-retry-exhausted")
	runID := common.RunID("run-dispatch-provider-retry-exhausted")
	rootStepID := common.StepID(runID + "/llm-provider-step")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-dispatch-provider-retry-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	for attempt := 1; attempt <= maxProviderRetryAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    providerRetryStepID(runID, rootStepID, attempt),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
				"failedStepId":                    string(rootStepID),
			},
			Output: map[string]any{
				"stage":               "llm_provider_timeout",
				failureOutputKeyClass: failureClassProviderTimeout,
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}

	if err := orch.DispatchNextStep(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("DispatchNextStep err = %v, want blocked", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider retry exhaustion decision", runItem)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	for _, step := range steps {
		if step != nil && strings.Contains(string(step.ID), "provider-retry-3") {
			t.Fatalf("steps = %+v, want no provider retry beyond budget", steps)
		}
	}
}

func TestRuntimeOrchestrator_ResumeResolvedProviderRetryExhaustionReblocksWithoutFailing(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
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

	workspaceID := common.WorkspaceID("ws-resolved-provider-exhaustion-reblock")
	runID := common.RunID("run-resolved-provider-exhaustion-reblock")
	rootStepID := common.StepID(runID + "/repair-001-of-planning-001")
	failedStepID := providerRetryStepID(runID, rootStepID, maxProviderRetryAttempts)
	ticketID := common.TicketID(runtimeChildID(failedStepID, "provider-retry-exhausted-decision"))
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: t.TempDir(),
		State:                common.WorkspaceStateWaitingDecision,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            "dod-resolved-provider-exhaustion-reblock",
		State:            common.RunStateBlockedOnDecision,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= maxProviderRetryAttempts; attempt++ {
		stepID := providerRetryStepID(runID, rootStepID, attempt)
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
				stepInputKeyRecoveryAction:        string(runtimeRecoveryActionProviderRetry),
				stepInputKeyRecoveryRootStepID:    string(rootStepID),
				stepInputKeyRecoveryLineageType:   "provider_retry",
			},
			Output: map[string]any{
				"stage":                   "llm_provider_timeout",
				"error":                   "context deadline exceeded",
				failureOutputKeyClass:     failureClassProviderTimeout,
				failureOutputKeySignature: "class=provider_timeout|stage=llm_provider_timeout",
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       failedStepID,
		State:        common.DecisionTicketStateResolved,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
			decisionContextKeyResultPolicy:   decisionContextResultPolicyNone,
		},
	}); err != nil {
		t.Fatalf("seed resolved ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-provider-exhaustion-continue",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "continue"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	err := orch.ResumeRun(ctx, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("ResumeRun err = %v, want blocked on renewed provider decision", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" || runItem.BlockingTicketID == ticketID || runItem.FailureReason != "" {
		t.Fatalf("run = %+v, want reblocked with a new provider ticket and no failure", runItem)
	}
	newTicket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get renewed ticket: %v", err)
	}
	if newTicket.Context[decisionContextKeyResultPolicy] != decisionContextResultPolicyNone ||
		newTicket.Context["previousTicketId"] != string(ticketID) ||
		newTicket.Context["repeatBlocker"] != true {
		t.Fatalf("renewed ticket context = %+v, want repeated provider blocker metadata", newTicket.Context)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %+v, want no business rejected result for provider blocker", results)
	}
	if len(runtime.requests) != 0 {
		t.Fatalf("runtime calls = %d, want no LLM dispatch while provider retry budget remains exhausted", len(runtime.requests))
	}
}

func TestRuntimeOrchestrator_ResumeDispatchForContractFailureUsesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-contract-failure")
	failedStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-contract-failure"),
		DoDID:           common.DoDID("dod-resume-contract-failure"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_execution_graph_plan",
			"error":               "state-changing planning llm step returned read-only executionGraph without patch output",
			failureOutputKeyClass: failureClassRuntime,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want runtime repair spec", specs)
	}
	if specs[0].Input[stepInputKeyProviderRetryFollowUp] == true ||
		specs[0].Input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("spec input = %+v, want runtime repair without provider retry", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_ResumeDispatchForLLMDecisionInvalidUsesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-llm-decision-invalid")
	failedStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-llm-decision-invalid"),
		DoDID:           common.DoDID("dod-resume-llm-decision-invalid"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			"error":               "invalid argument: response must be valid LLM decision JSON",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want runtime repair spec", specs)
	}
	if specs[0].Input[stepInputKeyProviderRetryFollowUp] == true ||
		specs[0].Input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("spec input = %+v, want LLM decision contract repair without provider retry", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_ProviderRetryAttemptCountIgnoresSuccessfulProviderRetries(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-budget-successful-retry")
	rootStepID := common.StepID(runID + "/llm-provider-root")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{"stage": "llm_execution_graph_plan"},
	}); err != nil {
		t.Fatalf("seed successful retry: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 2),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  2,
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}); err != nil {
		t.Fatalf("seed failed retry: %v", err)
	}

	if got := orch.repairManager().providerRetryAttemptCount(ctx, runID, rootStepID); got != 1 {
		t.Fatalf("providerRetryAttemptCount = %d, want only failed provider retry attempts", got)
	}
}

func TestRuntimeOrchestrator_ProviderProtocolInvalidUsesProviderRetry(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-protocol-retry")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   stepRepo,
		policy: NormalizeRuntimePolicy(RuntimePolicy{
			MaxProviderRetryAttempts: 3,
		}),
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-provider-protocol-retry",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failed := &runmodel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":                   "llm_provider_protocol_invalid",
			"error":                   "invalid argument: llm provider protocol invalid: stream error: stream ID 191; INTERNAL_ERROR; received from peer",
			failureOutputKeyClass:     failureClassProviderProtocolInvalid,
			failureOutputKeySignature: "class=provider_protocol_invalid|kind=llm/workspace_execution|stage=llm_provider_protocol_invalid",
		},
	}
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	action := orch.recoveryActionForFailedStep(ctx, run, failed)
	if action != runtimeRecoveryActionProviderRetry {
		t.Fatalf("recoveryActionForFailedStep = %q, want %q", action, runtimeRecoveryActionProviderRetry)
	}
	input := providerRetryStepInputWithMaxAttempts(failed, 1, errors.New("stream error: stream ID 191; INTERNAL_ERROR; received from peer"), maxProviderRetryAttemptsForPolicy(orch.policy))
	if input[stepInputKeyProviderRetryFollowUp] != true || input[stepInputKeyProviderRetryAttempt] != 1 {
		t.Fatalf("provider retry input = %+v, want retry attempt 1", input)
	}
	scope := mapFromValue(input[stepInputKeyRecoveryBudgetScope])
	if scope["failureClass"] != failureClassProviderProtocolInvalid {
		t.Fatalf("recovery budget scope = %+v, want provider protocol class", scope)
	}
}

func TestReplanContinuationIgnoresContextGateWrapperAsPrimaryFailure(t *testing.T) {
	continuation := map[string]any{
		stepInputKeyCurrentProgressDigest: map[string]any{
			"currentGap": "invalid argument: llm context gate fail_fast: repair_action_package_required_output_missing",
			"lastObservedFailure": map[string]any{
				"stepId":                  "run-context-wrapper/repair-001",
				"kind":                    llmWorkspaceExecutionStepKind,
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_context_integrity_failed|error=invalid argument: llm context gate fail_fast: repair_action_package_required_output_missing",
				"failureReason":           "invalid argument: llm context gate fail_fast: repair_action_package_required_output_missing",
				"evidenceValidity":        "actionable_failure",
			},
		},
		stepInputKeyProcessEvidenceTimeline: []map[string]any{
			{
				"stepId":                  "run-context-wrapper/repair-001",
				"kind":                    llmWorkspaceExecutionStepKind,
				"state":                   string(common.StepStateFailed),
				"stage":                   "llm_context_integrity_failed",
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_context_integrity_failed",
				"failureReason":           "invalid argument: llm context gate fail_fast: repair_action_package_required_output_missing",
				"evidenceValidity":        "actionable_failure",
			},
		},
	}

	if primary := primaryActionableFailureFromReplanContinuation(continuation); len(primary) > 0 {
		t.Fatalf("primaryActionableFailureFromReplanContinuation = %+v, want context gate wrapper ignored", primary)
	}
	digest := currentProgressDigestFromPrimaryActionableFailure(map[string]any{
		"failedStepId":            "run-context-wrapper/repair-001",
		"failedStepKind":          llmWorkspaceExecutionStepKind,
		failureOutputKeyClass:     failureClassRuntime,
		failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_context_integrity_failed",
		"reason":                  "invalid argument: llm context gate fail_fast: repair_action_package_required_output_missing",
		"evidenceValidity":        "actionable_failure",
	})
	if len(digest) > 0 {
		t.Fatalf("currentProgressDigestFromPrimaryActionableFailure = %+v, want context gate wrapper ignored", digest)
	}
}

func TestRuntimeOrchestrator_ProviderRetryInputDoesNotCarryRuntimeRepairMarkers(t *testing.T) {
	rootStepID := common.StepID("run-provider-retry-input/root")
	input := providerRetryStepInput(&runmodel.Step{
		ID:    rootStepID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  3,
			"previousStepId":                  "previous",
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}, 1, errors.New("provider timeout"))

	if input[stepInputKeyProviderRetryFollowUp] != true ||
		input[stepInputKeyRuntimeRepairFollowUp] != nil ||
		input[stepInputKeyRuntimeRepairAttempt] != nil {
		t.Fatalf("provider retry input = %+v, want provider markers without runtime repair markers", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageRepair {
		t.Fatalf("provider retry runtimeStage = %v, want repair", input[stepInputKeyRuntimeStage])
	}
	if input[stepInputKeyRecoveryAction] != string(runtimeRecoveryActionProviderRetry) ||
		input[stepInputKeyRecoveryRootStepID] != string(rootStepID) ||
		input[stepInputKeyRecoveryLineageType] != "provider_retry" {
		t.Fatalf("provider retry lineage = %+v, want explicit recovery lineage", input)
	}
	scope := mapFromValue(input[stepInputKeyRecoveryBudgetScope])
	if scope["lineageType"] != "provider_retry" || scope["rootStepId"] != string(rootStepID) || scope["failureClass"] != failureClassProviderTimeout {
		t.Fatalf("provider retry budget scope = %+v, want provider retry root scope", scope)
	}
}

func TestProviderRetryStepInputKeepsStateChangingRepairContract(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-state-changing-repair/repair-001-of-planning-001",
		RunID: "run-provider-retry-state-changing-repair",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    "run-provider-retry-state-changing-repair/step-1",
			stepInputKeyRepairOfStepID:        "run-provider-retry-state-changing-repair/step-1",
			"repairObjective":                 "produce_state_changing_output",
			"stateChangingOutputRequired":     true,
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "context deadline exceeded",
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))
	if input[stepInputKeyRuntimeRepairFollowUp] != nil || input[stepInputKeyRuntimeRepairAttempt] != nil {
		t.Fatalf("provider retry input = %+v, want retry markers without runtime repair dispatch markers", input)
	}
	if input["repairObjective"] != "produce_state_changing_output" ||
		input["stateChangingOutputRequired"] != true ||
		len(stringSliceFromValue(input["repairGuidance"])) == 0 {
		t.Fatalf("provider retry input = %+v, want state-changing repair contract preserved", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageRepair {
		t.Fatalf("provider retry runtimeStage = %v, want repair", input[stepInputKeyRuntimeStage])
	}
	if input[stepInputKeyRepairOfRootStepID] != "run-provider-retry-state-changing-repair/step-1" ||
		input[stepInputKeyRepairOfStepID] != "run-provider-retry-state-changing-repair/step-1" {
		t.Fatalf("provider retry repair lineage = %+v, want repair scope preserved", input)
	}
}

func TestRuntimePolicyConfiguresProviderRetryAttempts(t *testing.T) {
	if got := NormalizeRuntimePolicy(RuntimePolicy{}).MaxRuntimeRepairAttempts; got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("default max runtime repair attempts = %d, want %d", got, defaultMaxRuntimeRepairAttempts)
	}
	if got := NormalizeRuntimePolicy(RuntimePolicy{}).MaxProviderRetryAttempts; got != maxProviderRetryAttempts {
		t.Fatalf("default max provider retry attempts = %d, want %d", got, maxProviderRetryAttempts)
	}
	if got := NormalizeRuntimePolicy(RuntimePolicy{}).LLMOutput.MaxGraphBytes; got != defaultMaxGraphBytes {
		t.Fatalf("default max graph bytes = %d, want %d", got, defaultMaxGraphBytes)
	}
	orch := &RuntimeOrchestrator{
		policy: NormalizeRuntimePolicy(RuntimePolicy{
			MaxRuntimeRepairAttempts: 9,
			MaxProviderRetryAttempts: 7,
			LLMOutput: LLMOutputRuntimePolicy{
				MaxPatchContentBytes: 4096,
				MaxGraphBytes:        16384,
			},
		}),
	}
	repairInput := orch.runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{
		ID:          "run-policy",
		WorkspaceID: "ws-policy",
		DoDID:       "dod-policy",
	}, &runmodel.Step{
		ID:    "run-policy/failed-step",
		RunID: "run-policy",
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     "shell_command_failed",
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|command=go|exitCode=1|error=exit status 1",
		},
	}, 1, errors.New("exit status 1"))
	if got := repairInput["maxRuntimeRepairAttempts"]; got != 9 {
		t.Fatalf("maxRuntimeRepairAttempts = %v, want configured 9", got)
	}
	input := providerRetryStepInputWithMaxAttempts(&runmodel.Step{
		ID:    "run-policy/step-1",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":                   "llm_provider_timeout",
			failureOutputKeyClass:     failureClassProviderTimeout,
			failureOutputKeySignature: "provider-timeout",
		},
	}, 1, errors.New("context deadline exceeded"), maxProviderRetryAttemptsForPolicy(orch.policy))
	if got := input["maxProviderRetryAttempts"]; got != 7 {
		t.Fatalf("maxProviderRetryAttempts = %v, want configured 7", got)
	}
	if got := llmOutputPolicyValues(orch.policy)["maxPatchContentBytes"]; got != 4096 {
		t.Fatalf("llmOutputPolicy.maxPatchContentBytes = %v, want configured 4096", got)
	}
}

func TestProviderRetryStepInputNarrowsStateChangingProviderTimeout(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-state-changing-timeout/step-1",
		RunID: "run-provider-retry-state-changing-timeout",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failedExecutionGraph":            map[string]any{"nodes": []any{map[string]any{"kind": "shell/command"}}},
			stepInputKeyGraphEvidence:         []map[string]any{{"nodeId": "inspect", "output": map[string]any{"stdout": strings.Repeat("x", 2048)}}},
			stepInputKeyWorkspaceFileManifest: []map[string]any{{"path": "README.md"}},
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			"status":              "failed",
			"error":               "context deadline exceeded",
			failureOutputKeyClass: failureClassProviderTimeout,
			"contextBudget":       map[string]any{"actualBytes": 9999},
			"llmUsage":            map[string]any{"inputTokens": 1000},
			"provider":            map[string]any{"baseUrl": "http://127.0.0.1:8090/v1"},
			"streamDiagnostics":   map[string]any{"streamIncomplete": true, "doneReceived": false},
			"streamEvents": []map[string]any{
				{"type": "delta", "delta": "{\"completionState\":\"continue\",\"executionGraph\":{\"nodes\":[{\"id\":\"write_go_mod\""},
			},
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))

	if input["stateChangingRecoveryMode"] != "direct_state_change" ||
		input["providerTimeoutStateChangingRetry"] != true {
		t.Fatalf("provider retry input = %+v, want direct state-changing recovery mode", input)
	}
	for _, key := range []string{"failedExecutionGraph", stepInputKeyGraphEvidence, stepInputKeyWorkspaceFileManifest, "previousStepOutput"} {
		if _, ok := input[key]; ok {
			t.Fatalf("provider retry input retained %s: %+v", key, input[key])
		}
	}
	failedOutput := mapFromValue(input["failedStepOutput"])
	if failedOutput["contextBudget"] != nil || failedOutput["llmUsage"] != nil || failedOutput["provider"] != nil {
		t.Fatalf("failedStepOutput = %+v, want provider timeout summary without heavy provider payload", failedOutput)
	}
	if !strings.Contains(fmt.Sprint(failedOutput["partialOutputPreview"]), "write_go_mod") {
		t.Fatalf("partialOutputPreview = %q, want partial state-changing output hint", failedOutput["partialOutputPreview"])
	}
	forbidden := stringSliceFromValue(input["forbiddenRecoveryOutputs"])
	if !containsString(forbidden, "run_snapshot_graph") || !containsString(forbidden, "context_rehydration_graph") {
		t.Fatalf("forbiddenRecoveryOutputs = %+v, want self-inspection outputs forbidden", forbidden)
	}
}

func TestProviderRetryStepInputRestoresStateChangingContractFromMissingDoDRepair(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-dod-gap-repair/repair-001-of-planning-001",
		RunID: "run-provider-retry-dod-gap-repair",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    "run-provider-retry-dod-gap-repair/step-1",
			stepInputKeyRepairOfStepID:        "run-provider-retry-dod-gap-repair/assessment",
			"missingDoDItems": []map[string]any{
				{"item": "produce state-changing result", "state": "failed"},
			},
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "context deadline exceeded",
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))
	if input["repairObjective"] != "produce_state_changing_output" ||
		input["stateChangingOutputRequired"] != true ||
		len(stringSliceFromValue(input["repairGuidance"])) == 0 {
		t.Fatalf("provider retry input = %+v, want state-changing contract restored from missing DoD repair", input)
	}
	if len(mapSliceFromValue(input["missingDoDItems"])) != 1 {
		t.Fatalf("missingDoDItems = %+v, want missing DoD carried into provider retry", input["missingDoDItems"])
	}
}

func TestProviderRetryStepInputRestoresStateChangingContractFromGatedGraphFailure(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-gated-repair/repair-001-of-planning-001",
		RunID: "run-provider-retry-gated-repair",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    "run-provider-retry-gated-repair/step-1",
			stepInputKeyRepairOfStepID:        "run-provider-retry-gated-repair/step-1",
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "invalid argument: state-changing executionGraph patch/apply depends on llm/workspace_execution before patch",
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("invalid argument: state-changing executionGraph patch/apply depends on llm/workspace_execution before patch"))
	if input["repairObjective"] != "produce_state_changing_output" || input["stateChangingOutputRequired"] != true {
		t.Fatalf("provider retry input = %+v, want state-changing contract restored from gated graph failure", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageRepair {
		t.Fatalf("provider retry runtimeStage = %v, want repair", input[stepInputKeyRuntimeStage])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairBudgetCountsProviderTimeoutRepairDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-budget-provider-timeout")
	failedStepID := common.StepID(runID + "/graph-001/005-implement")
	signature := "class=runtime_failed|targetRef=main.go"
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}

	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    runtimeRepairFollowUpStepID(runID, &runmodel.Step{ID: failedStepID}, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfStepID:        string(failedStepID),
			stepInputKeyRepairOfRootStepID:    string(failedStepID),
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "context deadline exceeded",
		},
	}); err != nil {
		t.Fatalf("seed provider-timeout repair step: %v", err)
	}

	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
		},
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 1 {
		t.Fatalf("runtimeRepairAttemptCountForStep = %d, want provider-timeout repair dispatch counted", got)
	}
}

func TestProviderRetryRootStepIDDoesNotUseRepairFailedStepIDForNonRetryStep(t *testing.T) {
	repairStepID := common.StepID("run-provider-retry-root/zz-llm-repair-root-repair-1")
	step := &runmodel.Step{
		ID:   repairStepID,
		Kind: llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"failedStepId":                    "run-provider-retry-root/step-1",
		},
	}

	if got := providerRetryRootStepID(step); got != repairStepID {
		t.Fatalf("providerRetryRootStepID = %q, want repair step id %q", got, repairStepID)
	}

	input := providerRetryStepInput(step, 1, errors.New("provider timeout"))
	if got := input[stepInputKeyProviderRetryRootStep]; got != string(repairStepID) {
		t.Fatalf("providerRetryRootStepId = %v, want %s", got, repairStepID)
	}
}

func TestRuntimeOrchestrator_ProviderRetryKeepsRootAcrossRetryFailure(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-root-normalized")
	rootStepID := common.StepID("run-provider-retry-root-normalized/zz-llm-repair-root-repair-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}); err != nil {
		t.Fatalf("seed retry step: %v", err)
	}
	failedRetry := &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}

	if got := providerRetryRootStepID(failedRetry); got != rootStepID {
		t.Fatalf("providerRetryRootStepID = %q, want root %q", got, rootStepID)
	}
	if got := orch.repairManager().providerRetryAttemptCount(ctx, runID, providerRetryRootStepID(failedRetry)); got != 1 {
		t.Fatalf("providerRetryAttemptCount = %d, want prior retry counted for same root", got)
	}
	nextID := providerRetryStepID(runID, providerRetryRootStepID(failedRetry), 2)
	if strings.Contains(string(nextID), "provider-retry-1-provider-retry-2") {
		t.Fatalf("provider retry step id = %q, want non-nested retry id", nextID)
	}
	if want := providerRetryStepID(runID, rootStepID, 2); nextID != want {
		t.Fatalf("provider retry step id = %q, want %q", nextID, want)
	}
}

func TestProviderRetryRootStepIDNormalizesRepairOfProviderRetryShell(t *testing.T) {
	runID := common.RunID("run-provider-retry-repair-shell-root")
	businessRootID := common.StepID(runID + "/step-1")
	retryStepID := providerRetryStepID(runID, common.StepID(runID+"/repair-001-of-planning-001"), 1)
	repairShellID := common.StepID(runID + "/repair-001-of-provider-retry-001")
	step := &runmodel.Step{
		ID:    repairShellID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(retryStepID),
			stepInputKeyRepairOfRootStepID:    string(businessRootID),
			stepInputKeyRecoveryRootStepID:    string(businessRootID),
			"failedStepId":                    string(retryStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}

	if got := providerRetryRootStepID(step); got != businessRootID {
		t.Fatalf("providerRetryRootStepID = %q, want business root %q", got, businessRootID)
	}
	if got := runtimeRepairRootStepID(step); got != businessRootID {
		t.Fatalf("runtimeRepairRootStepID = %q, want business root %q", got, businessRootID)
	}
	nextID := providerRetryStepID(runID, providerRetryRootStepID(step), 1)
	if strings.Contains(string(nextID), "of-provider-retry-") {
		t.Fatalf("provider retry id = %q, want normalized readable root without provider retry shell", nextID)
	}
}

func TestRuntimeRepairFollowUpStepIDUsesBusinessRootForProviderRetryContractViolation(t *testing.T) {
	runID := common.RunID("run-repair-id-contract")
	businessRootID := common.StepID(runID + "/step-1")
	retryStepID := providerRetryStepID(runID, businessRootID, 1)
	failedRetry := &runmodel.Step{
		ID:    retryStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(businessRootID),
			stepInputKeyRepairOfRootStepID:    string(businessRootID),
		},
		Output: map[string]any{
			"stage":               "llm_execution_graph_plan",
			failureOutputKeyClass: failureClassRuntime,
		},
	}

	stepID := runtimeRepairFollowUpStepID(runID, failedRetry, 1)
	if strings.Contains(strings.TrimPrefix(string(stepID), string(runID)+"/"), "provider-retry") {
		t.Fatalf("runtime repair step id = %q, want business root label without provider retry shell", stepID)
	}
	if !strings.Contains(string(stepID), "planning-001") {
		t.Fatalf("runtime repair step id = %q, want business root label", stepID)
	}
}

func TestRuntimeRepairFollowUpStepIDUsesGraphNodeForExecutionFailure(t *testing.T) {
	runID := common.RunID("run-repair-id-graph-failure")
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-003/003-run-go-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:         runtimeStageExecution,
			stepInputKeyGraphNodeID:          "run_go_test",
			stepInputKeyRepairOfRootStepID:   string(runID) + "/step-1",
			stepInputKeyRepairOfFailureSig:   "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepInputKeyRepairOfFailureClass: failureClassDoDCoverageMissing,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run_go_test|exitCode=1",
			"error":                   "exit status 1",
		},
	}

	stepID := runtimeRepairFollowUpStepID(runID, failedStep, 1)
	if !strings.Contains(string(stepID), "graph-003-003-run-go-test") {
		t.Fatalf("runtime repair step id = %q, want failed graph node label", stepID)
	}
	if strings.Contains(string(stepID), "planning-001-2") || strings.HasSuffix(string(stepID), "planning-001") {
		t.Fatalf("runtime repair step id = %q, want graph failure label instead of planning root label", stepID)
	}
}

func TestRuntimeRepairFollowUpStepIDKeepsGraphNodeForSecondOrderRepair(t *testing.T) {
	runID := common.RunID("run-repair-id-second-order")
	originalGraphStepID := common.StepID(runID + "/graph-004/001-run-stream-test-baseline")
	failedRepair := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-graph-004-001-run-stream-test-baseline"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(originalGraphStepID),
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run-stream-test|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}

	stepID := runtimeRepairFollowUpStepID(runID, failedRepair, 2)
	if !strings.Contains(string(stepID), "graph-004-001-run-stream-test-baseline") {
		t.Fatalf("runtime repair step id = %q, want original graph node label", stepID)
	}
	if strings.Contains(string(stepID), "planning-001") {
		t.Fatalf("runtime repair step id = %q, want graph repair label instead of planning root label", stepID)
	}
}

func TestRuntimeRepairFollowUpStepIDKeepsEmbeddedGraphNodeForRepairOfRepair(t *testing.T) {
	runID := common.RunID("run-repair-id-embedded-graph-lineage")
	failedRepair := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-graph-001-002-run-go-test-2"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(runID) + "/repair-001-of-graph-001-002-run-go-test",
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
			"error":                   "invalid argument: invalid llm decision json",
		},
	}

	stepID := runtimeRepairFollowUpStepID(runID, failedRepair, 2)
	if !strings.Contains(string(stepID), "graph-001-002-run-go-test") {
		t.Fatalf("runtime repair step id = %q, want embedded original graph label", stepID)
	}
	if strings.Contains(string(stepID), "planning-001") {
		t.Fatalf("runtime repair step id = %q, want graph repair label instead of planning root label", stepID)
	}
}

func TestRuntimeRepairInputForSchemaInvalidRepairAddsJSONRecoveryContract(t *testing.T) {
	runID := common.RunID("run-schema-invalid-repair-contract")
	failedRepair := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-graph-001-002-run-go-test"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(runID) + "/graph-001/002-run-go-test",
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":        "main_test.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash",
				"content":          "package main\n",
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
			"error":                   "invalid argument: invalid llm decision json: invalid character '}' after array element",
			"reason":                  "invalid argument: invalid llm decision json: invalid character '}' after array element",
		},
	}

	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-schema-invalid-repair-contract", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failedRepair, 2, errors.New("invalid argument: invalid llm decision json"))

	if input["schemaInvalidRepairMode"] != "single_valid_json_decision" {
		t.Fatalf("schemaInvalidRepairMode = %v, want single_valid_json_decision", input["schemaInvalidRepairMode"])
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		t.Fatalf("repairActionPackage missing from input: %+v", input)
	}
	if recovery := mapFromValue(pkg["schemaInvalidRecovery"]); recovery["required"] != true {
		t.Fatalf("schemaInvalidRecovery = %+v, want required", recovery)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustReturnValidJSON"] != true || required["mustNotReuseRejectedJSON"] != true {
		t.Fatalf("requiredOutput = %+v, want schema invalid JSON constraints", required)
	}
	if required["mustPatchDirectlyFromProvidedSnapshot"] != true ||
		!strings.Contains(stringFromAny(required["sameGraphReadPatchRule"]), "do not put file/read before patch/apply") {
		t.Fatalf("requiredOutput = %+v, want direct patch from provided snapshot contract", required)
	}
	if guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"); !strings.Contains(guidance, "single valid JSON") {
		t.Fatalf("repairGuidance = %+v, want schema invalid JSON guidance", input["repairGuidance"])
	}
}

func TestRuntimeRepairInputForReadBeforePatchContractFailurePreservesDirectPatchContract(t *testing.T) {
	runID := common.RunID("run-read-before-patch-contract-repair")
	failedRepair := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-graph-001-014-build"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(runID) + "/graph-001/014-build",
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":        "main.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-main",
				"content":          "package main\n",
			}},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main.go",
					"exists":           true,
					"contentComplete":  true,
					"safeForFileWrite": true,
					"contentHash":      "hash-main",
					"content":          "package main\n",
				}},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
			"error":                   "invalid argument: executionGraph patch/apply must not depend on same-graph file/read; read current content in an earlier turn and submit a separate state-changing graph",
			"reason":                  "invalid argument: executionGraph patch/apply must not depend on same-graph file/read; read current content in an earlier turn and submit a separate state-changing graph",
		},
	}

	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-read-before-patch-contract", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failedRepair, 2, common.ErrInvalidArgument)

	if input["stateChangingOutputRequired"] != true || input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("input = %+v, want state-changing repair preserved", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		required["mustPatchDirectlyFromProvidedSnapshot"] != true {
		t.Fatalf("requiredOutput = %+v, want direct state-changing patch contract", required)
	}
	if !strings.Contains(stringFromAny(required["sameGraphReadPatchRule"]), "do not put file/read before patch/apply") {
		t.Fatalf("sameGraphReadPatchRule = %q, want explicit same-graph read patch rule", required["sameGraphReadPatchRule"])
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "file/read") {
		t.Fatalf("acceptedKinds = %+v, should not accept file/read for state-changing repair", required["acceptedKinds"])
	}
	if guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"); !strings.Contains(guidance, "single valid JSON") {
		t.Fatalf("repairGuidance = %+v, want schema invalid JSON guidance", input["repairGuidance"])
	}
}

func TestRuntimeRepairInputForSchemaInvalidPreservesConcreteValidationContract(t *testing.T) {
	runID := common.RunID("run-schema-invalid-validation-contract")
	failedRepair := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-verify"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "dod",
				"targetText": "verify external contract",
			}},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"exists":           true,
					"contentComplete":  true,
					"safeForFileWrite": true,
					"contentHash":      "hash-target",
					"source":           "concrete_failure_evidence",
				}},
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
					"forbiddenKinds":                        []string{"patch/apply"},
				},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
			"error":                   "invalid argument: invalid llm decision json: invalid character '0' in string escape code",
		},
	}

	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-schema-invalid-validation-contract", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failedRepair, 2, errors.New("invalid argument: invalid llm decision json"))

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		input["boundedValidationRequired"] != true ||
		input["evidenceRepairRequired"] != true ||
		input["concreteFailureEvidenceRequired"] == true {
		t.Fatalf("input = %+v, want concrete validation evidence contract preserved", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		required["mustReturnValidJSON"] != true ||
		required["mustNotReuseRejectedJSON"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want validation evidence plus schema recovery contract", required)
	}
	if targets := mapSliceFromValue(input["coverageTargetSet"]); len(targets) != 1 {
		t.Fatalf("coverageTargetSet = %+v, want inherited validation target", input["coverageTargetSet"])
	}
}

func TestRuntimeRepairInputCarriesCoverageTargetsAndProgressAfterRepairContractFailure(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-coverage-target-context-inherit")
	steps := memory.NewStepRepository()
	if err := steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/graph-002/001-patch-target"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "patch-target",
			stepInputKeyGraphRootStepID: string(runID) + "/repair-001-of-planning-001-3",
		},
		Output: map[string]any{
			"status": "completed",
			"patches": []map[string]any{{
				"targetRef": "main.go",
				"patchType": "unified_diff",
				"content":   "large patch body should not be repeated in progress",
			}},
		},
	}); err != nil {
		t.Fatalf("seed patch step: %v", err)
	}
	if err := steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/graph-002/002-run-tests"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "run-tests",
			stepInputKeyGraphRootStepID: string(runID) + "/repair-001-of-planning-001-3",
		},
		Output: map[string]any{
			"status":   "completed",
			"command":  "test",
			"args":     []string{"all"},
			"exitCode": 0,
			"stdout":   "ok target suite",
		},
	}); err != nil {
		t.Fatalf("seed test step: %v", err)
	}
	failedRepair := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-planning-001-4"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRepairOfStepID:        string(runID) + "/follow-up-graph-repair-001-of-planning-001-3-run-build",
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"missingDoDItems": []map[string]any{{
				"item":  "streaming responses must be directly verified",
				"state": "failed",
				"evidenceCandidates": []map[string]any{{
					"stepId": string(runID) + "/graph-002/002-run-tests",
					"kind":   "shell/command",
					"state":  "completed",
				}},
			}},
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-001",
				"raw":               "token accounting must be covered",
				"state":             "failed",
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=coverage-targets",
			"error":                   "invalid argument: coverage-gap state-changing repair patch/apply must declare coverageTargets matching missing DoD or Goal coverage items",
			"executionGraph": map[string]any{"nodes": []map[string]any{{
				"id":   "patch-without-coverage-targets",
				"kind": "patch/apply",
			}}},
		},
	}

	input := (&RuntimeOrchestrator{
		steps:      steps,
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-coverage-target-context-inherit", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failedRepair, 2, common.ErrInvalidArgument)

	if missing := mapSliceFromValue(input["missingDoDItems"]); len(missing) != 1 || missing[0]["item"] != "streaming responses must be directly verified" {
		t.Fatalf("missingDoDItems = %+v, want inherited unresolved DoD target", input["missingDoDItems"])
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || missing[0]["goalRequirementId"] != "goal-001" {
		t.Fatalf("missingGoalCoverage = %+v, want inherited unresolved goal target", input["missingGoalCoverage"])
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if pkg["coverageRepairRequired"] != true {
		t.Fatalf("repairActionPackage = %+v, want coverageRepairRequired", pkg)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustDeclareCoverageTargets"] != true {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want mustDeclareCoverageTargets", required)
	}
	if targets := mapSliceFromValue(input["coverageTargetSet"]); len(targets) != 2 {
		t.Fatalf("coverageTargetSet = %+v, want top-level DoD and Goal targets", input["coverageTargetSet"])
	}
	targets := mapSliceFromValue(pkg["coverageTargetSet"])
	if len(targets) != 2 {
		t.Fatalf("coverageTargetSet = %+v, want DoD and Goal targets", targets)
	}
	progress := mapFromValue(pkg["currentExecutionProgress"])
	if int64FromValue(progress["coverageGapCount"]) != 2 {
		t.Fatalf("currentExecutionProgress = %+v, want coverage gap count", progress)
	}
	if latest := mapSliceFromValue(progress["latestActionEvidence"]); len(latest) == 0 {
		t.Fatalf("currentExecutionProgress = %+v, want latest action evidence", progress)
	}
	if strings.Contains(fmt.Sprint(progress), "large patch body should not be repeated") {
		t.Fatalf("currentExecutionProgress leaked patch body: %+v", progress)
	}
}

func TestRuntimeRepairLatestActionEvidenceUsesExplicitSequenceOrder(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-context-order")
	steps := memory.NewStepRepository()
	for _, seed := range []struct {
		id       string
		seq      int
		nodeID   string
		expected bool
	}{
		{id: "graph-002/001-older-by-sequence", seq: 2, nodeID: "older"},
		{id: "graph-001/001-newer-by-sequence", seq: 3, nodeID: "newer", expected: true},
	} {
		if err := steps.Create(ctx, &runmodel.Step{
			ID:    common.StepID(string(runID) + "/" + seed.id),
			RunID: runID,
			Kind:  "shell/command",
			State: common.StepStateCompleted,
			Input: map[string]any{
				"stepSequence":              seed.seq,
				stepInputKeyGraphNodeID:     seed.nodeID,
				stepInputKeyGraphRootStepID: string(runID) + "/step-1",
			},
			Output: map[string]any{
				"status":   "completed",
				"command":  "test",
				"exitCode": 0,
			},
		}); err != nil {
			t.Fatalf("seed step: %v", err)
		}
	}
	orch := &RuntimeOrchestrator{steps: steps}
	evidence := orch.latestActionEvidenceCandidates(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-context-order"}, nil, 2)
	if len(evidence) != 2 {
		t.Fatalf("latestActionEvidenceCandidates = %+v, want two items", evidence)
	}
	if evidence[0]["graphNodeId"] != "newer" {
		t.Fatalf("latestActionEvidenceCandidates = %+v, want explicit sequence order", evidence)
	}
}

func TestRuntimeOrchestrator_ProviderUnavailableBlocksOnProviderDecisionWithoutRejectedResult(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-unavailable-decision")
	workspaceID := common.WorkspaceID("ws-provider-unavailable-decision")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resultRepo := memory.NewResultRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		nil,
		RuntimeEvidenceRepositories{Results: resultRepo},
		nil,
	)
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-provider-unavailable-decision",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":               "llm_provider_unavailable",
			"error":               "auth_unavailable: no auth available",
			failureOutputKeyClass: failureClassProviderUnavailable,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("create failed step: %v", err)
	}

	err := orch.DispatchNextStep(ctx, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("DispatchNextStep err = %v, want blocked", err)
	}
	updated, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.State != common.RunStateBlockedOnDecision || updated.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider decision", updated)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %+v, want no business rejected result for provider config blocker", results)
	}
	ticket, err := ticketRepo.Get(ctx, updated.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Context[decisionContextKeyScope] != decisionContextScopeWorkorchRuntime ||
		ticket.Context[decisionContextKeyResultPolicy] != decisionContextResultPolicyNone {
		t.Fatalf("ticket context = %+v, want provider config resultPolicy none", ticket.Context)
	}
}

func TestRuntimeOrchestrator_ProviderRetryContractInvalidRoutesToRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-contract-invalid")
	rootStepID := common.StepID(runID + "/follow-up-001/dod-gate")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-provider-retry-contract-invalid"),
		DoDID:           common.DoDID("dod-provider-retry-contract-invalid"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedRetry := &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			"graphFollowUp":                   true,
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
			stepInputKeyRecoveryAction:        string(runtimeRecoveryActionProviderRetry),
			stepInputKeyRecoveryRootStepID:    string(rootStepID),
			stepInputKeyRecoveryLineageType:   "provider_retry",
			stepInputKeyRecoveryBudgetScope: map[string]any{
				"lineageType": "provider_retry",
				"rootStepId":  string(rootStepID),
			},
		},
		Output: map[string]any{
			"stage":  "llm_schema_invalid",
			"status": "failed",
			"error":  "invalid argument: executionGraph patch/apply targetRef is required",
		},
	}
	attachRuntimeFailureClassification(failedRetry.Output, classifyRuntimeFailure(failedRetry, failedRetry.Output, "runtime step failed", errors.New("invalid argument: executionGraph patch/apply targetRef is required")))
	if err := stepRepo.Create(ctx, failedRetry); err != nil {
		t.Fatalf("seed failed retry: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedRetry)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want runtime repair spec", specs)
	}
	if specs[0].Input[stepInputKeyRuntimeRepairFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryFollowUp] == true {
		t.Fatalf("spec input = %+v, want runtime repair without provider retry", specs[0].Input)
	}
	if specs[0].Input[stepInputKeyRepairOfRootStepID] != string(rootStepID) {
		t.Fatalf("repair root = %v, want provider retry root %s", specs[0].Input[stepInputKeyRepairOfRootStepID], rootStepID)
	}
	if specs[0].Input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		specs[0].Input["stateChangingOutputRequired"] != nil ||
		specs[0].Input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("repair input = %+v, want targetRef contract wrapper failure to acquire concrete target evidence", specs[0].Input)
	}
}

func TestRuntimeFollowUpIDsUseReadableLabelsInsteadOfFullLineage(t *testing.T) {
	runID := common.RunID("run-stable-lineage-id")
	lineageStepID := common.StepID("run-stable-lineage-id/follow-up-graph-repair-001-of-provider-retry-001-of-planning-001-build-check")
	step := &runmodel.Step{
		ID: lineageStepID,
		Input: map[string]any{
			stepInputKeyGraphNodeID: "build_check",
			"graphRootStepId":       "run-stable-lineage-id/repair-001-of-provider-retry-001-of-planning-001",
		},
	}

	ids := []common.StepID{
		providerRetryStepID(runID, lineageStepID, 1),
		runtimeRepairFollowUpStepID(runID, step, 1),
		graphFollowUpStepID(runID, step),
		assessmentOnlyContractCorrectionStepID(runID, step),
		llmCompletionFollowUpStepID(runID, step),
	}
	for _, id := range ids {
		if !strings.HasPrefix(string(id), string(runID)+"/") {
			t.Fatalf("step id %q is not run-scoped", id)
		}
		if strings.Count(string(id), "provider-retry-") > 1 {
			t.Fatalf("step id %q retained full provider retry lineage", id)
		}
		if strings.Contains(string(id), "provider-retry-001-of-planning-001-build-check") {
			t.Fatalf("step id %q retained full provider retry ancestor context", id)
		}
		if strings.Contains(string(id), "step-") && strings.Contains(string(id), "provider-retry") {
			t.Fatalf("step id %q retained hash-like provider retry id", id)
		}
	}
}

func TestRuntimeOrchestrator_DuplicateRuntimeFailureBelowScopedBudgetDispatchesRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-duplicate-failure")
	runID := common.RunID("run-duplicate-failure")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{stepInputKeyRuntimeRepairFollowUp: true},
	}); err != nil {
		t.Fatalf("seed repair step: %v", err)
	}
	previousFailed := duplicateShellFailureStep(runID, "previous-shell-failure")
	if err := stepRepo.Create(ctx, previousFailed); err != nil {
		t.Fatalf("seed previous failed step: %v", err)
	}
	currentFailed := duplicateShellFailureStep(runID, "current-shell-failure")

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}, currentFailed, errors.New("exit status 1")); err != nil {
		t.Fatalf("duplicate failure below scoped repair budget should dispatch repair: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want duplicate failure to continue repair before scoped budget is exhausted", runItem)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	foundRepair := false
	for _, step := range steps {
		if step != nil && boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) && step.ID != common.StepID(string(runID)+"/repair-1") {
			foundRepair = true
			break
		}
	}
	if !foundRepair {
		t.Fatalf("steps = %+v, want runtime repair follow-up", steps)
	}
}

func TestRuntimeOrchestrator_DuplicateRuntimeFailureExhaustedScopeUsesRepairExhaustion(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-duplicate-failure-exhausted")
	runID := common.RunID("run-duplicate-failure-exhausted")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	previousFailed := duplicateShellFailureStep(runID, "previous-shell-failure")
	if err := stepRepo.Create(ctx, previousFailed); err != nil {
		t.Fatalf("seed previous failed step: %v", err)
	}
	currentFailed := duplicateShellFailureStep(runID, "current-shell-failure")
	signature := stringFromMap(currentFailed.Output, failureOutputKeySignature)
	class := stringFromMap(currentFailed.Output, failureOutputKeyClass)
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/scoped-repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  class,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed scoped repair %d: %v", attempt, err)
		}
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}, currentFailed, errors.New("exit status 1")); err != nil {
		t.Fatalf("duplicate failure with exhausted scoped repair budget should block on repair exhaustion: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on repair exhaustion decision", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get repair exhaustion ticket: %v", err)
	}
	if !strings.HasSuffix(string(ticket.ID), "repair-exhausted-decision") || ticket.Context["attempts"] != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("ticket = %+v, want repair exhaustion ticket", ticket)
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get repair exhaustion rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected {
		t.Fatalf("result = %+v, want rejected result for business repair exhaustion", result)
	}
}

func TestRuntimeOrchestrator_DuplicateRuntimeFailureUsesTargetScopeAcrossGraphNodes(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-duplicate-failure-target-scope")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		node := fmt.Sprintf("duplicate-scope-repair-%02d", attempt)
		signature := fmt.Sprintf("class=shell_command_failed|kind=shell/command|node=%s|failureTargets=stream_e2e_test.go|exitCode=1", node)
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/target-repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed target repair %d: %v", attempt, err)
		}
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/graph-010/001-baseline"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=baseline|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}); err != nil {
		t.Fatalf("seed previous target failure: %v", err)
	}
	currentFailed := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-011/002-targeted"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=targeted|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}

	if !orch.shouldBlockOnDuplicateRuntimeFailure(ctx, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-target-duplicate")}, currentFailed) {
		t.Fatalf("duplicate failure should block after target-scoped budget is exhausted")
	}
}

func TestRuntimeOrchestrator_RuntimeRepairExhaustionFailsRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{}
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
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-runtime-repair-exhausted")
	runID := common.RunID("run-runtime-repair-exhausted")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-runtime-repair-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		stepID := common.StepID(fmt.Sprintf("%s/seed-repair-%d", runID, attempt))
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{stepInputKeyRuntimeRepairFollowUp: true},
		}); err != nil {
			t.Fatalf("seed repair step: %v", err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(fmt.Sprintf("%s/block/run-tests", runID)),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID: "run-tests",
		},
		Output: map[string]any{"status": "failed", "stderr": "still failing"},
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-runtime-repair-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}, failedStep, errors.New("exit status 1")); err != nil {
		t.Fatalf("repair exhaustion should close run without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on decision after repair exhaustion", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get repair exhaustion ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen ||
		!strings.Contains(ticket.Reason, "runtime repair attempts exhausted") ||
		ticket.Context["attempts"] != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("ticket = %+v, want repair exhaustion decision context", ticket)
	}
	if ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		t.Fatalf("ticket context = %+v, want explicit repair recovery policy", ticket.Context)
	}
}

func TestRuntimeOrchestrator_ResumeRunningFailedDoDCoverageRepairExhaustionBlocksRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-running-dod-coverage-exhausted")
	runID := common.RunID("run-running-dod-coverage-exhausted")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-running-dod-coverage-exhausted",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	signature := "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment|error=invalid argument: dodassessment.goalcoverage is required when goal has uncovered leaf requirements"
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		stepID := common.StepID(fmt.Sprintf("%s/repair-%03d-of-llm-after-decision", runID, attempt))
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfStepID:        string(runID) + "/llm-after-decision",
				stepInputKeyRepairOfRootStepID:    string(runID) + "/llm-after-decision",
				stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
				stepInputKeyRepairOfFailureSig:    signature,
				stepInputKeyAssessmentOnly:        true,
			},
			Output: map[string]any{
				"stage":                   "llm_dod_assessment",
				"error":                   "invalid argument: dodAssessment.goalCoverage is required when Goal has uncovered leaf requirements",
				failureOutputKeyClass:     failureClassDoDCoverageMissing,
				failureOutputKeySignature: signature,
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"summary":   "DoD passed but goal coverage is missing.",
					"items": []map[string]any{{
						"item":         "verify runtime flow",
						"state":        "satisfied",
						"evidenceRefs": []string{string(stepID)},
					}},
				},
				"runtimeDiagnostics": map[string]any{
					"kind": failureClassDoDCoverageMissing,
					"missingGoalCoverage": []map[string]any{{
						"raw":   "verify LLM driven workspace execution",
						"kind":  "leaf",
						"state": "missing",
					}},
				},
			},
		}); err != nil {
			t.Fatalf("seed repair attempt %d: %v", attempt, err)
		}
	}

	err := orch.ResumeRun(ctx, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("ResumeRun err = %v, want blocked after repair exhaustion", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on decision after running failed terminal repair exhaustion", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Context["failureClass"] != failureClassDoDCoverageMissing || ticket.Context["attempts"] != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("ticket context = %+v, want exhausted dod coverage repair context", ticket.Context)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) == 0 || results[0].State != common.ResultStateRejected {
		t.Fatalf("results = %+v, want rejected result when repair exhaustion blocks", results)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairExhaustionWritesRejectedResultBeforeTicketEvent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resultRepo := memory.NewResultRepository()
	hub := events.NewMemoryHub()
	runID := common.RunID("run-repair-exhaustion-result-before-ticket")
	resultID := common.ResultID(string(runID) + "/result/dod-assessment")
	resultVisibleAtTicketEvent := false
	hub.Subscribe(events.TopicTicketCreated, func(ctx context.Context, _ events.Envelope) error {
		result, err := resultRepo.Get(ctx, resultID)
		resultVisibleAtTicketEvent = err == nil && result != nil && result.State == common.ResultStateRejected
		return nil
	})
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		hub,
	)
	workspaceID := common.WorkspaceID("ws-repair-exhaustion-result-before-ticket")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-repair-exhaustion-result-before-ticket",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	signature := "class=runtime_failed|target=planning"
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d-of-planning-001", runID, attempt)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
				stepInputKeyRepairOfFailureClass:  failureClassRuntime,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{
				"stage":                   "llm_execution_graph_plan",
				"error":                   "invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply",
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: signature,
			},
		}); err != nil {
			t.Fatalf("seed repair attempt %d: %v", attempt, err)
		}
	}
	failedStep, err := stepRepo.Get(ctx, common.StepID(fmt.Sprintf("%s/repair-%03d-of-planning-001", runID, defaultMaxRuntimeRepairAttempts)))
	if err != nil {
		t.Fatalf("get failed repair: %v", err)
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-repair-exhaustion-result-before-ticket",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}, failedStep, errors.New("invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply")); err != nil {
		t.Fatalf("repair exhaustion: %v", err)
	}
	if !resultVisibleAtTicketEvent {
		t.Fatalf("rejected result was not visible when repair exhausted ticket event was published")
	}
}
