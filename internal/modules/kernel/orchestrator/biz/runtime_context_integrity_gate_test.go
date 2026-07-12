package orchestrator

import (
	"errors"
	"strings"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeContextIntegrityGateFailureIsNotStateChangingRepair(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-ctx/step-1"),
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"stage": "llm_context_integrity_failed",
			"error": "invalid argument: llm context gate read_required: protected_repair_action_target_snapshot_missing_after_context_budget",
			"contextBudget": map[string]any{
				"contextGateAction": "read_required",
				"contextGateReason": "protected_repair_action_target_snapshot_missing_after_context_budget",
			},
		},
	}

	if !runtimeContextIntegrityGateFailure(step) {
		t.Fatalf("runtimeContextIntegrityGateFailure = false, want true")
	}
	if stateChangingRepairRequired(nil, step, common.ErrInvalidArgument) {
		t.Fatalf("stateChangingRepairRequired = true, want false for provider context gate failure")
	}

	run := &runModel.Run{
		ID:    common.RunID("run-ctx"),
		State: common.RunStateActive,
		ExecutionContext: map[string]any{
			"completionGateRequired": true,
		},
	}
	orch := &RuntimeOrchestrator{}
	if action := runtimeRecoveryActionForFailedStepForTest(t.Context(), orch, run, step); action != runtimeRecoveryActionContextIntegrityBlocked {
		t.Fatalf("recoveryActionForFailedStep = %q, want %q", action, runtimeRecoveryActionContextIntegrityBlocked)
	}
}

func TestRuntimeRequestBudgetFailureUsesContextGateInsteadOfProviderRetry(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-budget/step-1"),
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":               "llm_protected_context_floor_exceeded",
			failureOutputKeyClass: failureClassProtectedContextFloor,
			"contextBudget": map[string]any{
				"contextGateAction":            "fail_fast",
				"contextGateReason":            "P0 contract exceeds provider context budget",
				"minimumProtectedRequestBytes": 63952,
				"maxBytes":                     61178,
			},
		},
	}
	run := &runModel.Run{ID: "run-budget", InitialStepKind: llmWorkspaceExecutionStepKind, State: common.RunStateActive}
	orch := &RuntimeOrchestrator{}
	if !runtimeContextIntegrityGateFailure(step) {
		t.Fatal("runtimeContextIntegrityGateFailure = false, want true")
	}
	if action := runtimeRecoveryActionForFailedStepForTest(t.Context(), orch, run, step); action != runtimeRecoveryActionContextIntegrityBlocked {
		t.Fatalf("recovery action = %q, want context integrity blocked", action)
	}
	question := localizedRequestBudgetGateQuestion("zh-CN")
	if strings.Contains(question, "修复或切换 runtime LLM provider") || !strings.Contains(question, "本地请求预算") {
		t.Fatalf("localized request budget question = %q, want local budget guidance without provider configuration remediation", question)
	}
}

func TestStateChangingRepairRequiredUsesStructuredContractViolationType(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-contract/step-1"),
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			outputKeyContractViolationType: contractViolationRepairNoStateChangingOutput,
		},
	}

	if !stateChangingRepairRequired(nil, step, nil) {
		t.Fatalf("stateChangingRepairRequired = false, want true from structured contractViolationType")
	}
}

func TestAttachContractViolationTypeUsesStructuredValidationError(t *testing.T) {
	err := newRuntimeContractViolationError(contractViolationRepairNoStateChangingOutput, "state-changing output is required")
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("structured contract violation error does not wrap ErrInvalidArgument: %v", err)
	}
	output := map[string]any{}

	attachContractViolationType(output, err)

	if got := output[outputKeyContractViolationType]; got != contractViolationRepairNoStateChangingOutput {
		t.Fatalf("contractViolationType = %v, want %s", got, contractViolationRepairNoStateChangingOutput)
	}
	if got := output["contractViolationRule"]; got != contractViolationRuleStateChangingRequiredMissing {
		t.Fatalf("contractViolationRule = %v, want %s", got, contractViolationRuleStateChangingRequiredMissing)
	}
	if got := output["contractViolationRecoverability"]; got != "retry_same_contract" {
		t.Fatalf("contractViolationRecoverability = %v, want retry_same_contract", got)
	}
}

func TestAttachContractViolationTypeClassifiesPatchTargetRef(t *testing.T) {
	output := map[string]any{"stage": "patch_dispatch_invalid"}
	attachContractViolationType(output, common.ErrInvalidArgument)

	if got := output[outputKeyContractViolationType]; got != contractViolationPatchDispatchInvalid {
		t.Fatalf("contractViolationType = %v, want %s", got, contractViolationPatchDispatchInvalid)
	}

	output = map[string]any{"error": "invalid argument: executionGraph patch/apply targetRef is required"}
	attachContractViolationType(output, nil)
	if got := output[outputKeyContractViolationType]; got != contractViolationPatchTargetRefRequired {
		t.Fatalf("contractViolationType = %v, want %s", got, contractViolationPatchTargetRefRequired)
	}
}

func TestAttachContractViolationTypeClassifiesCoverageTargetsRequired(t *testing.T) {
	output := map[string]any{
		"error": "invalid argument: coverage-bound state-changing patch/apply must declare coverageTargets matching current Goal, DoD, or deliverable coverage targets",
	}
	attachContractViolationType(output, nil)

	if got := output[outputKeyContractViolationType]; got != contractViolationCoverageTargetsRequired {
		t.Fatalf("contractViolationType = %v, want %s", got, contractViolationCoverageTargetsRequired)
	}
	step := &runModel.Step{
		ID:     common.StepID("run-contract/step-coverage-targets"),
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateFailed,
		Output: output,
	}
	if !stateChangingRepairRequired(nil, step, nil) {
		t.Fatalf("stateChangingRepairRequired = false, want true from coverage target contract violation")
	}
}

func TestCoverageBoundPatchAcceptsAuthorityCoverageWorkItemID(t *testing.T) {
	run := &runModel.Run{ID: "run-coverage-target", WorkspaceID: "ws-coverage-target"}
	step := &runModel.Step{
		ID:    "run-coverage-target/graph-001/001-patch",
		RunID: run.ID,
		Kind:  "patch/apply",
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"runtimeStage":                runtimeStageRepair,
			"repairObjective":             "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"coverageTargetSet": []map[string]any{{
					"coverageTargetToken": "source_change",
					"sourceRequirementId": "source_change",
				}},
				"coverageFocus": map[string]any{
					"id":                  "focus-cwi-036",
					"coverageWorkItemIds": []string{"cwi-036"},
					"targetRefs":          []string{"cwi-036", "deliverable:deliverable:source_change", "source_change"},
				},
				"requiredOutput": map[string]any{
					"coverageFocusId":     "focus-cwi-036",
					"coverageWorkItemIds": []string{"cwi-036"},
					"structuredTargetRefs": []string{
						"cwi-036",
						"deliverable:deliverable:source_change",
						"source_change",
					},
				},
			},
		},
	}
	output := map[string]any{
		"patches": []map[string]any{{
			"targetRef":       "main_test.go",
			"patchType":       "text_replace",
			"coverageTargets": []string{"cwi-036"},
		}},
	}

	if err := validatePatchStepCoverageTargetsBeforeRuntime(run, step, output); err != nil {
		t.Fatalf("validatePatchStepCoverageTargetsBeforeRuntime err = %v, want nil", err)
	}
}

func TestPatchStepContractViolationDiagnosticsAreRecoverable(t *testing.T) {
	run := &runModel.Run{ID: "run-patch-contract", WorkspaceID: "ws-patch-contract"}
	step := &runModel.Step{
		ID:     "run-patch-contract/graph-001/001-patch",
		RunID:  run.ID,
		Kind:   "patch/apply",
		State:  common.StepStateFailed,
		Input:  map[string]any{"runtimeStage": runtimeStageRepair},
		Output: map[string]any{},
	}
	err := errors.New("invalid argument: coverage-bound state-changing patch/apply must declare coverageTargets matching current Goal, DoD, or deliverable coverage targets")

	attachContractViolationType(step.Output, err)
	attachRuntimeFailureClassification(step.Output, classifyRuntimeFailure(step, step.Output, "runtime state-changing contract gate failed", err))
	attachContractViolationDiagnostics(run, step, err)

	if got := mapFromValue(step.Output[stepOutputKeyContractViolation]); len(got) == 0 {
		t.Fatalf("contractViolation = %+v, want nested recoverable violation", step.Output[stepOutputKeyContractViolation])
	}
	if !failedStepHasRecoverableContractViolation(step) {
		t.Fatalf("failedStepHasRecoverableContractViolation = false, want true")
	}
}

func TestRuntimeContextIntegrityGateCoverageTargetFailureCanRecover(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-ctx-coverage/repair-001"),
		RunID: common.RunID("run-ctx-coverage"),
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"coverageRepairRequired":          true,
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-runtime-context",
				"raw":               "runtime context must carry coverage targets",
				"state":             "failed",
			}},
		},
		Output: map[string]any{
			"stage": "llm_context_integrity_failed",
			"error": "invalid argument: llm context gate replan_required: coverage_repair_required_without_coverage_target_set",
			"contextBudget": map[string]any{
				"contextGateAction": "replan_required",
				"contextGateReason": "coverage_repair_required_without_coverage_target_set",
			},
		},
	}
	if !runtimeContextIntegrityGateFailure(step) {
		t.Fatalf("runtimeContextIntegrityGateFailure = false, want true")
	}
	if !runtimeContextIntegrityGateCanRecoverWithCoverageTargets(step) {
		t.Fatalf("runtimeContextIntegrityGateCanRecoverWithCoverageTargets = false, want true")
	}

	run := &runModel.Run{
		ID:              common.RunID("run-ctx-coverage"),
		State:           common.RunStateActive,
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"completionGateRequired": true,
		},
	}
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   memory.NewStepRepository(),
	}
	if action := runtimeRecoveryActionForFailedStepForTest(t.Context(), orch, run, step); action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recoveryActionForFailedStep = %q, want %q", action, runtimeRecoveryActionRuntimeRepair)
	}
	input := orch.runtimeRepairFollowUpStepInput(t.Context(), run, step, 1, common.ErrInvalidArgument)
	if targets := mapSliceFromValue(input["coverageTargetSet"]); len(targets) != 1 || targets[0]["goalRequirementId"] != "goal-req-runtime-context" {
		t.Fatalf("coverageTargetSet = %+v, want inherited Goal target", input["coverageTargetSet"])
	}
}

func TestBlockOnContextIntegrityGateRecordsRecommendedRecoveryAction(t *testing.T) {
	ctx := t.Context()
	workspaceID := common.WorkspaceID("ws-context-gate")
	runID := common.RunID("run-context-gate")
	stepID := common.StepID("run-context-gate/repair-001")
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{
		ID:          workspaceID,
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	run := &runModel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	failed := &runModel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"repairActionPackage": map[string]any{
				"coverageTargetSet": []map[string]any{{"targetText": "target"}},
			},
		},
		Output: map[string]any{
			"stage": "llm_context_integrity_failed",
			"contextBudget": map[string]any{
				"contextGateAction": "replan_required",
				"contextGateReason": "coverage_repair_required_without_coverage_target_set",
			},
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{})

	if err := orch.blockOnContextIntegrityGate(ctx, run, failed, common.ErrInvalidArgument); err != nil {
		t.Fatalf("block on context integrity gate: %v", err)
	}
	updated, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get updated run: %v", err)
	}
	ticket, err := ticketRepo.Get(ctx, updated.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Context["recommendedRecoveryAction"] != "replan_required" {
		t.Fatalf("ticket context recommendedRecoveryAction = %#v, want replan_required", ticket.Context["recommendedRecoveryAction"])
	}
	allowed, ok := ticket.Context["allowedRecoveryActions"].([]string)
	if !ok || len(allowed) != 3 || allowed[0] != "replan" || allowed[1] != "provide_information" || allowed[2] != "terminate" {
		t.Fatalf("ticket context allowedRecoveryActions = %#v, want [replan provide_information terminate]", ticket.Context["allowedRecoveryActions"])
	}
}

func TestResumeDeterministicProtectedFloorReopensResolvedTicketWithoutRetry(t *testing.T) {
	ctx := t.Context()
	workspaceID := common.WorkspaceID("ws-protected-floor-dedupe")
	runID := common.RunID("run-protected-floor-dedupe")
	stepID := common.StepID("run-protected-floor-dedupe/llm")
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	stepRepo := memory.NewStepRepository()
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	run := &runModel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	failed := &runModel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":                   "llm_protected_context_floor_exceeded",
			failureOutputKeyClass:     failureClassProtectedContextFloor,
			failureOutputKeySignature: "class=protected_context_floor_exceeded",
			"contextBudget": map[string]any{
				"contextGateAction":            "fail_fast",
				"contextGateReason":            "P0 contract exceeds provider context budget",
				"shrinkLevel":                  4,
				"p0Overflow":                   true,
				"minimumProtectedRequestBytes": 90515,
				"maxBytes":                     81018,
				"stablePrefixHash":             "stable-prefix",
			},
		},
	}
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("create failed step: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo,
		memory.NewApprovalRepository(), RuntimeEvidenceRepositories{})
	if err := orch.blockOnContextIntegrityGate(ctx, run, failed, common.ErrInvalidArgument); err != nil {
		t.Fatalf("first block: %v", err)
	}
	blocked, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run: %v", err)
	}
	ticketID := blocked.BlockingTicketID
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{ID: "resolution-floor", TicketID: ticketID, Type: decisioncommon.ResolutionTypeProvideMissingInfo}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	if err := orch.ResumeRun(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("ResumeRun error = %v, want deterministic blocked result", err)
	}
	reblocked, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get reblocked run: %v", err)
	}
	if reblocked.BlockingTicketID != ticketID {
		t.Fatalf("blocking ticket = %q, want reused %q", reblocked.BlockingTicketID, ticketID)
	}
	if reblocked.State != common.RunStateBlockedOnDecision {
		t.Fatalf("run state = %q, want blocked_on_decision", reblocked.State)
	}
	if _, err := resolutionRepo.GetByTicketID(ctx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("resolution lookup error = %v, want deleted stale resolution", err)
	}
	reused, err := ticketRepo.Get(ctx, ticketID)
	if err != nil || reused.State != common.DecisionTicketStateOpen {
		t.Fatalf("reused ticket = %+v err=%v, want open", reused, err)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil || len(steps) != 1 || steps[0].ID != stepID {
		t.Fatalf("steps = %+v err=%v, want no context-gate retry", steps, err)
	}
}
