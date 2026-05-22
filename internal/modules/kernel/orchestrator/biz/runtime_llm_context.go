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
		ret["repairActionPackage"] = compactRepairActionPackageForLLMContext(pkg)
	}
	return ret
}

func compactRepairActionPackageForLLMContext(pkg map[string]any) map[string]any {
	if len(pkg) == 0 {
		return nil
	}
	ret := compactLLMContextMap(pkg, 0)
	if targets := compactRepairActionTargetSnapshotsForLLMContext(mapSliceFromValue(pkg["targetSnapshots"])); len(targets) > 0 {
		ret["targetSnapshots"] = targets
	}
	return ret
}

func compactRepairActionTargetSnapshotsForLLMContext(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	limit := min(len(items), maxLLMContextSliceItems)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		item := items[idx]
		next := map[string]any{}
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
			"contextSource",
			"error",
		} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		content := strings.TrimSpace(stringFromMap(item, "content"))
		if content != "" && !containsRuntimeRedactionMarker(content) &&
			boolValue(item["contentComplete"]) && boolValue(item["safeForFileWrite"]) &&
			!boolValue(item["redacted"]) && !boolValue(item["truncated"]) && !boolValue(item["isBinary"]) &&
			len(content) <= maxPatchRepairSnapshotBytes {
			next["content"] = content
		} else if preview := strings.TrimSpace(stringFromMap(item, "contentPreview")); preview != "" && !containsRuntimeRedactionMarker(preview) {
			next["contentPreview"] = truncateLLMContextString(preview)
		} else if content != "" && !containsRuntimeRedactionMarker(content) {
			next["contentPreview"] = truncateLLMContextString(content)
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
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
	if timeline := mapSliceFromValue(input[stepInputKeyProcessEvidenceTimeline]); len(timeline) > 0 {
		ret[stepInputKeyProcessEvidenceTimeline] = compactProcessEvidenceTimelineForRuntimeContext(timeline)
	}
	if digest := mapFromValue(input[stepInputKeyCurrentProgressDigest]); len(digest) > 0 {
		ret[stepInputKeyCurrentProgressDigest] = compactLLMContextValue(digest, 2)
	}
	return stripRuntimeLLMBinding(ret)
}

func compactHistoricalReplanContinuationForRuntimeContext(input map[string]any) map[string]any {
	ret := compactReplanContinuationForRuntimeContext(input)
	if len(ret) == 0 {
		return nil
	}
	ret["historicalOnly"] = true
	ret["supersededBy"] = "rejectedResultContinuation"
	ret["useAsCurrentProgress"] = false
	delete(ret, stepInputKeyProcessEvidenceTimeline)
	delete(ret, stepInputKeyCurrentProgressDigest)
	return ret
}

func inheritedContinuationProcessEvidence(executionContext map[string]any) ([]map[string]any, map[string]any) {
	if timeline, digest := inheritedRejectedResultProcessEvidence(executionContext); len(timeline) > 0 {
		return timeline, digest
	}
	return inheritedReplanProcessEvidence(executionContext)
}

func inheritedRejectedResultProcessEvidence(executionContext map[string]any) ([]map[string]any, map[string]any) {
	continuation := mapFromValue(executionContext["rejectedResultContinuation"])
	if len(continuation) == 0 {
		return nil, nil
	}
	timeline := processEvidenceTimelineFromRejectedResultContinuation(continuation)
	if len(timeline) == 0 {
		return nil, nil
	}
	digest := currentProgressDigestFromRejectedResultContinuation(continuation, timeline)
	return compactProcessEvidenceTimelineForRuntimeContext(timeline), digest
}

func inheritedReplanProcessEvidence(executionContext map[string]any) ([]map[string]any, map[string]any) {
	replan := mapFromValue(executionContext["replanContinuation"])
	if len(replan) == 0 {
		return nil, nil
	}
	timeline := mapSliceFromValue(replan[stepInputKeyProcessEvidenceTimeline])
	if len(timeline) == 0 {
		timeline = processEvidenceTimelineFromReplanContinuation(replan)
	}
	if len(timeline) == 0 {
		return nil, nil
	}
	digest := mapFromValue(replan[stepInputKeyCurrentProgressDigest])
	if len(digest) == 0 {
		digest = currentProgressDigestFromProcessEvidenceTimeline(timeline)
	}
	return compactProcessEvidenceTimelineForRuntimeContext(timeline), digest
}

func processEvidenceTimelineFromRejectedResultContinuation(continuation map[string]any) []map[string]any {
	if len(continuation) == 0 {
		return nil
	}
	sourceRef := firstNonEmptyString(stringFromMap(continuation, "sourceRef"), stringFromMap(continuation, "resultId"))
	item := map[string]any{
		"order":      1,
		"sourceRef":  firstNonEmptyString(sourceRef, "rejectedResultContinuation"),
		"kind":       "result/rejected",
		"actionKind": "result_assessment",
		"state":      string(common.ResultStateRejected),
	}
	if summary := strings.TrimSpace(stringFromMap(continuation, "summary")); summary != "" {
		item["summary"] = summary
		item["failureReason"] = summary
	}
	if evidence := stringSliceFromValue(continuation["requiredEvidence"]); len(evidence) > 0 {
		item["requiredEvidence"] = limitedStrings(evidence, 8)
	}
	if refs := stringSliceFromValue(continuation["resultEvidenceRefs"]); len(refs) > 0 {
		item["evidenceRefs"] = limitedStrings(refs, 8)
	}
	return []map[string]any{item}
}

func currentProgressDigestFromRejectedResultContinuation(continuation map[string]any, timeline []map[string]any) map[string]any {
	if len(continuation) == 0 {
		return nil
	}
	required := stringSliceFromValue(continuation["requiredEvidence"])
	currentGap := firstNonEmptyString(strings.Join(required, "\n"), stringFromMap(continuation, "summary"), stringFromMap(continuation, "goalOutcome"))
	if strings.TrimSpace(currentGap) == "" {
		return nil
	}
	lastFailure := map[string]any{
		"kind":             "result/rejected",
		"state":            string(common.ResultStateRejected),
		"failureClass":     "rejected_result_continuation",
		"failureReason":    truncateLLMContextString(currentGap),
		"source":           "rejectedResultContinuation",
		"evidenceValidity": "actionable_gap",
	}
	if sourceRef := firstNonEmptyString(stringFromMap(continuation, "sourceRef"), stringFromMap(continuation, "resultId")); sourceRef != "" {
		lastFailure["sourceRef"] = sourceRef
	}
	if len(timeline) > 0 {
		if stepID := strings.TrimSpace(stringFromMap(timeline[0], "stepId")); stepID != "" {
			lastFailure["stepId"] = stepID
		}
	}
	return map[string]any{
		"currentGap":          truncateLLMContextString(currentGap),
		"lastObservedFailure": lastFailure,
		"nextRequiredAction":  "address_rejected_result_required_evidence",
		"source":              "rejectedResultContinuation",
	}
}

func processEvidenceTimelineFromReplanContinuation(replan map[string]any) []map[string]any {
	if len(replan) == 0 {
		return nil
	}
	failureContext := mapFromValue(replan["failureContext"])
	if len(failureContext) == 0 {
		failureContext = mapFromValue(replan["ticketContext"])
	}
	if len(failureContext) == 0 {
		return nil
	}
	failedStepID := firstNonEmptyString(
		stringFromMap(failureContext, "failedStepId"),
		stringFromMap(replan, "ticketStepId"),
	)
	item := map[string]any{
		"order":      1,
		"sourceRef":  "replanContinuation",
		"kind":       firstNonEmptyString(stringFromMap(failureContext, "failedStepKind"), "runtime/replan_continuation"),
		"actionKind": "runtime",
		"state":      string(common.StepStateFailed),
	}
	if failedStepID != "" {
		item["stepId"] = failedStepID
		item["sourceRef"] = "step://" + failedStepID
	}
	for _, key := range []string{
		"stage",
		"failureClass",
		"failureSignature",
		"failureReason",
		"failureTargetScope",
	} {
		if value, ok := failureContext[key]; ok && value != nil {
			item[key] = value
		}
	}
	if _, ok := item["failureReason"]; !ok {
		if reason := firstNonEmptyString(
			stringFromMap(failureContext, "reason"),
			stringFromMap(failureContext, "error"),
			stringFromMap(replan, "ticketReason"),
		); reason != "" {
			item["failureReason"] = reason
		}
	}
	if output := mapFromValue(failureContext["failedStepOutput"]); len(output) > 0 {
		item["output"] = compactFailureStepOutputForRuntimeContext(output)
		if summary := processEvidenceOutputSummary(output); summary != "" {
			item["summary"] = summary
			item["outputSummary"] = summary
		}
	}
	if input := mapFromValue(failureContext["failedStepInput"]); len(input) > 0 {
		if summary := processEvidenceInputSummary(input); summary != "" {
			item["inputSummary"] = summary
		}
	}
	return []map[string]any{item}
}

func compactProcessEvidenceTimelineForRuntimeContext(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	selected := input
	if len(selected) > maxLLMContextSliceItems {
		selected = selected[len(selected)-maxLLMContextSliceItems:]
	}
	ret := make([]map[string]any, 0, len(selected))
	for _, item := range selected {
		if len(item) == 0 {
			continue
		}
		next := map[string]any{}
		for _, key := range []string{
			"order",
			"sourceRef",
			"stepId",
			"kind",
			"actionKind",
			"state",
			"stage",
			"graphNodeId",
			"graphRootStepId",
			"summary",
			"inputSummary",
			"failureClass",
			"failureSignature",
			"failureReason",
			"requiredEvidence",
			"outputRef",
			"payloadRef",
		} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if refs := stringSliceFromValue(item["evidenceRefs"]); len(refs) > 0 {
			next["evidenceRefs"] = refs
		}
		if output := mapFromValue(item["output"]); len(output) > 0 {
			next["output"] = compactFailureStepOutputForRuntimeContext(output)
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	return ret
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
		ret["failedStepOutput"] = compactFailureStepOutputForRuntimeContext(failedOutput)
	}
	if targets := compactRepairTargetSnapshotsForLineage(input); len(targets) > 0 {
		ret["targetSnapshots"] = targets
	}
	return stripRuntimeLLMBinding(ret)
}

func compactFailureStepOutputForRuntimeContext(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"status",
		"stage",
		"error",
		"reason",
		"summary",
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureCategory",
		"failureTargetScope",
		"exitCode",
		"command",
		"timeoutMillis",
		"workingDir",
	} {
		if value, ok := output[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{"stdout", "stderr", "stdoutPreview", "stderrPreview"} {
		if value := strings.TrimSpace(stringFromMap(output, key)); value != "" {
			targetKey := key
			if !strings.HasSuffix(key, "Preview") {
				targetKey = key + "Preview"
			}
			ret[targetKey] = truncateLLMEvidenceText(value)
		}
	}
	if args := stringSliceFromValue(output["args"]); len(args) > 0 {
		ret["args"] = args
	}
	if diagnostics := mapFromValue(output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		ret["lastFailureDiagnostics"] = compactFailureStepOutputForRuntimeContext(diagnostics)
	}
	if patches := mapSliceFromValue(output["patches"]); len(patches) > 0 {
		ret["patches"] = compactPatchMetadataForRuntimeContext(patches)
	}
	if stream := mapFromValue(output["streamDiagnostics"]); len(stream) > 0 {
		ret["streamDiagnostics"] = compactRuntimeStreamDiagnostics(stream)
	}
	if _, ok := output["rawContentPreview"]; ok {
		ret["rawContentPreviewOmitted"] = true
	}
	if _, ok := output["streamEvents"]; ok {
		ret["streamEventsOmitted"] = true
	}
	return stripRuntimeLLMBinding(ret)
}

func compactPatchMetadataForRuntimeContext(patches []map[string]any) []map[string]any {
	if len(patches) == 0 {
		return nil
	}
	limit := min(len(patches), maxLLMContextSliceItems)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		patch := patches[idx]
		next := map[string]any{}
		for _, key := range []string{
			"targetRef",
			"patchType",
			"summary",
			"baseHash",
			"state",
			"approvalId",
			"addedLines",
			"deletedLines",
			"diffHash",
		} {
			if value, ok := patch[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	return ret
}

func compactRuntimeStreamDiagnostics(stream map[string]any) map[string]any {
	if len(stream) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"streaming",
		"streamIncomplete",
		"doneReceived",
		"receivedBytes",
		"receivedDeltaCount",
		"receivedEventCount",
	} {
		if value, ok := stream[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	return ret
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

func truncateLLMEvidenceText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLLMContextEvidenceTextBytes {
		return value
	}
	return value[:maxLLMContextEvidenceTextBytes] + "..."
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
	tailStart := len(text) - maxLLMContextEvidenceTextBytes
	if tailStart < 0 {
		tailStart = 0
	}
	return map[string]any{
		"textPreview": text[:maxLLMContextEvidenceTextBytes] + "...",
		"headPreview": text[:maxLLMContextEvidenceTextBytes],
		"tailPreview": text[tailStart:],
		"bytes":       len(text),
		"truncated":   true,
		"refOnly":     false,
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
