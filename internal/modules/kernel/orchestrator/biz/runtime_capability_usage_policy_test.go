package orchestrator

import (
	"strings"
	"testing"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
