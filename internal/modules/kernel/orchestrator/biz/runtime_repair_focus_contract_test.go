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

func TestRepairCoverageFocusContractSurvivesEvidenceRequiredOutputRebuild(t *testing.T) {
	input := repairFocusContractInput()
	pkg := map[string]any{"requiredOutput": stateChangingRepairRequiredOutput()}

	required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
	if !boolValue(required["mustBindCoverageFocus"]) {
		t.Fatalf("requiredOutput = %+v, want mustBindCoverageFocus preserved", required)
	}
	if got := stringFromMap(required, "coverageFocusId"); got != "focus-cwi-primary" {
		t.Fatalf("coverageFocusId = %q, want focus-cwi-primary", got)
	}
	if got := stringSliceFromValue(required["coverageWorkItemIds"]); len(got) != 1 || got[0] != "cwi-primary" {
		t.Fatalf("coverageWorkItemIds = %+v, want cwi-primary", got)
	}

	validationRequired := concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, required)
	if !boolValue(validationRequired["mustBindCoverageFocus"]) {
		t.Fatalf("validation requiredOutput = %+v, want mustBindCoverageFocus preserved", validationRequired)
	}
	if got := stringFromMap(validationRequired, "coverageFocusId"); got != "focus-cwi-primary" {
		t.Fatalf("validation coverageFocusId = %q, want focus-cwi-primary", got)
	}
}

func TestStateChangingConvergencePreservesCoverageFocusRequiredOutput(t *testing.T) {
	input := repairFocusContractInput()
	input["repairActionPackage"] = map[string]any{
		"coverageFocusBindingRequired": true,
		coverageFocusKey:               cloneStepPayload(mapFromValue(input[coverageFocusKey])),
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode": true,
			"acceptedKinds":                []string{"patch/apply", "shell/command", "mcp/tool_call"},
		},
	}

	pkg := stateChangingRepairActionPackageForConvergence(input, nil)
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustBindCoverageFocus"]) {
		t.Fatalf("requiredOutput = %+v, want mustBindCoverageFocus after state-changing convergence", required)
	}
	if got := stringFromMap(required, "coverageFocusId"); got != "focus-cwi-primary" {
		t.Fatalf("coverageFocusId = %q, want focus-cwi-primary", got)
	}
	if got := stringSliceFromValue(required["coverageWorkItemIds"]); len(got) != 1 || got[0] != "cwi-primary" {
		t.Fatalf("coverageWorkItemIds = %+v, want cwi-primary", got)
	}
}

func TestValidateLLMRepairCoverageFocusAcceptsStateChangingReasonBinding(t *testing.T) {
	run := repairFocusContractRun()
	step := repairFocusContractStep()
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				{
					ID:   "change-primary",
					Kind: "shell/command",
					Input: map[string]any{
						"command":             "change-primary",
						"stateChanging":       true,
						"targetRef":           "primary-resource",
						"stateChangingReason": "collect validation evidence for focus-cwi-primary",
					},
				},
				{
					ID:        "verify-primary",
					Kind:      "shell/command",
					DependsOn: []string{"change-primary"},
					Input: map[string]any{
						"command":           "verify-primary",
						"verificationScope": "focus-cwi-primary",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want stateChangingReason focus binding allowed", err)
	}
}

func TestValidateLLMRepairCoverageFocusNormalizesEvidenceGraphBinding(t *testing.T) {
	run := repairFocusContractRun()
	step := repairFocusEvidenceContractStep()
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				{
					ID:   "read-primary",
					Kind: "file/read",
					Input: map[string]any{
						"path": "primary-resource",
					},
				},
				{
					ID:        "validate-primary",
					Kind:      "shell/command",
					DependsOn: []string{"read-primary"},
					Input: map[string]any{
						"command": "validate-primary",
						"validationEvidence": map[string]any{
							"mode":            "executable_contract",
							"observationType": "executable_contract_observation",
						},
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want normalized evidence graph allowed", err)
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		t.Fatalf("normalized executionGraph missing from output")
	}
	if got := stringFromMap(graph.Nodes[1].Input, "coverageFocusId"); got != "focus-cwi-primary" {
		t.Fatalf("coverageFocusId = %q, want focus-cwi-primary", got)
	}
	if got := stringSliceFromValue(graph.Nodes[1].Input["coverageWorkItemIds"]); len(got) != 1 || got[0] != "cwi-primary" {
		t.Fatalf("coverageWorkItemIds = %+v, want cwi-primary", got)
	}
	if got := stringFromMap(graph.Nodes[0].Input, "coverageFocusId"); got != "" {
		t.Fatalf("read-only support node coverageFocusId = %q, want not injected", got)
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

func repairFocusEvidenceContractStep() *runmodel.Step {
	input := repairFocusContractInput()
	delete(input, "stateChangingOutputRequired")
	input["repairObjective"] = repairObjectiveAcquireFailureEvidence
	input["concreteFailureEvidenceRequired"] = true
	input["repairActionPackage"] = repairActionPackageFromRepairInput(input)
	pkg := mapFromValue(input["repairActionPackage"])
	pkg["evidenceAcquisitionRequired"] = true
	required := mapFromValue(pkg["requiredOutput"])
	required["mustProduceConcreteValidationEvidence"] = true
	required["acceptedKinds"] = []string{"file/read", "file/list", "shell/command", "mcp/tool_call"}
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	return &runmodel.Step{
		ID:    "run-repair-focus-contract/repair-evidence-001",
		RunID: "run-repair-focus-contract",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateDispatched,
		Input: input,
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
