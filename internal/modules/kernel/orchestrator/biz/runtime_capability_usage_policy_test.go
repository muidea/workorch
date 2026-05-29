package orchestrator

import (
	"context"
	"strings"
	"testing"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestValidateLLMCapabilityUsagePolicyRequiresDependentMCPFollowUpAfterPatch(t *testing.T) {
	run := capabilityPolicyTestRun()
	step := capabilityPolicyTestStep()
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				capabilityPolicyPatchNode(),
				{
					ID:        "verify",
					Kind:      "shell/command",
					DependsOn: []string{"patch"},
					Input:     map[string]any{"command": "go", "args": []string{"test", "./..."}},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "capability usage policy") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want capability usage policy rejection", err)
	}
}

func TestValidateLLMCapabilityUsagePolicyAllowsDependentMCPFollowUpAfterPatch(t *testing.T) {
	run := capabilityPolicyTestRun()
	step := capabilityPolicyTestStep()
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				capabilityPolicyPatchNode(),
				{
					ID:        "semantic-diagnostics",
					Kind:      "mcp/tool_call",
					DependsOn: []string{"patch"},
					Input: map[string]any{
						"serverId": "mcp-code",
						"tool":     "semantic.diagnostics",
						"input":    map[string]any{"path": "main.go"},
					},
				},
				{
					ID:        "verify",
					Kind:      "shell/command",
					DependsOn: []string{"semantic-diagnostics"},
					Input:     map[string]any{"command": "go", "args": []string{"test", "./..."}},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want dependent MCP follow-up allowed", err)
	}
}

func TestValidateLLMCapabilityUsagePolicyAllowsStructuredSkipReason(t *testing.T) {
	run := capabilityPolicyTestRun()
	step := capabilityPolicyTestStep()
	output := map[string]any{
		"capabilityPolicySkips": []map[string]any{{
			"policyId":       "code-semantics-policy",
			"category":       "code_semantics",
			"reason":         "build_output_sufficient",
			"fallbackPolicy": "fallback_to_file_read_and_build_output_without_blocking",
		}},
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				capabilityPolicyPatchNode(),
				{
					ID:        "verify",
					Kind:      "shell/command",
					DependsOn: []string{"patch"},
					Input:     map[string]any{"command": "go", "args": []string{"test", "./..."}},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want structured skip reason allowed", err)
	}
}

func TestValidateLLMCapabilityUsagePolicySkipDoesNotForcePatchFailedSurfaceToGraphContract(t *testing.T) {
	run := capabilityPolicyTestRun()
	step := capabilityPolicyTestStep()
	step.ID = common.StepID("run-capability-policy/repair-001")
	step.Input[stepInputKeyRuntimeStage] = runtimeStageRepair
	step.Input["repairObjective"] = repairObjectiveStateChangingOutput
	step.Input["stateChangingOutputRequired"] = true
	step.Input["capabilityPolicyRepairRequired"] = true
	step.Input["coverageTargetSet"] = []map[string]any{{
		"coverageTargetToken": "source_change",
		"targetText":          "source_change",
	}}
	step.Input["capabilityPolicyRepairContract"] = map[string]any{
		"triggeredPolicies": []map[string]any{{
			"id":                 "code-semantics-policy",
			"appliesToCategory":  "code_semantics",
			"preferredTools":     []string{"semantic.diagnostics"},
			"requiredAfterKinds": []string{"patch/apply"},
		}},
	}
	step.Input["strategyReassessment"] = map[string]any{
		"required":      true,
		"failedSurface": `invalid argument: executionGraph triggered capability usage policy "code-semantics-policy" after "patch/apply" but returned no dependent matching mcp/tool_call or structured capabilityPolicySkips reason`,
		"requiredDecisionFields": []string{
			"failedSurface",
			"rootCauseTarget",
			"repairTargetDecision",
			"repairMode",
			"forbiddenRepair",
			"verificationPlan",
		},
	}
	patch := capabilityPolicyPatchNode()
	patch.Input["coverageTargets"] = []string{"source_change"}
	patch.Input["failedSurface"] = "workspace has no deliverable files"
	patch.Input["rootCauseTarget"] = "workspace root"
	patch.Input["repairTargetDecision"] = "create the smallest deliverable state change for the current coverage item"
	patch.Input["repairMode"] = "file_write creation"
	patch.Input["forbiddenRepair"] = "do not write unrelated marker files"
	patch.Input["verificationPlan"] = "run bounded validation for source_change"
	output := map[string]any{
		"capabilityPolicySkips": []map[string]any{{
			"policyId": "code-semantics-policy",
			"category": "code_semantics",
			"reason":   "current graph creates initial files and will use bounded build output as the fallback semantic signal",
		}},
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				patch,
				{
					ID:        "verify",
					Kind:      "shell/command",
					DependsOn: []string{"patch"},
					Input:     map[string]any{"command": "go", "args": []string{"test", "./..."}},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want capability-policy graph repair separated from patch failedSurface", err)
	}
}

func TestCapabilityUsagePoliciesForStepDeduplicatesByPolicyID(t *testing.T) {
	run := capabilityPolicyTestRun()
	step := capabilityPolicyTestStep()
	step.Input["executionContract"] = run.ExecutionContext["executionContract"]

	policies := capabilityUsagePoliciesForStep(run, step)
	if len(policies) != 1 {
		t.Fatalf("capabilityUsagePoliciesForStep len = %d, want 1: %#v", len(policies), policies)
	}
}

func TestRuntimeRepairCapabilityPolicyViolationRequiresCorrectedGraphOrSkip(t *testing.T) {
	run := capabilityPolicyTestRun()
	failed := capabilityPolicyTestStep()
	failed.State = common.StepStateFailed
	failed.Output = map[string]any{
		"stage":               "llm_execution_graph_plan",
		failureOutputKeyClass: failureClassRuntime,
		"error":               `invalid argument: executionGraph triggered capability usage policy "code-semantics-policy" after "patch/apply" but returned no dependent matching mcp/tool_call or structured capabilityPolicySkips reason`,
		"missingDoDItems":     []map[string]any{{"item": "preserve policy follow-up coverage", "state": "failed"}},
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{
			Nodes: []runtimemodel.BlockExecutionNode{
				capabilityPolicyPatchNode(),
				{
					ID:        "verify",
					Kind:      "shell/command",
					DependsOn: []string{"patch"},
					Input:     map[string]any{"command": "go", "args": []string{"test", "./..."}},
				},
			},
		},
	}

	input := (&RuntimeOrchestrator{steps: memory.NewStepRepository()}).runtimeRepairFollowUpStepInput(context.Background(), run, failed, 2, common.ErrInvalidArgument)

	if input[stepInputKeyAssessmentOnly] != nil {
		t.Fatalf("repair input assessmentOnly = %#v, want capability policy repair to stay executable", input[stepInputKeyAssessmentOnly])
	}
	if input["capabilityPolicyRepairRequired"] != true {
		t.Fatalf("repair input = %#v, want capabilityPolicyRepairRequired", input)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if pkg["capabilityPolicyRepairRequired"] != true {
		t.Fatalf("repairActionPackage = %#v, want capability policy repair contract", pkg)
	}
	contract := mapFromValue(pkg["capabilityPolicyRepairContract"])
	policies := mapSliceFromValue(contract["triggeredPolicies"])
	if len(policies) != 1 || policies[0]["id"] != "code-semantics-policy" {
		t.Fatalf("capabilityPolicyRepairContract = %#v, want triggered neutral policy", contract)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustRepairCapabilityPolicyFollowUp"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "mcp/tool_call") ||
		!stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "assessment_only_capability_policy_repair") {
		t.Fatalf("requiredOutput = %#v, want graph follow-up or structured skip contract", required)
	}
	if pkg["evidenceAcquisitionRequired"] == true {
		t.Fatalf("repairActionPackage = %#v, capability policy graph repair must not be downgraded to evidence acquisition", pkg)
	}
}

func capabilityPolicyTestRun() *runmodel.Run {
	return &runmodel.Run{
		ID:              common.RunID("run-capability-policy"),
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"capabilityUsagePolicies": []map[string]any{{
					"id":                 "code-semantics-policy",
					"appliesToCategory":  "code_semantics",
					"usageMode":          "prefer_when_triggered",
					"triggerStages":      []string{runtimeStageExecution, runtimeStageRepair},
					"preferredTools":     []string{"semantic.diagnostics"},
					"requiredAfterKinds": []string{"patch/apply"},
					"fallbackPolicy":     "fallback_to_file_read_and_build_output_without_blocking",
					"evidenceStrength":   "auxiliary",
				}},
			},
			"mcpCapabilities": []map[string]any{{
				"id":       "mcp-code:tool:semantic.diagnostics",
				"serverId": "mcp-code",
				"kind":     "tool",
				"name":     "semantic.diagnostics",
				"category": "code_semantics",
				"enabled":  true,
				"status":   "active",
			}},
		},
	}
}

func capabilityPolicyTestStep() *runmodel.Step {
	return &runmodel.Step{
		ID:    common.StepID("run-capability-policy/step-1"),
		RunID: common.RunID("run-capability-policy"),
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
		},
	}
}

func capabilityPolicyPatchNode() runtimemodel.BlockExecutionNode {
	return runtimemodel.BlockExecutionNode{
		ID:   "patch",
		Kind: "patch/apply",
		Input: map[string]any{
			"patchType": "text_replace",
			"targetRef": "main.go",
			"oldText":   "old",
			"newText":   "new",
		},
	}
}
