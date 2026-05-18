package orchestrator

import (
	"sort"
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	maxLLMContextStringBytes       = 4096
	maxLLMContextEvidenceTextBytes = 1200
	maxLLMContextSliceItems        = 20
	maxLLMContextMapEntries        = 32
)

func compactStepInputForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := compactLLMContextMap(input, 0)
	delete(ret, "executionContext")
	delete(ret, "failedStepInput")
	delete(ret, "failedStepOutput")
	delete(ret, "previousStepOutput")
	delete(ret, stepInputKeyExecutionContextPackage)
	delete(ret, stepInputKeyWorkspaceFileManifest)
	delete(ret, stepOutputKeyExecutionGraph)
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		ret["repairActionPackage"] = compactLLMContextMap(pkg, 0)
	}
	return ret
}

func compactReplanContinuationForRuntimeContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"predecessorRunId",
		"reason",
		"ticketId",
		"ticketStepId",
		"ticketReason",
		"resolutionId",
		"resolutionType",
		"recoveryAction",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if payload := mapFromValue(input["resolutionPayload"]); len(payload) > 0 {
		ret["resolutionPayload"] = compactDecisionPayloadForRuntimeContext(payload)
	}
	if failure := mapFromValue(input["failureContext"]); len(failure) > 0 {
		ret["failureContext"] = compactFailureContextForRuntimeContext(failure)
	}
	if ticketContext := mapFromValue(input["ticketContext"]); len(ticketContext) > 0 {
		ret["ticketContext"] = compactFailureContextForRuntimeContext(ticketContext)
	}
	return stripRuntimeLLMBinding(ret)
}

func compactRejectedResultContinuationForRuntimeContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"mode",
		"predecessorRunId",
		"resultId",
		"resultState",
		"sourceType",
		"sourceRef",
		"summary",
		"goalOutcome",
		"satisfiedDoDItems",
		"unsatisfiedDoDItems",
		"coverageGaps",
		"requiredEvidence",
		"resultEvidenceRefs",
		"artifactRefs",
		"summaryEvidenceRefs",
		"summaryArtifactRefs",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	return stripRuntimeLLMBinding(common.RedactSensitiveMap(ret))
}

func compactGovernanceHistoryForRuntimeContext(input []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(input) == 0 {
		return nil
	}
	if len(input) > limit {
		input = input[:limit]
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		if compacted := compactGovernanceEntryForRuntimeContext(item); len(compacted) > 0 {
			ret = append(ret, common.RedactSensitiveMap(compacted))
		}
	}
	return ret
}

func compactGovernanceEntryForRuntimeContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"id",
		"kind",
		"state",
		"summary",
		"reason",
		"question",
		"ticketId",
		"runId",
		"stepId",
		"ticketStepId",
		"decisionKind",
		"resolutionId",
		"resolutionType",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if context := mapFromValue(input["ticketContext"]); len(context) > 0 {
		ret["ticketContext"] = compactFailureContextForRuntimeContext(context)
	} else if context := mapFromValue(input["context"]); len(context) > 0 {
		ret["context"] = compactFailureContextForRuntimeContext(context)
	}
	if payload := mapFromValue(input["resolutionPayload"]); len(payload) > 0 {
		ret["resolutionPayload"] = compactDecisionPayloadForRuntimeContext(payload)
	}
	return stripRuntimeLLMBinding(ret)
}

func compactDecisionPayloadForRuntimeContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"answer",
		"note",
		"operator",
		"recoveryAction",
		"resolution",
		"summary",
		"reason",
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"attempts",
		"maxAttempts",
		"repairOfRootStepId",
		"recoveryPolicy",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{"failureContext", "ticketContext", "context"} {
		if context := mapFromValue(input[key]); len(context) > 0 {
			ret[key] = compactFailureContextForRuntimeContext(context)
		}
	}
	return stripRuntimeLLMBinding(ret)
}

func compactFailureContextForRuntimeContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"failureReason",
		"repairObjective",
		"attempts",
		"maxAttempts",
		"repairAttempts",
		"repairOfRootStepId",
		"runtimeRepairAttempt",
		"maxRuntimeRepairAttempts",
		"recoveryPolicy",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if failedOutput := mapFromValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		ret["failedStepOutput"] = compactStepOutputForLLMContext(failedOutput)
	}
	if targets := compactRepairTargetSnapshotsForLineage(input); len(targets) > 0 {
		ret["targetSnapshots"] = targets
	}
	return stripRuntimeLLMBinding(ret)
}

func compactRepairTargetSnapshotsForLineage(input map[string]any) []map[string]any {
	var ret []map[string]any
	seen := map[string]bool{}
	add := func(items []map[string]any, source string) {
		for _, item := range items {
			targetRef := strings.TrimSpace(stringFromMap(item, "targetRef"))
			if targetRef == "" || seen[targetRef] {
				continue
			}
			seen[targetRef] = true
			snapshot := map[string]any{}
			for _, key := range []string{
				"targetRef",
				"exists",
				"contentHash",
				"contentComplete",
				"safeForFileWrite",
				"redacted",
				"truncated",
				"isBinary",
				"size",
				"source",
				"error",
			} {
				if value, ok := item[key]; ok && value != nil {
					snapshot[key] = compactLLMContextValue(value, 1)
				}
			}
			if source != "" {
				snapshot["contextSource"] = source
			}
			ret = append(ret, snapshot)
			if len(ret) >= maxLLMContextSliceItems {
				return
			}
		}
	}
	add(mapSliceFromValue(input["targetSnapshots"]), "lineage_target")
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["failureTargetRepairContext"]), "failure_target")
	}
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["patchRepairContext"]), "patch_target")
	}
	return ret
}

func compactStepOutputForLLMContext(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	if llmDecisionInvalidOutput(output) {
		return stripRuntimeLLMBinding(compactLLMDecisionInvalidOutputForContext(output))
	}
	ret := compactLLMContextMap(output, 0)
	return stripRuntimeLLMBinding(ret)
}

func compactWorkspaceFileManifest(manifest []map[string]any) []map[string]any {
	if len(manifest) == 0 {
		return nil
	}
	limit := min(len(manifest), maxLLMContextSliceItems)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		ret = append(ret, compactLLMContextMap(manifest[idx], 0))
	}
	return ret
}

func compactExecutionGraphForLLMContext(graph runtimemodel.BlockExecutionGraph) runtimemodel.BlockExecutionGraph {
	if len(graph.Nodes) == 0 {
		return graph
	}
	nodes := make([]runtimemodel.BlockExecutionNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		compactNode := runtimemodel.BlockExecutionNode{
			ID:   strings.TrimSpace(node.ID),
			Kind: strings.TrimSpace(node.Kind),
		}
		if len(node.DependsOn) > 0 {
			limit := min(len(node.DependsOn), 10)
			compactNode.DependsOn = append([]string(nil), node.DependsOn[:limit]...)
		}
		if branch := strings.TrimSpace(node.Branch); branch != "" {
			compactNode.Branch = truncateLLMContextString(branch)
		}
		if len(node.Input) > 0 {
			compactNode.Input = compactLLMContextMap(node.Input, 0)
		}
		nodes = append(nodes, compactNode)
	}
	return runtimemodel.BlockExecutionGraph{Nodes: nodes}
}

func compactLLMContextMap(input map[string]any, depth int) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, min(len(input), maxLLMContextMapEntries))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(ret) >= maxLLMContextMapEntries {
			break
		}
		if shouldSkipLLMContextKey(key) {
			continue
		}
		if isLargeEvidenceTextKey(key) {
			ret[key] = compactLLMContextEvidenceText(input[key])
			continue
		}
		ret[key] = compactLLMContextValue(input[key], depth+1)
	}
	return ret
}

func compactLLMContextValue(value any, depth int) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return compactLLMContextString(v)
	case map[string]any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(v)
		}
		return compactLLMContextMap(v, depth)
	case []map[string]any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(map[string]any{"items": len(v)})
		}
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactLLMContextMap(v[idx], depth))
		}
		return ret
	case []any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(map[string]any{"items": len(v)})
		}
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactLLMContextValue(v[idx], depth))
		}
		return ret
	case []string:
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactLLMContextString(v[idx]))
		}
		return ret
	default:
		return v
	}
}

func compactLLMContextLeafSummary(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, min(len(input), maxLLMContextMapEntries))
	for key, value := range input {
		if shouldSkipLLMContextKey(key) {
			continue
		}
		switch v := value.(type) {
		case string:
			ret[key] = compactLLMContextString(v)
		case []string:
			ret[key] = len(v)
		case []any:
			ret[key] = len(v)
		case []map[string]any:
			ret[key] = len(v)
		case map[string]any:
			ret[key] = len(v)
		default:
			ret[key] = v
		}
		if len(ret) >= maxLLMContextMapEntries {
			break
		}
	}
	return ret
}

func llmDecisionInvalidOutput(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	switch strings.TrimSpace(stringFromMap(output, "stage")) {
	case "llm_schema_invalid", "llm_decision_invalid":
		return true
	}
	return strings.TrimSpace(stringFromMap(output, failureOutputKeyClass)) == failureClassLLMDecisionInvalid
}

func compactLLMDecisionInvalidOutputForContext(output map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range []string{
		"stage",
		"status",
		"error",
		"reason",
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureCategory",
	} {
		if value, ok := output[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{"streamDiagnostics", "runtimeDiagnostics", "contextBudget", "llmUsage"} {
		if value := mapFromValue(output[key]); len(value) > 0 {
			ret[key] = compactLLMContextMap(value, 0)
		}
	}
	if _, ok := output["rawContentPreview"]; ok {
		ret["rawContentPreviewOmitted"] = true
	}
	if _, ok := output["repairRawContentPreview"]; ok {
		ret["repairRawContentPreviewOmitted"] = true
	}
	if len(mapSliceFromValue(output["streamEvents"])) > 0 {
		ret["streamEventsOmitted"] = true
	}
	if ret["rawContentPreviewOmitted"] == true ||
		ret["repairRawContentPreviewOmitted"] == true ||
		ret["streamEventsOmitted"] == true {
		ret["omittedContentReason"] = "invalid or rejected LLM decision content is not reusable repair input; rebuild from current context, target snapshots, or fresh reads"
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		ret["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	} else if graph, ok := runtimeGraphSpecFromValue(mapFromValue(output["llmDecision"])[stepOutputKeyExecutionGraph]); ok {
		ret["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	}
	return ret
}

func compactLLMContextString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if containsRuntimeRedactionMarker(value) {
		return redactedRuntimeContentOmittedForLLMContext(value)
	}
	return truncateLLMContextString(value)
}

func redactedRuntimeContentOmittedForLLMContext(value string) map[string]any {
	value = strings.TrimSpace(value)
	ret := map[string]any{
		"omitted": true,
		"reason":  "redacted runtime marker content is not reusable repair input; rebuild from current target snapshots or fresh reads",
		"bytes":   len(value),
	}
	if value != "" {
		ret["containedRuntimeRedactionMarker"] = true
	}
	return ret
}

func shouldSkipLLMContextKey(key string) bool {
	switch key {
	case "executionContext",
		"failedStepInput",
		"failedStepOutput",
		"llmProviderId",
		"previousStepOutput",
		"provider",
		"providerId",
		stepInputKeyExecutionContextPackage,
		stepInputKeyWorkspaceFileManifest:
		return true
	default:
		return false
	}
}

func truncateLLMContextString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLLMContextStringBytes {
		return value
	}
	return value[:maxLLMContextStringBytes] + "..."
}

func isLargeEvidenceTextKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "stdout", "stderr":
		return true
	default:
		return false
	}
}

func compactLLMContextEvidenceText(value any) any {
	text, ok := value.(string)
	if !ok {
		return compactLLMContextValue(value, 1)
	}
	text = strings.TrimSpace(text)
	if containsRuntimeRedactionMarker(text) {
		return redactedRuntimeContentOmittedForLLMContext(text)
	}
	if len(text) <= maxLLMContextEvidenceTextBytes {
		return text
	}
	return map[string]any{
		"textPreview": text[:maxLLMContextEvidenceTextBytes] + "...",
		"bytes":       len(text),
		"truncated":   true,
	}
}

func stripRuntimeLLMBinding(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	delete(input, "provider")
	delete(input, "providerId")
	delete(input, "llmProviderId")
	if budget := mapFromValue(input["contextBudget"]); len(budget) > 0 {
		delete(budget, "providerId")
		delete(budget, "model")
		delete(budget, "profile")
		input["contextBudget"] = budget
	}
	if usage := mapFromValue(input["llmUsage"]); len(usage) > 0 {
		delete(usage, "provider")
		delete(usage, "providerId")
		delete(usage, "model")
		if budget := mapFromValue(usage["contextBudget"]); len(budget) > 0 {
			delete(budget, "providerId")
			delete(budget, "model")
			delete(budget, "profile")
			usage["contextBudget"] = budget
		}
		input["llmUsage"] = usage
	}
	return input
}
