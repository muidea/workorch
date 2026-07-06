package orchestrator

import (
	"errors"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDegradedRuntimeFailureOutputAllowsPolicyDeclaredAuxiliaryMCPFallback(t *testing.T) {
	run := auxiliaryMCPFallbackRun()
	step := auxiliaryMCPFallbackStep()
	output, ok := degradedRuntimeFailureOutput(run, step, map[string]any{
		"status":   "failed",
		"stage":    "mcp_tool_call_failed",
		"serverId": "mcp-lsp-bridge",
		"tool":     "lsp_initialize",
		"error":    "typescript language server is not installed",
	}, errors.New("mcp tool lsp_initialize failed: typescript language server is not installed"))

	if !ok {
		t.Fatalf("degradedRuntimeFailureOutput ok = false, want policy-declared auxiliary MCP degradation")
	}
	if output["status"] != "degraded" ||
		output["capabilityDegraded"] != true ||
		output["evidenceQuality"] != string(common.EvidenceQualitySkipped) ||
		output["zeroWork"] != true {
		t.Fatalf("output = %+v, want degraded skipped zero-work evidence", output)
	}
	if output[failureOutputKeyClass] != failureClassMCPRuntime {
		t.Fatalf("failureClass = %#v, want %s", output[failureOutputKeyClass], failureClassMCPRuntime)
	}
	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	if diagnostics["serverId"] != "mcp-lsp-bridge" || diagnostics["tool"] != "lsp_initialize" {
		t.Fatalf("runtimeDiagnostics = %+v, want MCP call identity preserved", diagnostics)
	}
	tags := stringSliceFromValue(output["evidenceTags"])
	if !stringSliceContains(tags, "capability_fallback") || strings.Contains(strings.Join(tags, ","), "semantic") {
		t.Fatalf("evidenceTags = %+v, want neutral fallback tag only", tags)
	}
}

func TestDegradedRuntimeFailureOutputRequiresFallbackContract(t *testing.T) {
	run := auxiliaryMCPFallbackRun()
	run.ExecutionContext["executionContract"] = map[string]any{}
	step := auxiliaryMCPFallbackStep()

	if output, ok := degradedRuntimeFailureOutput(run, step, map[string]any{
		"status":   "failed",
		"stage":    "mcp_tool_call_failed",
		"serverId": "mcp-lsp-bridge",
		"tool":     "lsp_initialize",
	}, errors.New("mcp tool failed")); ok {
		t.Fatalf("degradedRuntimeFailureOutput output=%+v ok=true, want no degradation without fallback contract", output)
	}
}

func TestDegradedRuntimeFailureOutputUsesPolicyCategoryWithoutBusinessSpecialCase(t *testing.T) {
	run := auxiliaryMCPFallbackRun()
	capabilities := run.ExecutionContext["mcpCapabilities"].([]map[string]any)
	capabilities[0]["category"] = "workspace_index"
	contract := mapFromValue(run.ExecutionContext["executionContract"])
	policies := contract["capabilityUsagePolicies"].([]map[string]any)
	policies[0]["id"] = "workspace-index-auxiliary"
	policies[0]["appliesToCategory"] = "workspace_index"

	if _, ok := degradedRuntimeFailureOutput(run, auxiliaryMCPFallbackStep(), map[string]any{
		"status":   "failed",
		"stage":    "mcp_tool_call_failed",
		"serverId": "mcp-lsp-bridge",
		"tool":     "lsp_initialize",
	}, errors.New("mcp tool failed")); !ok {
		t.Fatal("degradedRuntimeFailureOutput ok = false, want fallback driven by policy category")
	}
}

func TestDegradedRuntimeFailureOutputDoesNotSkipRequiredValidationMCP(t *testing.T) {
	run := auxiliaryMCPFallbackRun()
	step := auxiliaryMCPFallbackStep()
	step.Input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	step.Input["boundedValidationRequired"] = true

	if output, ok := degradedRuntimeFailureOutput(run, step, map[string]any{
		"status":   "failed",
		"stage":    "mcp_tool_call_failed",
		"serverId": "mcp-lsp-bridge",
		"tool":     "lsp_initialize",
	}, errors.New("mcp tool failed")); ok {
		t.Fatalf("degradedRuntimeFailureOutput output=%+v ok=true, want required validation MCP to stay recoverable", output)
	}
}

func auxiliaryMCPFallbackRun() *runmodel.Run {
	return &runmodel.Run{
		ID:          "run-auxiliary-mcp-fallback",
		WorkspaceID: "ws-auxiliary-mcp-fallback",
		ExecutionContext: map[string]any{
			"mcpCapabilities": []map[string]any{{
				"id":       "mcp-lsp-bridge:tool:lsp_initialize",
				"serverId": "mcp-lsp-bridge",
				"kind":     "tool",
				"name":     "lsp_initialize",
				"category": "dependency_index",
			}},
			"executionContract": map[string]any{
				"capabilityUsagePolicies": []map[string]any{{
					"id":                "dependency-index-auxiliary",
					"appliesToCategory": "dependency_index",
					"evidenceStrength":  "auxiliary",
					"usageMode":         "prefer_when_triggered",
					"fallbackPolicy":    "fallback_without_blocking",
				}},
			},
		},
	}
}

func auxiliaryMCPFallbackStep() *runmodel.Step {
	return &runmodel.Step{
		ID:    "run-auxiliary-mcp-fallback/graph-001/001-init-lsp",
		RunID: "run-auxiliary-mcp-fallback",
		Kind:  "mcp/tool_call",
		State: common.StepStateExecuting,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "init-lsp",
			stepInputKeyGraphRootStepID: "run-auxiliary-semantic-mcp/root",
			"serverId":                  "mcp-lsp-bridge",
			"tool":                      "lsp_initialize",
		},
	}
}
