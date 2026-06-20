package orchestrator

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestUpdateConvergenceBudgetEscalatesNoDeltaRepeatedFailure(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-convergence-budget",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			coverageFocusKey: map[string]any{
				"id":      "focus-item-1",
				"sliceId": "slice-1",
				"state":   "active",
			},
			convergenceBudgetKey: map[string]any{
				"state":                       "tracking",
				"decision":                    "continue",
				"maxNoCoverageDeltaRounds":    2,
				"maxRepeatedFailureSignature": 2,
				"maxRepairAttemptsPerFocus":   5,
				"maxLLMRoundsPerSlice":        5,
				"maxValidationOnlyRetries":    5,
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-convergence-budget/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|target=one",
		},
	}

	if !updateConvergenceBudgetFromStep(run, step, false) {
		t.Fatal("updateConvergenceBudgetFromStep changed = false, want budget update")
	}
	step.ID = "run-convergence-budget/repair-002"
	if !updateConvergenceBudgetFromStep(run, step, false) {
		t.Fatal("second update changed = false, want budget escalation")
	}
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if budget["decision"] != "escalate_decision" ||
		budget["state"] != "escalated" ||
		budget["lastDecisionReason"] != "max_repeated_failure_signature_exceeded" {
		t.Fatalf("budget = %+v, want repeated failure escalation", budget)
	}
	if int64FromValue(budget["noCoverageDeltaRounds"]) != 2 ||
		int64FromValue(budget["repeatedFailureSignatures"]) != 2 ||
		int64FromValue(budget["repairAttempts"]) != 2 {
		t.Fatalf("budget counters = %+v, want no-delta/repeated/repair counters", budget)
	}
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	if focus["failureClass"] != failureClassShellCommand ||
		focus["failureSignature"] != "class=shell_command_failed|kind=shell/command|target=one" ||
		int64FromValue(focus["attempt"]) != 2 {
		t.Fatalf("focus = %+v, want failure and attempt projected", focus)
	}
}

func TestUpdateConvergenceBudgetResetsNoDeltaAfterCoverageDelta(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-convergence-budget-reset",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			coverageFocusKey: map[string]any{
				"id":      "focus-item-1",
				"sliceId": "slice-1",
				"state":   "active",
			},
			convergenceBudgetKey: map[string]any{
				"state":                    "escalated",
				"decision":                 "escalate_decision",
				"lastDecisionReason":       "max_no_coverage_delta_rounds_exceeded",
				"noCoverageDeltaRounds":    2,
				"maxNoCoverageDeltaRounds": 2,
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-convergence-budget-reset/verify",
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"status": "completed",
		},
	}

	if !updateConvergenceBudgetFromStep(run, step, true) {
		t.Fatal("updateConvergenceBudgetFromStep changed = false, want reset update")
	}
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if budget["decision"] != "continue" || budget["state"] != "tracking" {
		t.Fatalf("budget = %+v, want resumed tracking after coverage delta", budget)
	}
	if int64FromValue(budget["noCoverageDeltaRounds"]) != 0 || budget["lastNoDeltaReason"] != nil || budget["lastDecisionReason"] != nil {
		t.Fatalf("budget = %+v, want no-delta state cleared", budget)
	}
}

func TestConvergenceBudgetDoesNotCountConcreteEvidenceGraphNodeAsRepairAttempt(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-convergence-budget-evidence-node",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			coverageFocusKey: map[string]any{
				"id":      "focus-item-1",
				"sliceId": "slice-1",
				"state":   "active",
			},
			convergenceBudgetKey: map[string]any{
				"state":                       "tracking",
				"decision":                    "continue",
				"maxNoCoverageDeltaRounds":    2,
				"maxRepairAttemptsPerFocus":   2,
				"maxLLMRoundsPerSlice":        5,
				"maxRepeatedFailureSignature": 5,
			},
		},
	}
	evidenceNode := &runmodel.Step{
		ID:    "run-convergence-budget-evidence-node/graph-001/001-list-root",
		Kind:  "file/list",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphNodeID:           "list-root",
			stepInputKeyGraphRootStepID:       "run-convergence-budget-evidence-node/repair-001",
			"concreteFailureEvidenceRequired": true,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
		Output: map[string]any{"stage": "file_list", "status": "completed"},
	}

	if !updateConvergenceBudgetFromStep(run, evidenceNode, false) {
		t.Fatal("updateConvergenceBudgetFromStep changed = false, want focus/budget scope update")
	}
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if int64FromValue(budget["repairAttempts"]) != 0 ||
		int64FromValue(budget["noCoverageDeltaRounds"]) != 0 ||
		budget["decision"] != "continue" {
		t.Fatalf("budget after evidence node = %+v, want no repair/no-delta consumption", budget)
	}

	llmRepair := &runmodel.Step{
		ID:    "run-convergence-budget-evidence-node/repair-002",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
		Output: map[string]any{failureOutputKeyClass: failureClassLLMDecisionInvalid},
	}
	if !updateConvergenceBudgetFromStep(run, llmRepair, false) {
		t.Fatal("LLM repair update changed = false, want budget update")
	}
	budget = mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if int64FromValue(budget["repairAttempts"]) != 1 ||
		int64FromValue(budget["noCoverageDeltaRounds"]) != 1 {
		t.Fatalf("budget after llm repair = %+v, want one repair/no-delta round", budget)
	}
}

func TestConvergenceBudgetEscalationBlocksRuntimeRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-convergence-budget-block",
		DoDID:           "dod-convergence-budget-block",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			convergenceBudgetKey: map[string]any{
				"state":              "escalated",
				"decision":           "escalate_decision",
				"lastDecisionReason": "max_no_coverage_delta_rounds_exceeded",
			},
		},
	}
	failedStep := &runmodel.Step{
		ID:    "run-convergence-budget-block/repair-003",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|target=one",
		},
	}

	if !shouldEscalateRuntimeRepairExhaustion(nil, &repairSubsystem{}, RuntimePolicy{}, true, run, failedStep) {
		t.Fatal("shouldEscalateRuntimeRepairExhaustion = false, want convergence budget escalation")
	}
	context := convergenceBudgetEscalationContext(run)
	if context["lastDecisionReason"] != "max_no_coverage_delta_rounds_exceeded" {
		t.Fatalf("convergenceBudgetEscalationContext = %+v, want budget reason", context)
	}
}
