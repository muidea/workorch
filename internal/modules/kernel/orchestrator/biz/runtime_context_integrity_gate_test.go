package orchestrator

import (
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
	if action := orch.recoveryActionForFailedStep(t.Context(), run, step); action != runtimeRecoveryActionContextIntegrityBlocked {
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
	if action := orch.recoveryActionForFailedStep(t.Context(), run, step); action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recoveryActionForFailedStep = %q, want %q", action, runtimeRecoveryActionRuntimeRepair)
	}
	input := orch.runtimeRepairFollowUpStepInput(t.Context(), run, step, 1, common.ErrInvalidArgument)
	if targets := mapSliceFromValue(input["coverageTargetSet"]); len(targets) != 1 || targets[0]["goalRequirementId"] != "goal-req-runtime-context" {
		t.Fatalf("coverageTargetSet = %+v, want inherited Goal target", input["coverageTargetSet"])
	}
}
