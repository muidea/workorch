package orchestrator

import (
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDegradedRuntimeFailureOutputAllowsAuxiliarySemanticMCPFallback(t *testing.T) {
	run := auxiliarySemanticMCPRun()
	step := auxiliarySemanticMCPStep()
	output, ok := degradedRuntimeFailureOutput(run, step, map[string]any{
		"status":   "failed",
		"stage":    "mcp_tool_call_failed",
		"serverId": "mcp-lsp-bridge",
		"tool":     "lsp_initialize",
		"error":    "typescript language server is not installed",
	}, errors.New("mcp tool lsp_initialize failed: typescript language server is not installed"))

	if !ok {
		t.Fatalf("degradedRuntimeFailureOutput ok = false, want auxiliary semantic MCP degradation")
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
}

func TestDegradedRuntimeFailureOutputRequiresFallbackContract(t *testing.T) {
	run := auxiliarySemanticMCPRun()
	run.ExecutionContext["executionContract"] = map[string]any{}
	step := auxiliarySemanticMCPStep()

	if output, ok := degradedRuntimeFailureOutput(run, step, map[string]any{
		"status":   "failed",
		"stage":    "mcp_tool_call_failed",
		"serverId": "mcp-lsp-bridge",
		"tool":     "lsp_initialize",
	}, errors.New("mcp tool failed")); ok {
		t.Fatalf("degradedRuntimeFailureOutput output=%+v ok=true, want no degradation without fallback contract", output)
	}
}

func auxiliarySemanticMCPRun() *runmodel.Run {
	return &runmodel.Run{
		ID:          "run-auxiliary-semantic-mcp",
		WorkspaceID: "ws-auxiliary-semantic-mcp",
		ExecutionContext: map[string]any{
			"mcpCapabilities": []map[string]any{{
				"id":       "mcp-lsp-bridge:tool:lsp_initialize",
				"serverId": "mcp-lsp-bridge",
				"kind":     "tool",
				"name":     "lsp_initialize",
				"category": "code_semantics",
			}},
			"executionContract": map[string]any{
				"capabilityUsagePolicies": []map[string]any{{
					"id":                "software-dev-code-semantics-guidance",
					"appliesToCategory": "code_semantics",
					"evidenceStrength":  "auxiliary",
					"usageMode":         "prefer_when_triggered",
					"fallbackPolicy":    "fallback_to_file_read_and_build_output_without_blocking",
				}},
			},
		},
	}
}

func auxiliarySemanticMCPStep() *runmodel.Step {
	return &runmodel.Step{
		ID:    "run-auxiliary-semantic-mcp/graph-001/001-init-lsp",
		RunID: "run-auxiliary-semantic-mcp",
		Kind:  "mcp/tool_call",
		State: common.StepStateDispatched,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "init-lsp",
			stepInputKeyGraphRootStepID: "run-auxiliary-semantic-mcp/root",
			"serverId":                  "mcp-lsp-bridge",
			"tool":                      "lsp_initialize",
		},
	}
}
