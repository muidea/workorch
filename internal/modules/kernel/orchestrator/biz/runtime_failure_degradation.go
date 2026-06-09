package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func degradedRuntimeFailureOutput(run *runModel.Run, step *runModel.Step, output map[string]any, execErr error) (map[string]any, bool) {
	if run == nil || step == nil || execErr == nil {
		return nil, false
	}
	failure := runtimeFailureOutput(output, execErr)
	classification := classifyRuntimeFailure(step, failure, "runtime step degraded", execErr)
	if !runtimeFailureCanDegrade(run, step, failure, classification) {
		return nil, false
	}
	attachRuntimeFailureClassification(failure, classification)
	stepWithFailure := *step
	stepWithFailure.Output = failure
	attachRuntimeFailureDiagnostics(&stepWithFailure, classification, execErr)
	ensureDegradedRuntimeDiagnostics(step, failure, classification, execErr)
	failure["status"] = "degraded"
	failure["capabilityDegraded"] = true
	failure["degradedReason"] = firstNonEmptyString(stringFromMap(failure, "error"), errorString(execErr))
	failure["evidenceQuality"] = string(common.EvidenceQualitySkipped)
	failure["evidenceReason"] = "auxiliary semantic MCP capability degraded; fallback to direct file/read, build/test, and validation evidence"
	failure["evidenceTags"] = []string{"auxiliary_capability_degraded", "semantic_mcp_fallback"}
	failure["zeroWork"] = true
	if strings.TrimSpace(stringFromMap(failure, "summary")) == "" {
		failure["summary"] = "auxiliary semantic MCP capability degraded and was skipped"
	}
	return failure, true
}

func ensureDegradedRuntimeDiagnostics(step *runModel.Step, output map[string]any, classification runtimeFailureClassification, execErr error) {
	if step == nil || output == nil || len(mapFromValue(output["runtimeDiagnostics"])) > 0 {
		return
	}
	diagnostics := map[string]any{
		"kind":             firstNonEmptyString(classification.Class, stringFromMap(output, failureOutputKeyClass)),
		"stage":            firstNonEmptyString(classification.Stage, stringFromMap(output, "stage")),
		"failureClass":     firstNonEmptyString(classification.Class, stringFromMap(output, failureOutputKeyClass)),
		"failureSignature": firstNonEmptyString(classification.Signature, stringFromMap(output, failureOutputKeySignature)),
		"stepId":           string(step.ID),
		"stepKind":         step.Kind,
		"error": firstNonEmptyString(
			stringFromMap(output, "error"),
			stringFromMap(output, "reason"),
			errorString(execErr),
		),
	}
	for _, key := range []string{"serverId", "tool", "uri", "prompt"} {
		if value := firstNonEmptyString(stringFromMap(output, key), stringFromMap(step.Input, key)); value != "" {
			diagnostics[key] = value
		}
	}
	output["runtimeDiagnostics"] = diagnostics
}

func runtimeFailureCanDegrade(run *runModel.Run, step *runModel.Step, output map[string]any, classification runtimeFailureClassification) bool {
	if run == nil || step == nil || strings.TrimSpace(step.Kind) != "mcp/tool_call" {
		return false
	}
	if graphNodeID(step) == "" {
		return false
	}
	if classification.Class != failureClassMCPRuntime {
		return false
	}
	serverID := firstNonEmptyString(stringFromMap(output, "serverId"), stringFromMap(step.Input, "serverId"))
	tool := firstNonEmptyString(stringFromMap(output, "tool"), stringFromMap(step.Input, "tool"))
	if strings.TrimSpace(serverID) == "" || strings.TrimSpace(tool) == "" {
		return false
	}
	category := runtimeMCPCapabilityCategory(run, serverID, tool)
	if category == "" && strings.TrimSpace(serverID) == "mcp-lsp-bridge" {
		category = "code_semantics"
	}
	if category != "code_semantics" {
		return false
	}
	return runtimeContractAllowsAuxiliaryCapabilityFallback(run, category)
}

func runtimeMCPCapabilityCategory(run *runModel.Run, serverID string, tool string) string {
	if run == nil || run.ExecutionContext == nil {
		return ""
	}
	serverID = strings.TrimSpace(serverID)
	tool = strings.TrimSpace(tool)
	if serverID == "" || tool == "" {
		return ""
	}
	for _, item := range mapSliceFromValue(run.ExecutionContext["mcpCapabilities"]) {
		if strings.TrimSpace(stringFromMap(item, "serverId")) != serverID {
			continue
		}
		name := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "name"), stringFromMap(item, "tool")))
		id := strings.TrimSpace(stringFromMap(item, "id"))
		if name != tool && !strings.Contains(id, serverID+":tool:"+tool) {
			continue
		}
		return strings.TrimSpace(stringFromMap(item, "category"))
	}
	return ""
}

func runtimeContractAllowsAuxiliaryCapabilityFallback(run *runModel.Run, category string) bool {
	if run == nil || run.ExecutionContext == nil {
		return false
	}
	category = strings.TrimSpace(category)
	if category == "" {
		return false
	}
	contract := mapFromValue(run.ExecutionContext["executionContract"])
	for _, policy := range mapSliceFromValue(contract["capabilityUsagePolicies"]) {
		if strings.TrimSpace(stringFromMap(policy, "appliesToCategory")) != category {
			continue
		}
		if strings.TrimSpace(stringFromMap(policy, "evidenceStrength")) != "auxiliary" {
			continue
		}
		fallback := strings.ToLower(strings.TrimSpace(stringFromMap(policy, "fallbackPolicy")))
		usageMode := strings.ToLower(strings.TrimSpace(stringFromMap(policy, "usageMode")))
		if strings.Contains(fallback, "fallback") ||
			strings.Contains(fallback, "without_blocking") ||
			strings.Contains(fallback, "without-blocking") ||
			strings.Contains(fallback, "non_blocking") ||
			strings.Contains(fallback, "non-blocking") ||
			strings.Contains(usageMode, "prefer") {
			return true
		}
	}
	return false
}
