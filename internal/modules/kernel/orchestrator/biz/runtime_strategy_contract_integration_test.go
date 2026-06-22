package orchestrator

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateLLMExecutionOutputContractRejectsMissingStrategyReassessmentDecisionFields(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-missing",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-missing/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "failureTargets=main_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main-test",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "main_test.go",
						"patchType": "unified_diff",
						"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "strategyReassessment.required=true") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want strategy reassessment field rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStrategyReassessmentDecisionFieldsInPatchInput(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-valid",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-valid/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "failureTargets=main_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main-test",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "main_test.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/main_test.go\n+++ b/main_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=main_test.go",
						"rootCauseTarget":      "main_test.go imports",
						"repairTargetDecision": "patch main_test.go to align imports and symbols",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not edit unrelated files",
						"verificationPlan":     "run targeted verification for main_test.go and related import/symbol checks after patch",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want strategy reassessment-compliant patch accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsDisallowedStrategyRepairMode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-disallowed-mode",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-disallowed-mode/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=validation-target.txt",
			"strategyReassessment": map[string]any{
				"required":             true,
				"failedSurface":        "failureTargets=validation-target.txt",
				"targetRepairAttempts": int64(3),
				"allowedRepairModes":   []string{strategyRepairModeShiftRootCauseTarget},
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"repairActionPackage": map[string]any{
				"repairTargetSet": []string{"validation-target.txt", "implementation-target.txt"},
				"targetSnapshots": []map[string]any{{
					"targetRef": "validation-target.txt",
					"content":   "old",
				}},
				"rootCauseCandidateContext": []map[string]any{{
					"targetRef": "implementation-target.txt",
					"content":   "implementation target content",
				}},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-validation-target",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "validation-target.txt",
						"patchType":            "text_replace",
						"oldText":              "old",
						"newText":              "new",
						"failedSurface":        "failureTargets=validation-target.txt",
						"rootCauseTarget":      "validation-target.txt",
						"repairTargetDecision": "continue patching the failed validation surface",
						"repairMode":           strategyRepairModeContinueSameTarget,
						"forbiddenRepair":      "do not patch unrelated resources",
						"verificationPlan":     "run bounded verification for validation-target.txt",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "repairMode=continue_same_target_repair is not allowed") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want disallowed repair mode rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStrategySurfaceByTargetRef(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-target-ref-surface",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-target-ref-surface/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "targetRef=main.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "main.go",
						"patchType":            "text_replace",
						"oldText":              "old",
						"newText":              "new",
						"failedSurface":        "main.go current build failure",
						"rootCauseTarget":      "main.go",
						"repairTargetDecision": "patch main.go to address the current failed surface",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair":      "do not edit unrelated files",
						"verificationPlan":     "run bounded verification for targetRef=main.go",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want targetRef-aligned surface accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStrategyDecisionArrayFields(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-array-fields",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-array-fields/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "targetRef=main.go",
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "targetRef=main.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"repairActionPackage": map[string]any{
				"repairTargetSet": []string{"main.go"},
				"targetSnapshots": []map[string]any{{
					"targetRef": "main.go",
					"content":   "old",
				}},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "main.go",
						"patchType":            "text_replace",
						"oldText":              "old",
						"newText":              "new",
						"failedSurface":        "targetRef=main.go",
						"rootCauseTarget":      "main.go",
						"repairTargetDecision": "patch main.go current target",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair": []string{
							"do not edit unrelated files",
						},
						"verificationPlan": "run bounded verification for targetRef=main.go",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want non-empty array decision fields accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStrategyCurrentSnapshotRepairMode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-current-snapshot",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-current-snapshot/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "runtime contract wrapper failure",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"repairActionPackage": map[string]any{
				"repairTargetSet": []string{"go.mod"},
				"targetSnapshots": []map[string]any{{
					"targetRef": "go.mod",
					"content":   "module demo\n",
				}},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-go-mod",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "go.mod",
						"patchType":            "text_replace",
						"oldText":              "module demo\n",
						"newText":              "module demo\n\nrequire example.com/lib v1.0.0\n",
						"failedSurface":        "go.mod dependency surface",
						"rootCauseTarget":      "go.mod",
						"repairTargetDecision": "repair the current snapshot target",
						"repairMode":           "exact_replacement_against_current_snapshot",
						"forbiddenRepair":      "do not edit unrelated files",
						"verificationPlan":     "run bounded verification for targetRef=go.mod",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want current snapshot strategy repair accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStrategyCurrentSnapshotTextDelete(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-current-snapshot-delete",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-current-snapshot-delete/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "runtime contract wrapper failure",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"repairActionPackage": map[string]any{
				"repairTargetSet": []string{"main.go"},
				"targetSnapshots": []map[string]any{{
					"targetRef": "main.go",
					"content":   "package main\n\nimport \"strings\"\n",
				}},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main-delete",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "main.go",
						"patchType":            "text_replace",
						"oldText":              "import \"strings\"\n",
						"newText":              "",
						"failedSurface":        "main.go current failure",
						"rootCauseTarget":      "main.go",
						"repairTargetDecision": "delete stale text from current snapshot target",
						"repairMode":           "exact_replacement_against_current_snapshot",
						"forbiddenRepair":      "do not edit unrelated files",
						"verificationPlan":     "run bounded verification for targetRef=main.go",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want current snapshot text deletion accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsUndistinguishedStrategyReassessmentRootCause(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-undistinguished-root-cause",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-undistinguished-root-cause/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "failureTargets=main_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main-test",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "main_test.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/main_test.go\n+++ b/main_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=main_test.go",
						"rootCauseTarget":      "failureTargets=main_test.go",
						"repairTargetDecision": "patch main_test.go directly",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not edit unrelated files",
						"verificationPlan":     "run full regression across the workspace",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "strategyReassessment.required=true") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want undistinguished root cause rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsScopedRootCauseWithinFailedSurface(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-scoped-root-cause",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-scoped-root-cause/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "failureTargets=target.txt",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-scoped-root-cause",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "target.txt",
						"patchType":            "text_replace",
						"oldText":              "old scoped behavior",
						"newText":              "new scoped behavior",
						"failedSurface":        "failureTargets=target.txt",
						"rootCauseTarget":      "target.txt:scoped behavior block",
						"repairTargetDecision": "patch the scoped behavior block within the failed target using the current snapshot",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not repeat a broad file-level rewrite",
						"verificationPlan":     "run targeted verification for target.txt scoped behavior",
					},
				},
				{
					"id":        "verify-scoped-root-cause",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-scoped-root-cause"},
					"input": map[string]any{
						"command":           "verify",
						"args":              []string{"--target=target.txt"},
						"targetRef":         "target.txt",
						"verificationScope": "bounded_to_target.txt_scoped_behavior",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want scoped root-cause repair accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsUnboundedVerificationForScopedFailedSurface(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-unbounded-verification",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-unbounded-verification/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "failureTargets=gateway_dod_additional_paths_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-target-test",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "gateway_dod_additional_paths_test.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/gateway_dod_additional_paths_test.go\n+++ b/gateway_dod_additional_paths_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
						"rootCauseTarget":      "proxy helper recursion in gateway_dod_additional_paths_test.go",
						"repairTargetDecision": "patch recursive proxy helper into concrete handler call",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair":      "do not change unrelated feature files",
						"verificationPlan":     "run full regression across the workspace",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "bounded verification") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want bounded-verification rejection for scoped failed surface", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsRepeatedFailedSurfacePatchWithoutTargetShift(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-repeat-same-surface",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-repeat-same-surface/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-target-test-again",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "gateway_dod_additional_paths_test.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/gateway_dod_additional_paths_test.go\n+++ b/gateway_dod_additional_paths_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
						"rootCauseTarget":      "gateway routing contract in production handler",
						"repairTargetDecision": "patch gateway_dod_additional_paths_test.go again",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair":      "do not change unrelated files",
						"verificationPlan":     "run targeted verification for gateway_dod_additional_paths_test.go",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "continue_same_target_repair") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want repeated failed-surface patch rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsRepeatedFailedSurfaceSameTargetWhenPrimaryDiagnosticAddressed(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-same-target-primary-diagnostic",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-same-target-primary-diagnostic/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=target.txt",
			"primaryDiagnosticRepair": map[string]any{
				"required":    true,
				"targetRefs":  []string{"target.txt"},
				"identifiers": []string{"missingName"},
				"lineRefs": []map[string]any{{
					"targetRef": "target.txt",
					"line":      12,
				}},
			},
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "failureTargets=target.txt",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-current-diagnostic",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "target.txt",
						"patchType":            "unified_diff",
						"diff":                 "--- a/target.txt\n+++ b/target.txt\n@@ -12,1 +12,1 @@\n-missingName old\n+missingName new\n",
						"failedSurface":        "failureTargets=target.txt",
						"rootCauseTarget":      "target.txt",
						"repairTargetDecision": "patch target.txt current primary diagnostic",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair":      "do not change unrelated resources",
						"verificationPlan":     "run targeted verification for target.txt",
					},
				},
				{
					"id":        "verify-current-diagnostic",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-current-diagnostic"},
					"input": map[string]any{
						"command":           "verify",
						"args":              []string{"--target=target.txt"},
						"targetRef":         "target.txt",
						"verificationScope": "bounded_to_failure_target",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want same-target diagnostic repair accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsGroundedRootCauseCandidateShift(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-root-cause-candidate",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-root-cause-candidate/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=surface.txt",
			"failureTargetRepairContext": []map[string]any{{
				"targetRef": "surface.txt",
				"exists":    true,
				"content":   "observed failed surface\n",
			}},
			"rootCauseCandidateContext": []map[string]any{{
				"targetRef":        "root.txt",
				"exists":           true,
				"source":           "same_directory_resource",
				"content":          "root cause old\n",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-root",
			}},
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "failureTargets=surface.txt",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-root-cause",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "root.txt",
						"patchType":            "unified_diff",
						"diff":                 "--- a/root.txt\n+++ b/root.txt\n@@ -1,1 +1,1 @@\n-root cause old\n+root cause new\n",
						"failedSurface":        "failureTargets=surface.txt",
						"rootCauseTarget":      "root.txt",
						"repairTargetDecision": "patch grounded root-cause candidate",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not repeatedly patch the failed surface without evidence",
						"verificationPlan":     "run bounded verification for surface.txt after root.txt changes",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want grounded root-cause candidate shift allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsUngroundedRootCauseCandidateShift(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-ungrounded-root",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-ungrounded-root/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=surface.txt",
			"failureTargetRepairContext": []map[string]any{{
				"targetRef": "surface.txt",
				"exists":    true,
				"content":   "observed failed surface\n",
			}},
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "failureTargets=surface.txt",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-ungrounded-root",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "root.txt",
						"patchType":            "unified_diff",
						"diff":                 "--- a/root.txt\n+++ b/root.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=surface.txt",
						"rootCauseTarget":      "root.txt",
						"repairTargetDecision": "patch ungrounded root target",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not repeatedly patch the failed surface without evidence",
						"verificationPlan":     "run bounded verification for surface.txt",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "not related to failure target") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want ungrounded root-cause target rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsIncompleteSurfaceContractCorrection(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-incomplete-surface-correction",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-incomplete-surface-correction/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "targetRef=surface.txt",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"failureTargetScope": "targetRef=surface.txt",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-surface-again",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "surface.txt",
						"patchType":            "unified_diff",
						"diff":                 "--- a/surface.txt\n+++ b/surface.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "targetRef=surface.txt",
						"rootCauseTarget":      "targetRef=surface.txt",
						"repairTargetDecision": "patch the failed surface itself",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not change unrelated resources",
						"verificationPlan":     "run targeted verification for surface.txt",
						"surfaceContractCorrection": map[string]any{
							"whySurfaceIsWrong": "current resource evidence shows the surface contract is stale",
						},
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "strategyReassessment.required=true") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want incomplete surface correction rejected", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsCompleteSurfaceContractCorrection(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-complete-surface-correction",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-complete-surface-correction/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "targetRef=surface.txt",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"failureTargetScope": "targetRef=surface.txt",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-surface-contract",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "surface.txt",
						"patchType":            "unified_diff",
						"diff":                 "--- a/surface.txt\n+++ b/surface.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "targetRef=surface.txt",
						"rootCauseTarget":      "targetRef=surface.txt",
						"repairTargetDecision": "patch the failed surface because the surface contract itself is stale",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not change unrelated resources",
						"verificationPlan":     "run targeted verification for surface.txt",
						"surfaceContractCorrection": map[string]any{
							"failedSurface":           "targetRef=surface.txt",
							"whySurfaceIsWrong":       "current resource evidence shows the failed surface no longer matches the active contract",
							"contractConflict":        "the surface contract conflicts with the latest Goal and DoD evidence",
							"currentResourceEvidence": []string{"surface.txt current snapshot", "latest accepted contract evidence"},
							"boundedVerificationPlan": "run targeted verification for surface.txt only",
						},
					},
				},
				{
					"id":        "verify-surface",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-surface-contract"},
					"input": map[string]any{
						"command": "verify",
						"args":    []string{"--target=surface.txt"},
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want complete surface correction accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsRepeatedFailedSurfaceWhenPatchTargetsDistinctRootCause(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-shift-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-shift-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"failureTargetScope": "failureTargets=gateway_dod_additional_paths_test.go",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-production-handler",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "main.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/main.go\n+++ b/main.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
						"rootCauseTarget":      "main.go route registration",
						"repairTargetDecision": "patch main.go to align runtime route behavior with the acceptance contract",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not patch the failed verification artifact again in this iteration",
						"verificationPlan":     "run targeted verification for gateway_dod_additional_paths_test.go after the production route change",
						"relatedFailureTargets": []string{
							"gateway_dod_additional_paths_test.go",
						},
					},
				},
				{
					"id":        "verify-targeted",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-production-handler"},
					"input": map[string]any{
						"command": "go",
						"args":    []string{"test", "./...", "-run", "TestGateway_"},
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want distinct root-cause target accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsDependencyPrerequisiteRootCauseRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-dependency-prerequisite",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-dependency-prerequisite/repair-005",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureReason":                   "config.go:4:2: no required module provides package example.org/dependency",
			"failureTargetScope":              "failureTargets=config.go",
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "repeated_repair_failure",
				"targetRepairAttempts": 0,
				"failedSurface":        "invalid argument: strategyReassessment.required=true requires state-changing patch output to declare fields aligned with current failed surface",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"repairActionPackage": map[string]any{
				"failureTargetScope": "failureTargets=config.go",
			},
			"failedStepOutput": map[string]any{
				"stderr": "config.go:4:2: no required module provides package example.org/dependency; to add it:\n\tadd dependency metadata",
			},
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":       "module.manifest",
				"exists":          true,
				"contentHash":     "hash-module-manifest",
				"contentComplete": true,
			}},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "fix-dependency-metadata",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "module.manifest",
						"patchType":            "text_replace",
						"baseHash":             "hash-module-manifest",
						"oldText":              "module example\n",
						"newText":              "module example\nrequire example.org/dependency v1.0.0\n",
						"failedSurface":        "config.go:4:2: no required module provides package example.org/dependency",
						"rootCauseTarget":      "module.manifest",
						"repairTargetDecision": "add the missing dependency declaration required by config.go",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair":      "do not remove the import from config.go or patch unrelated resources",
						"verificationPlan":     "run targeted build after updating module.manifest",
						"coverageTargets": []string{
							"configuration dependency metadata",
						},
					},
				},
				{
					"id":        "verify-build",
					"kind":      "shell/command",
					"dependsOn": []string{"fix-dependency-metadata"},
					"input": map[string]any{
						"command": "build",
						"args":    []string{"--target=config.go"},
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want dependency prerequisite root-cause repair accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsCoverageTargetSurfaceRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-coverage-surface",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-coverage-surface/repair-005",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"coverageTargetSet": []map[string]any{{
				"coverageTargetToken": "dod-console-feedback",
				"targetText":          "DoD requires bounded operational feedback to be visible in the runtime output.",
				"item":                "DoD requires bounded operational feedback to be visible in the runtime output.",
				"kind":                "dod",
				"state":               "failed",
			}},
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "repeated_repair_failure",
				"targetRepairAttempts": 0,
				"failedSurface":        "invalid argument: concrete validation evidence repair must return bounded executionGraph instead of assessment-only output",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":       "handler.go",
				"exists":          true,
				"contentHash":     "hash-handler",
				"contentComplete": true,
			}},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-handler-feedback",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "handler.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/handler.go\n+++ b/handler.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "runtime output feedback is not yet visible",
						"rootCauseTarget":      "handler.go",
						"repairTargetDecision": "patch handler.go to expose bounded operational feedback",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair":      "do not modify unrelated resources",
						"verificationPlan":     "run targeted verification for handler.go",
						"coverageTargets": []string{
							"dod-console-feedback",
						},
					},
				},
				{
					"id":        "verify-feedback",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-handler-feedback"},
					"input": map[string]any{
						"command":           "verify",
						"args":              []string{"--target=handler.go"},
						"targetRef":         "handler.go",
						"verificationScope": "bounded_to_repaired_target",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want coverage-target surface repair accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsRepeatedFailedSurfacePatchWhenRootCauseTargetDoesNotMatchPatchTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-root-cause-mismatch",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-root-cause-mismatch/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 3,
				"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"failureTargetScope": "failureTargets=gateway_dod_additional_paths_test.go",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-target-test-again",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "gateway_dod_additional_paths_test.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/gateway_dod_additional_paths_test.go\n+++ b/gateway_dod_additional_paths_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
						"rootCauseTarget":      "main.go",
						"repairTargetDecision": "patch the verification artifact again",
						"repairMode":           "shift_root_cause_target",
						"forbiddenRepair":      "do not patch unrelated files",
						"verificationPlan":     "run targeted verification for gateway_dod_additional_paths_test.go",
						"relatedFailureTargets": []string{
							"gateway_dod_additional_paths_test.go",
						},
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "match declared rootCauseTarget") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want root-cause target consistency rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsReplanContinuationRepeatedFailedSurfacePatchWithoutDecisionFields(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-continuation-strategy-reassessment",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-replan-continuation-strategy-reassessment/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"replanStateChangingContinuation": true,
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"targetRepairAttempts": 2,
				"failedSurface":        "targetRef=e2e_gateway_contract_test.go",
				"failureTargetScope":   "targetRef=e2e_gateway_contract_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"repairActionPackage": map[string]any{
				"failureTargetScope": "targetRef=e2e_gateway_contract_test.go",
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-e2e-contract-test-full-rewrite",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "e2e_gateway_contract_test.go",
						"patchType": "file_write",
						"content":   "package main\n",
						"summary":   "rewrite failed surface again without root-cause target shift",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "strategyReassessment.required=true") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want replan continuation reassessment rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsPatchScopedVerificationForScopedStateChangingRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-scoped-repair-unbounded-verification-without-reassessment",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-scoped-repair-unbounded-verification-without-reassessment/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=gateway_dod_additional_paths_test.go",
			"lastFailureDiagnostics": map[string]any{
				"failureClass":     "shell_command_failed",
				"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=gateway_dod_additional_paths_test.go",
				"error":            "exit status 1",
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-target-test",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "gateway_dod_additional_paths_test.go",
						"patchType": "unified_diff",
						"diff":      "--- a/gateway_dod_additional_paths_test.go\n+++ b/gateway_dod_additional_paths_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
					},
				},
				{
					"id":        "verify-all",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-target-test"},
					"input": map[string]any{
						"command": "go",
						"args":    []string{"test", "./..."},
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want patch-scoped verification accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsCurrentFailureDiagnosticsForStateChangingRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-current-failure-diagnostics",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-current-failure-diagnostics/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failedStepId":                    "run-current-failure-diagnostics/graph-001/002-verify",
			"failureTargetScope":              "failureTargets=main.go",
			"currentFailure": map[string]any{
				"failedStepId":     "run-current-failure-diagnostics/graph-001/002-verify",
				"failedStepKind":   "shell/command",
				"failureClass":     failureClassShellCommand,
				"failureSignature": "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=main.go",
				"reason":           "test output contains failure markers",
			},
			"repairActionPackage": map[string]any{
				"currentFailure": map[string]any{
					"failureClass":     failureClassShellCommand,
					"failureSignature": "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=main.go",
					"reason":           "test output contains failure markers",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "main.go",
						"patchType": "file_write",
						"baseHash":  "hash",
						"content":   "package main\n",
					},
				},
				{
					"id":        "verify-main",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-main"},
					"input": map[string]any{
						"command":               "go",
						"args":                  []string{"test", "./..."},
						"relatedFailureTargets": []string{"main.go"},
						"verificationScope":     "main.go",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want currentFailure diagnostics accepted", err)
	}
}

func TestCompactReplanContinuationOmitsRawOutputContent(t *testing.T) {
	rawContent := strings.Repeat("package main\n", 1000)
	compacted := compactReplanContinuationForRuntimeContext(map[string]any{
		"failureContext": map[string]any{
			"failedStepId":     "run-replan/graph-001/001-patch",
			"failedStepKind":   "patch/apply",
			"failureClass":     failureClassPatchApply,
			"failureSignature": "class=patch_apply_failed|targetRef=main.go",
			"failedStepOutput": map[string]any{
				"status":  "completed",
				"error":   "patch failed",
				"content": rawContent,
				stepOutputKeyExecutionGraph: map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "rewrite-main",
							"kind": "patch/apply",
							"input": map[string]any{
								"targetRef": "main.go",
								"content":   rawContent,
							},
						},
					},
				},
				"patches": []map[string]any{
					{
						"targetRef": "main.go",
						"patchType": "file_write",
						"content":   rawContent,
						"diff":      rawContent,
						"summary":   "rewrite main",
					},
				},
			},
		},
		stepInputKeyProcessEvidenceTimeline: []map[string]any{
			{
				"sourceRef": "step://run-replan/graph-001/001-patch",
				"kind":      "patch/apply",
				"state":     string(common.StepStateFailed),
				"output": map[string]any{
					"content": rawContent,
					"patches": []map[string]any{
						{"targetRef": "main.go", "content": rawContent, "diff": rawContent},
					},
				},
			},
		},
	})

	encoded, err := json.Marshal(compacted)
	if err != nil {
		t.Fatalf("marshal compacted replan continuation: %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"content", "executionGraph", rawContent[:64]} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("compacted replan continuation retained %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "targetRef") {
		t.Fatalf("compacted replan continuation lost patch metadata: %s", text)
	}
}

func TestValidateLLMExecutionOutputContractRejectsUnboundedVerificationForScopedStateChangingRepairWithoutScopeLink(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-unbounded-verification-without-scope-link",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-unbounded-verification-without-scope-link/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=gateway_dod_additional_paths_test.go",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-target-test",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "gateway_dod_additional_paths_test.go",
						"patchType": "unified_diff",
						"diff":      "--- a/gateway_dod_additional_paths_test.go\n+++ b/gateway_dod_additional_paths_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
					},
				},
				{
					"id":   "verify-all",
					"kind": "shell/command",
					"input": map[string]any{
						"command": "go",
						"args":    []string{"test", "./..."},
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "scoped state-changing runtime repair requires bounded verification") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want scoped state-changing bounded-verification rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsBoundedVerificationForScopedFailedSurface(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-strategy-reassessment-bounded-verification",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-strategy-reassessment-bounded-verification/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"strategyReassessment": map[string]any{
				"required":      true,
				"failedSurface": "failureTargets=gateway_dod_additional_paths_test.go",
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-target-test",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef":            "gateway_dod_additional_paths_test.go",
						"patchType":            "unified_diff",
						"diff":                 "--- a/gateway_dod_additional_paths_test.go\n+++ b/gateway_dod_additional_paths_test.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
						"failedSurface":        "failureTargets=gateway_dod_additional_paths_test.go",
						"rootCauseTarget":      "proxy helper recursion in gateway_dod_additional_paths_test.go",
						"repairTargetDecision": "patch recursive proxy helper into concrete handler call",
						"repairMode":           "continue_same_target_repair",
						"forbiddenRepair":      "do not change unrelated feature files",
						"verificationPlan":     "run targeted verification for gateway_dod_additional_paths_test.go and related contract checks",
					},
				},
				{
					"id":        "verify-targeted",
					"kind":      "shell/command",
					"dependsOn": []string{"patch-target-test"},
					"input": map[string]any{
						"command": "go",
						"args":    []string{"test", "./...", "-run", "TestGateway_UsageCSVAppendWithUsage|TestGateway_ConcurrentCSVAppend_NoLoss"},
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want bounded verification accepted", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsMissingLastFailureDiagnosticsForStateChangingRepair(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-missing-last-failure-diagnostics",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-missing-last-failure-diagnostics/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failedStepId":                    "run-state-changing-repair-missing-last-failure-diagnostics/graph-001/002-verify",
			stepInputKeyRepairOfStepID:        "run-state-changing-repair-missing-last-failure-diagnostics/graph-001/002-verify",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "main.go",
						"patchType": "file_write",
						"content":   "package main\n",
					},
				},
			},
		},
	}
	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "requires lastFailureDiagnostics") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want missing state-changing repair diagnostics rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsCoverageContinuationWithoutFailureDiagnostics(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-coverage-continuation-without-failure-diagnostics",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-coverage-continuation-without-failure-diagnostics/follow-up-repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"repairOfStepId":                  "run-coverage-continuation-without-failure-diagnostics/previous-invalid-graph",
			"coverageRepairRequired":          true,
			"coverageTargetSet": []map[string]any{{
				"sourceRequirementId": "source_change",
				"missingEvidenceKinds": []string{
					"state_change",
					"validation",
				},
			}},
			"currentProgressDigest": map[string]any{
				"currentGap":         "none_observed_in_recent_process_evidence",
				"nextRequiredAction": "continue_current_stage_contract",
			},
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "patch-target",
				"kind": "patch/apply",
				"input": map[string]any{
					"targetRef":       "deliverable.txt",
					"patchType":       "text_replace",
					"oldText":         "old",
					"newText":         "new",
					"coverageTargets": []string{"source_change"},
				},
			}},
		},
	}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want coverage continuation accepted without failure diagnostics", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairWithLastFailureDiagnostics(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-with-last-failure-diagnostics",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-with-last-failure-diagnostics/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"lastFailureDiagnostics": map[string]any{
				"failureClass":     "shell_command_failed",
				"failureSignature": "class=shell_command_failed|kind=shell/command|exitCode=1",
				"error":            "exit status 1",
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "patch-main",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "main.go",
						"patchType": "file_write",
						"content":   "package main\n",
					},
				},
			},
		},
	}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want allowed when lastFailureDiagnostics exists", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsPatchRepairReadOnlyNoOpGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-patch-repair-readonly-noop",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-patch-repair-readonly-noop/repair-001-of-graph-001-003-patch",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"failedStepKind":              "patch/apply",
			"failedStepOutput": map[string]any{
				"stage":                   "patch_apply",
				"error":                   "invalid argument: unified diff hunk context mismatch",
				failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply",
			},
			"patchRepairContext": []map[string]any{
				{"targetRef": "main.go", "exists": true, "contentHash": "hash-main"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "read-main",
					"kind":  "file/read",
					"input": map[string]any{"path": "main.go"},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "executionGraph without patch/apply") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want read-only patch no-op graph rejected", err)
	}
}

func TestValidateLLMExecutionOutputContractRequiresGraphNodeStateChangeOnlyByExplicitInput(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-node-explicit-state-change",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_workspace_execution",
		"summary": "read-only node completed",
	}
	readOnlyStep := &runmodel.Step{
		ID:   "run-graph-node-explicit-state-change/graph-001/001-implement-looking-name",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
			stepInputKeyGraphNodeID:  "implement_feature",
		},
	}
	if err := validateLLMExecutionOutputContract(run, readOnlyStep, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want graph node id text ignored without explicit contract", err)
	}

	stateChangingStep := &runmodel.Step{
		ID:   "run-graph-node-explicit-state-change/graph-001/002-unit",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			stepInputKeyGraphNodeID:       "unit-002",
			"stateChangingOutputRequired": true,
		},
	}
	err := validateLLMExecutionOutputContract(run, stateChangingStep, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "no state-changing output") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want explicit state-changing graph node rejection", err)
	}
}

func TestValidatePatchStepInputBeforeRuntimeRejectsPlaceholder(t *testing.T) {
	err := validatePatchStepInputBeforeRuntime("patch/apply", map[string]any{
		"patchType": "file_write",
		"targetRef": "config.yaml",
		"content":   "__FROM_PREVIOUS_STEP_OUTPUT__",
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %v, want unresolved placeholder rejection", err)
	}
}

func TestValidatePatchStepInputBeforeRuntimeRejectsPlaceholderBaseHash(t *testing.T) {
	err := validatePatchStepInputBeforeRuntime("patch/apply", map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main_test.go",
		"baseHash":  "${read-main-test.output.contentHash}",
		"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@ -1 +1 @@\n-old\n+new\n",
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "baseHash contains unresolved runtime template") {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %v, want unresolved baseHash placeholder rejection", err)
	}
}

func TestValidatePatchStepInputBeforeRuntimeRejectsEllipsisUnifiedDiffHunk(t *testing.T) {
	err := validatePatchStepInputBeforeRuntime("patch/apply", map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main_test.go",
		"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@ ... @@\n-old\n+new\n",
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "diff contains unresolved runtime template") {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %v, want ellipsis hunk placeholder rejection", err)
	}
}

func TestValidatePatchStepInputBeforeRuntimeRejectsBareUnifiedDiffHunk(t *testing.T) {
	err := validatePatchStepInputBeforeRuntime("patch/apply", map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main_test.go",
		"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@\n-func makeApp(baseURL string) *App {\n+func makeApp(baseURL string) *Gateway {\n",
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "explicit line ranges") {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %v, want explicit hunk range rejection", err)
	}
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %T, want diagnostics", err)
	}
	diagnostics := diagnostic.RuntimeFailureDiagnostics()
	if diagnostics["errorDetail"] != "unified_diff_missing_line_ranges" || diagnostics["targetRef"] != "main_test.go" {
		t.Fatalf("diagnostics = %+v, want missing line range target diagnostic", diagnostics)
	}
}

func TestParseSingleFileApplyPatchDiffAllowsBareHunkHeader(t *testing.T) {
	patch, err := parseSingleFileUnifiedDiff("*** Begin Patch\n*** Update File: main_test.go\n@@\n-func makeApp(baseURL string) *App {\n+func makeApp(baseURL string) *Gateway {\n*** End Patch\n")
	if err != nil {
		t.Fatalf("parseSingleFileUnifiedDiff apply patch style err = %v, want allowed", err)
	}
	if patch.NewRef != "main_test.go" || len(patch.Hunks) != 1 || patch.Hunks[0].HasOldStart {
		t.Fatalf("patch = %+v, want apply-patch style bare hunk", patch)
	}
}

func TestValidatedCompletionDoDAssessmentCoversCurrentDoDWithMarkdownNormalization(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-dod-coverage-complete",
		WorkspaceID:     "ws-dod-coverage-complete",
		DoDID:           "dod-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{
					"### Acceptance",
					"- [ ] **build passes**",
					"- [ ] **error path covered**",
				},
			},
		},
	}
	_, summary, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "all current DoD items passed",
			"items": []map[string]any{
				{
					"item":         "Build passes with go test",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-complete/step-1"},
				},
				{
					"item":         "Error path covered by regression test",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-complete/step-2"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if summary != "all current DoD items passed" {
		t.Fatalf("summary = %q, want normalized summary", summary)
	}
}
