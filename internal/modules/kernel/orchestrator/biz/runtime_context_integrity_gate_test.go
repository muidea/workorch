package orchestrator

import (
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
