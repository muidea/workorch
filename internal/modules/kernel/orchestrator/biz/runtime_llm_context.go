package orchestrator

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	maxLLMContextStringBytes       = 4096
	maxLLMContextEvidenceTextBytes = 1200
	maxLLMContextSliceItems        = 20
	maxLLMContextMapEntries        = 32
)

type llmContextSubsystem struct{}

func newLLMContextSubsystem() *llmContextSubsystem {
	return &llmContextSubsystem{}
}

func (c *llmContextSubsystem) compactStepInput(input map[string]any) map[string]any {
	return compactStepInputForLLMContext(input)
}

func (c *llmContextSubsystem) compactStepOutput(output map[string]any) map[string]any {
	return compactStepOutputForLLMContext(output)
}

func (c *llmContextSubsystem) compactExecutionGraph(graph runtimemodel.BlockExecutionGraph) runtimemodel.BlockExecutionGraph {
	return compactExecutionGraphForLLMContext(graph)
}

func (c *llmContextSubsystem) compactMap(input map[string]any, depth int) map[string]any {
	return compactLLMContextMap(input, depth)
}

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
	if continuation := mapFromValue(input["artifactContinuationContext"]); len(continuation) > 0 {
		ret["artifactContinuationContext"] = compactArtifactContinuationContextForLLMContext(continuation)
	}
	if contract := mapFromValue(input["artifactOutputContract"]); len(contract) > 0 {
		ret["artifactOutputContract"] = compactArtifactOutputContractForLLMContext(contract)
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
			"snapshotRef",
			"payloadRef",
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
		if _, ok := next["snapshotRef"]; !ok {
			if ref := compactTargetSnapshotRef(item); ref != "" {
				next["snapshotRef"] = ref
			}
		}
		content := strings.TrimSpace(stringFromMap(item, "content"))
		preview := strings.TrimSpace(stringFromMap(item, "contentPreview"))
		if content != "" || preview != "" {
			next["contentOmitted"] = true
			next["contentOmittedReason"] = "target snapshot body is available only through snapshotRef or an explicit targeted read"
			if content != "" {
				next["contentBytes"] = len([]byte(content))
			} else {
				next["contentPreviewBytes"] = len([]byte(preview))
			}
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	return ret
}

func compactTargetSnapshotRef(item map[string]any) string {
	if len(item) == 0 {
		return ""
	}
	for _, key := range []string{"snapshotRef", "payloadRef", "sourceRef"} {
		if ref := strings.TrimSpace(stringFromMap(item, key)); ref != "" {
			return ref
		}
	}
	targetRef := strings.TrimSpace(stringFromMap(item, "targetRef"))
	contentHash := strings.TrimSpace(stringFromMap(item, "contentHash"))
	if targetRef == "" || contentHash == "" {
		return ""
	}
	return "snapshot://target/" + compactSnapshotRefComponent(targetRef) + "#" + compactSnapshotRefComponent(contentHash)
}

func compactSnapshotRefComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		" ", "%20",
		"#", "%23",
		"?", "%3F",
		"\n", "",
		"\r", "",
		"\t", "%09",
	)
	return replacer.Replace(value)
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
	if replanContinuationCarriesActionableFailure(executionContext) {
		return inheritedReplanProcessEvidence(executionContext)
	}
	if timeline, digest := inheritedRejectedResultProcessEvidence(executionContext); len(timeline) > 0 {
		return timeline, digest
	}
	return inheritedReplanProcessEvidence(executionContext)
}

func replanContinuationCarriesActionableFailure(executionContext map[string]any) bool {
	if len(executionContext) == 0 {
		return false
	}
	return len(primaryActionableFailureFromReplanContinuation(mapFromValue(executionContext["replanContinuation"]))) > 0
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
	if primary := primaryActionableFailureFromReplanContinuation(replan); len(primary) > 0 {
		if primaryDigest := currentProgressDigestFromPrimaryActionableFailure(primary); len(primaryDigest) > 0 {
			digest = primaryDigest
		}
	}
	return compactProcessEvidenceTimelineForRuntimeContext(timeline), digest
}

func processEvidenceTimelineFromRejectedResultContinuation(continuation map[string]any) []map[string]any {
	if len(continuation) == 0 {
		return nil
	}
	sourceRef := firstNonEmptyString(stringFromMap(continuation, "sourceRef"), stringFromMap(continuation, "resultId"))
	currentGap := rejectedResultContinuationCurrentGap(continuation)
	item := map[string]any{
		"order":      1,
		"sourceRef":  firstNonEmptyString(sourceRef, "rejectedResultContinuation"),
		"kind":       "result/rejected",
		"actionKind": "result_assessment",
		"state":      string(common.ResultStateRejected),
	}
	if summary := strings.TrimSpace(stringFromMap(continuation, "summary")); summary != "" {
		item["summary"] = summary
	}
	if currentGap != "" {
		item["failureReason"] = currentGap
	}
	if targetRef := rejectedResultContinuationTargetRef(continuation); targetRef != "" {
		item["targetRef"] = targetRef
	}
	if targetScope := rejectedResultContinuationTargetScope(continuation); targetScope != "" {
		item["targetScope"] = targetScope
		item["failureTargetScope"] = targetScope
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
	currentGap := rejectedResultContinuationCurrentGap(continuation)
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
	if targetRef := rejectedResultContinuationTargetRef(continuation); targetRef != "" {
		lastFailure["targetRef"] = targetRef
	}
	if targetScope := rejectedResultContinuationTargetScope(continuation); targetScope != "" {
		lastFailure["targetScope"] = targetScope
		lastFailure["failureTargetScope"] = targetScope
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

func rejectedResultContinuationCurrentGap(continuation map[string]any) string {
	if len(continuation) == 0 {
		return ""
	}
	diagnostics := mapFromValue(continuation["diagnostics"])
	runtimeDiagnostics := mapFromValue(diagnostics["runtimeDiagnostics"])
	if len(runtimeDiagnostics) == 0 {
		runtimeDiagnostics = mapFromValue(continuation["runtimeDiagnostics"])
	}
	for _, value := range []string{
		stringFromMap(diagnostics, "gateFailureReason"),
		stringFromMap(diagnostics, "rejectionReason"),
		stringFromMap(continuation, "gateFailureReason"),
		stringFromMap(continuation, "rejectionReason"),
		stringFromMap(runtimeDiagnostics, "reason"),
		stringFromMap(runtimeDiagnostics, "failureReason"),
		stringFromMap(runtimeDiagnostics, "error"),
	} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	if missing := rejectedContinuationMissingDoDLines(mapSliceFromValue(runtimeDiagnostics["missingDoDItems"])); len(missing) > 0 {
		return strings.Join(missing, "\n")
	}
	if missing := rejectedContinuationMissingDoDLines(mapSliceFromValue(continuation["unsatisfiedDoDItems"])); len(missing) > 0 {
		return strings.Join(missing, "\n")
	}
	if gaps := rejectedContinuationCoverageGapLines(mapFromValue(continuation["coverageGaps"])); len(gaps) > 0 {
		return strings.Join(gaps, "\n")
	}
	requiredJoined := strings.TrimSpace(strings.Join(stringSliceFromValue(continuation["requiredEvidence"]), "\n"))
	return firstNonEmptyString(
		requiredJoined,
		stringFromMap(continuation, "summary"),
		stringFromMap(continuation, "goalOutcome"),
	)
}

func rejectedResultContinuationTargetRef(continuation map[string]any) string {
	if len(continuation) == 0 {
		return ""
	}
	diagnostics := mapFromValue(continuation["diagnostics"])
	runtimeDiagnostics := mapFromValue(diagnostics["runtimeDiagnostics"])
	if len(runtimeDiagnostics) == 0 {
		runtimeDiagnostics = mapFromValue(continuation["runtimeDiagnostics"])
	}
	return firstNonEmptyString(
		stringFromMap(continuation, "targetRef"),
		stringFromMap(diagnostics, "targetRef"),
		stringFromMap(runtimeDiagnostics, "targetRef"),
	)
}

func rejectedResultContinuationTargetScope(continuation map[string]any) string {
	if len(continuation) == 0 {
		return ""
	}
	diagnostics := mapFromValue(continuation["diagnostics"])
	runtimeDiagnostics := mapFromValue(diagnostics["runtimeDiagnostics"])
	if len(runtimeDiagnostics) == 0 {
		runtimeDiagnostics = mapFromValue(continuation["runtimeDiagnostics"])
	}
	return firstNonEmptyString(
		stringFromMap(continuation, "failureTargetScope"),
		stringFromMap(continuation, "targetScope"),
		stringFromMap(diagnostics, "failureTargetScope"),
		stringFromMap(diagnostics, "targetScope"),
		stringFromMap(runtimeDiagnostics, "failureTargetScope"),
		stringFromMap(runtimeDiagnostics, "targetScope"),
	)
}

func rejectedContinuationMissingDoDLines(items []map[string]any) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		text := firstNonEmptyString(
			stringFromMap(item, "item"),
			stringFromMap(item, "requirement"),
			stringFromMap(item, "goal"),
			stringFromMap(item, "goalItem"),
			stringFromMap(item, "id"),
		)
		reason := firstNonEmptyString(
			stringFromMap(item, "reason"),
			stringFromMap(item, "failureReason"),
			stringFromMap(item, "requiredEvidenceHint"),
		)
		line := strings.TrimSpace(text)
		if reason != "" {
			line = strings.TrimSpace(line + ": " + reason)
		}
		if line != "" {
			ret = append(ret, line)
		}
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func rejectedContinuationCoverageGapLines(gaps map[string]any) []string {
	if len(gaps) == 0 {
		return nil
	}
	ret := []string{}
	for _, key := range []string{"missingGoalCoverage", "missingAcceptanceDimensions", "missingDeliverableCoverage"} {
		for _, line := range rejectedContinuationMissingDoDLines(mapSliceFromValue(gaps[key])) {
			ret = append(ret, key+": "+line)
			if len(ret) >= 8 {
				return ret
			}
		}
	}
	return ret
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
		"targetRef",
		"targetScope",
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
			stepInputKeyRepairOfStepID,
			"targetRef",
			"targetScope",
			"failureTargetScope",
			"summary",
			"summaryKind",
			"historicalOnly",
			"itemCount",
			"firstSourceRef",
			"lastSourceRef",
			"inputSummary",
			"failureClass",
			"failureSignature",
			"failureReason",
			"requiredEvidence",
			"artifactContinuation",
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
		if refs := stringSliceFromValue(item["sourceRefs"]); len(refs) > 0 {
			next["sourceRefs"] = limitedStrings(refs, maxLLMContextSliceItems)
		}
		for _, key := range []string{"stateCounts", "kindCounts", "stageCounts", "latestFailure"} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
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
		"diagnostics",
		"runtimeDiagnostics",
		"gateFailureReason",
		"rejectionReason",
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
		"targetRef",
		"targetScope",
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
		"targetRef",
		"targetScope",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"failureReason",
		"repairObjective",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"coverageRepairRequired",
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
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		ret["repairActionPackage"] = compactLLMContextValue(pkg, 2)
	}
	for _, key := range []string{
		"coverageTargetSet",
		"missingDoDItems",
		"missingGoalCoverage",
		"missingDeliverableCoverage",
	} {
		if values := mapSliceFromValue(input[key]); len(values) > 0 {
			ret[key] = cloneMapSlice(values)
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
		"targetRef",
		"targetScope",
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
	if diagnostic := mapFromValue(output["validationFailureDiagnostic"]); len(diagnostic) > 0 {
		ret["validationFailureDiagnostic"] = compactLLMContextMap(diagnostic, 1)
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
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		delete(ret, stepOutputKeyExecutionGraph)
		ret["executionGraphSummary"] = rejectedGraphSummary(graph)
	}
	if chunk := mapFromValue(output["artifactChunk"]); len(chunk) > 0 {
		ret["artifactChunk"] = compactArtifactChunkForLLMContext(chunk)
	}
	if plan := mapFromValue(output["artifactPlan"]); len(plan) > 0 {
		ret["artifactPlan"] = compactArtifactPlanForLLMContext(plan)
	}
	if repair := mapFromValue(output["artifactChunkRepair"]); len(repair) > 0 {
		ret["artifactChunkRepair"] = compactArtifactChunkRepairForLLMContext(repair)
	}
	if decision := mapFromValue(output[stepOutputKeyLLMDecision]); len(decision) > 0 {
		compactDecision := compactLLMContextMap(decision, 0)
		if graph, ok := runtimeGraphSpecFromValue(decision[stepOutputKeyExecutionGraph]); ok {
			delete(compactDecision, stepOutputKeyExecutionGraph)
			compactDecision["executionGraphSummary"] = rejectedGraphSummary(graph)
		}
		if chunk := mapFromValue(decision["artifactChunk"]); len(chunk) > 0 {
			compactDecision["artifactChunk"] = compactArtifactChunkForLLMContext(chunk)
		}
		if plan := mapFromValue(decision["artifactPlan"]); len(plan) > 0 {
			compactDecision["artifactPlan"] = compactArtifactPlanForLLMContext(plan)
		}
		if repair := mapFromValue(decision["artifactChunkRepair"]); len(repair) > 0 {
			compactDecision["artifactChunkRepair"] = compactArtifactChunkRepairForLLMContext(repair)
		}
		ret[stepOutputKeyLLMDecision] = compactDecision
	}
	return stripRuntimeLLMBinding(ret)
}

func compactArtifactContinuationContextForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"id",
		"runId",
		"rootStepId",
		"state",
		"artifactId",
		"targetRef",
		"operation",
		"expectedSequence",
		"finalReceived",
		"bytesWritten",
		"rollingHash",
		"lastChunkHash",
		"previousChunkTailBytes",
		"previousChunkTailHash",
		"coverageTargets",
		"receivedSequences",
		"receivedChunkHashes",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	return ret
}

func compactArtifactOutputContractForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"mode",
		"phase",
		"mustReturnArtifactPlan",
		"mustReturnArtifactChunk",
		"mustReturnArtifactChunkRepair",
		"maxArtifactChunkBytes",
		"previousChunkTailBytes",
		"stateChangingEvidenceStage",
		"schema",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	return ret
}

func compactArtifactPlanForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	if items := mapSliceFromValue(input["artifacts"]); len(items) > 0 {
		limit := min(len(items), maxLLMContextSliceItems)
		artifacts := make([]map[string]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			item := items[idx]
			next := map[string]any{}
			for _, key := range []string{
				"artifactId",
				"targetRef",
				"operation",
				"coverageTargets",
				"estimatedChunks",
				"estimatedBytes",
			} {
				if value, ok := item[key]; ok && value != nil {
					next[key] = compactLLMContextValue(value, 1)
				}
			}
			if len(next) > 0 {
				artifacts = append(artifacts, next)
			}
		}
		if len(artifacts) > 0 {
			ret["artifacts"] = artifacts
		}
	}
	return ret
}

func compactArtifactChunkForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"artifactId",
		"targetRef",
		"sequence",
		"isFinal",
		"chunkHash",
		"startsAfter",
		"endsWith",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if content := strings.TrimSpace(stringFromMap(input, "content")); content != "" {
		ret["contentOmitted"] = true
		ret["contentBytes"] = len([]byte(content))
	}
	return ret
}

func compactArtifactChunkRepairForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"artifactId",
		"targetRef",
		"sequence",
		"failureReason",
		"nextAction",
		"constraints",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	return ret
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
	normalizeCompactedTargetSnapshotForLLMContext(ret, input)
	return ret
}

func normalizeCompactedTargetSnapshotForLLMContext(ret map[string]any, original map[string]any) {
	if len(ret) == 0 || len(original) == 0 || !looksLikeRuntimeTargetSnapshot(original) {
		return
	}
	originalContent := strings.TrimSpace(stringFromMap(original, "content"))
	if originalContent == "" {
		return
	}
	visibleContent, hasVisibleContent := ret["content"].(string)
	if hasVisibleContent && visibleContent == originalContent && !containsRuntimeRedactionMarker(visibleContent) {
		return
	}
	if hasVisibleContent && strings.TrimSpace(visibleContent) != "" {
		ret["contentPreview"] = visibleContent
	}
	delete(ret, "content")
	ret["contentOmitted"] = true
	ret["contentComplete"] = false
	ret["safeForFileWrite"] = false
	ret["truncated"] = true
	ret["contentBytes"] = len([]byte(originalContent))
	if hasVisibleContent {
		ret["visibleContentBytes"] = len([]byte(visibleContent))
	}
	if containsRuntimeRedactionMarker(originalContent) {
		ret["redacted"] = true
		ret["contentOmittedReason"] = "target snapshot content contains runtime redaction markers and is not reusable as exact patch or file_write content"
	} else {
		ret["contentOmittedReason"] = "target snapshot content was compacted for LLM context and is not a complete exact patch or file_write baseline"
	}
}

func looksLikeRuntimeTargetSnapshot(value map[string]any) bool {
	if len(value) == 0 || strings.TrimSpace(stringFromMap(value, "targetRef")) == "" {
		return false
	}
	for _, key := range []string{"patchType", "oldText", "newText", "diff"} {
		if _, ok := value[key]; ok {
			return false
		}
	}
	for _, key := range []string{
		"content",
		"contentHash",
		"contentComplete",
		"safeForFileWrite",
		"truncated",
		"redacted",
		"isBinary",
		"exists",
	} {
		if _, ok := value[key]; ok {
			return true
		}
	}
	return false
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
