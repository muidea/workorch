package orchestrator

import (
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRepairActionPackageDeclaresCoverageFocusBinding(t *testing.T) {
	input := repairFocusContractInput()

	pkg := repairActionPackageFromRepairInput(input)
	if len(pkg) == 0 {
		t.Fatalf("repairActionPackage is empty")
	}
	focus := mapFromValue(pkg[coverageFocusKey])
	if stringFromMap(focus, "id") != "focus-cwi-primary" {
		t.Fatalf("repairActionPackage.coverageFocus = %+v, want current focus", focus)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustBindCoverageFocus"]) {
		t.Fatalf("requiredOutput = %+v, want mustBindCoverageFocus", required)
	}
	if stringFromMap(required, "coverageFocusId") != "focus-cwi-primary" {
		t.Fatalf("requiredOutput.coverageFocusId = %q", stringFromMap(required, "coverageFocusId"))
	}
	if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-primary" {
		t.Fatalf("requiredOutput.coverageWorkItemIds = %+v, want cwi-primary", ids)
	}
}

func TestValidateLLMRepairCoverageFocusRejectsUnboundPatchGraph(t *testing.T) {
	run := repairFocusContractRun()
	step := repairFocusContractStep()
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{{
				ID:   "change-secondary",
				Kind: "patch/apply",
				Input: map[string]any{
					"targetRef":       "secondary-resource",
					"patchType":       "file_write",
					"content":         "secondary update",
					"baseHash":        "hash-secondary",
					"coverageTargets": []string{"unrelated closure"},
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "repair_focus_mismatch") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want repair_focus_mismatch", err)
	}
}

func TestValidateLLMRepairCoverageFocusAllowsCurrentCoverageTargetBinding(t *testing.T) {
	run := repairFocusContractRun()
	step := repairFocusContractStep()
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{{
				ID:   "change-primary",
				Kind: "patch/apply",
				Input: map[string]any{
					"targetRef":       "primary-resource",
					"patchType":       "file_write",
					"content":         "primary update",
					"baseHash":        "hash-primary",
					"coverageTargets": []string{"primary closure"},
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want current coverage target binding allowed", err)
	}
}

func TestValidateLLMRepairCoverageFocusAllowsBoundPatchGraph(t *testing.T) {
	run := repairFocusContractRun()
	step := repairFocusContractStep()
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				{
					ID:   "change-primary",
					Kind: "patch/apply",
					Input: map[string]any{
						"targetRef":           "primary-resource",
						"patchType":           "file_write",
						"content":             "primary update",
						"baseHash":            "hash-primary",
						"coverageFocusId":     "focus-cwi-primary",
						"coverageWorkItemIds": []string{"cwi-primary"},
						"coverageTargets":     []string{"primary closure"},
					},
				},
				{
					ID:        "validate-primary",
					Kind:      "shell/command",
					DependsOn: []string{"change-primary"},
					Input: map[string]any{
						"command":            "validate-primary",
						"coverageFocusId":    "focus-cwi-primary",
						"coverageWorkItemId": "cwi-primary",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want focus-bound graph allowed", err)
	}
}

func TestValidateLLMRepairCoverageFocusAllowsStructuredBlocker(t *testing.T) {
	run := repairFocusContractRun()
	step := repairFocusContractStep()
	output := map[string]any{
		"completionState": "cannot_progress",
	}

	if err := validateLLMRepairCoverageFocusBindingContract(step, output); err != nil {
		t.Fatalf("validateLLMRepairCoverageFocusBindingContract err = %v, want blocker allowed", err)
	}
	if err := validateLLMExecutionOutputContract(run, step, map[string]any{
		stepOutputKeyBlockDecisionReason:   "current focus cannot continue without external input",
		stepOutputKeyBlockDecisionKind:     "cannot_progress",
		stepOutputKeyBlockDecisionQuestion: "Provide missing input for the current focus.",
	}); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract decision blocker err = %v", err)
	}
}

func repairFocusContractRun() *runmodel.Run {
	return &runmodel.Run{
		ID:              "run-repair-focus-contract",
		WorkspaceID:     "ws-repair-focus-contract",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
}

func repairFocusContractStep() *runmodel.Step {
	input := repairFocusContractInput()
	input["repairActionPackage"] = repairActionPackageFromRepairInput(input)
	return &runmodel.Step{
		ID:    "run-repair-focus-contract/repair-001",
		RunID: "run-repair-focus-contract",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateDispatched,
		Input: input,
	}
}

func repairFocusContractInput() map[string]any {
	return map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		coverageFocusKey: map[string]any{
			"id":                    "focus-cwi-primary",
			"coverageWorkItemIds":   []string{"cwi-primary"},
			"requiredEvidenceKinds": []string{"state_change", "validation"},
			"targetRefs":            []string{"primary-resource"},
			"state":                 "active",
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                   "cwi-primary",
				"targetText":           "primary closure",
				"missingEvidenceKinds": []string{"state_change", "validation"},
				"targetRefs":           []string{"primary-resource"},
			},
			"unresolvedItems": []map[string]any{
				{
					"id":                   "cwi-primary",
					"targetText":           "primary closure",
					"missingEvidenceKinds": []string{"state_change", "validation"},
					"targetRefs":           []string{"primary-resource"},
				},
				{
					"id":                   "cwi-secondary",
					"targetText":           "secondary closure",
					"missingEvidenceKinds": []string{"state_change"},
					"targetRefs":           []string{"secondary-resource"},
				},
			},
		},
		"coverageTargetSet": []map[string]any{
			{
				"kind":                "acceptance",
				"coverageWorkItemId":  "cwi-primary",
				"coverageTargetToken": "primary closure",
				"targetText":          "primary closure",
			},
			{
				"kind":                "acceptance",
				"coverageWorkItemId":  "cwi-secondary",
				"coverageTargetToken": "secondary closure",
				"targetText":          "secondary closure",
			},
		},
		"targetSnapshots": []map[string]any{
			{
				"targetRef":        "primary-resource",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-primary",
				"content":          "primary original",
			},
			{
				"targetRef":        "secondary-resource",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-secondary",
				"content":          "secondary original",
			},
		},
	}
}
