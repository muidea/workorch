package orchestrator

import (
	"errors"
	"testing"

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
		State: common.RunStateRunning,
		ExecutionContext: map[string]any{
			"completionGateRequired": true,
		},
	}
	orch := &RuntimeOrchestrator{}
	if action := runtimeRecoveryActionForFailedStepForTest(t.Context(), orch, run, step); action != runtimeRecoveryActionContextIntegrityBlocked {
		t.Fatalf("recoveryActionForFailedStep = %q, want %q", action, runtimeRecoveryActionContextIntegrityBlocked)
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
			"error": "invalid argument: llm context gate fail_fast: coverage_repair_required_without_coverage_target_set",
			"contextBudget": map[string]any{
				"contextGateAction": "fail_fast",
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
		State:           common.RunStateRunning,
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
