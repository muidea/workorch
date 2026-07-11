package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

const (
	defaultProviderRequestBudgetBytes = 96 * 1024
	defaultEstimatedTokensPerByte     = 0.25
	providerBudgetStringBytes         = 2048
	providerBudgetAggressiveBytes     = 768
	providerBudgetSliceItems          = 12
	providerBudgetAggressiveItems     = 6
	providerBudgetMapEntries          = 24
	providerBudgetAggressiveEntries   = 12
	providerContextTracePolicy        = "summary_refs_only"
)

const (
	providerContextGatePass                = "pass"
	providerContextGateReadRequired        = "read_required"
	providerContextGateDegradeToAssessment = "degrade_to_assessment"
	providerContextGateBlockerRequired     = "blocker_required"
	providerContextGateFailFast            = "fail_fast"
	providerContextGateReplanRequired      = "replan_required"
)

var providerP0InputKeys = []string{
	"workspaceId", "goalId", "doDId", "previousStepId", "currentSoulStage",
	"runtimeRepairFollowUp", "runtimeRepairAttempt", "providerRetryFollowUp", "providerRetryAttempt",
	"assessmentOnly", "assessmentOnlyReason", "assessmentOnlyContractCorrection",
	"graphExpansionLimitFollowUp", "graphFollowUpExpansionCount", "graphFollowUpExpansionLimit",
	"providerRetryAssessmentOnly",
	"runtimeStage", "runtimeStageMode", "currentRuntimeStage", "currentRuntimeStageMode",
	"completionFollowUp", "graphFollowUp", "failureReason", "failureClass", "failureSignature",
	"repairObjective", "stateChangingOutputRequired", "stateChangingRecoveryMode",
	"providerTimeoutStateChangingRetry", "forbiddenRecoveryOutputs", "requiredRecoveryOutputs",
	"decisionResolution", "approvalResolution",
	"contractViolationRecovery",
	"currentActionContract",
	"coverageWorkItemContext",
	"evidenceLedger",
	"stagePacket",
}

var providerP1InputKeys = []string{
	"failedStepInput", "failedStepOutput", "skillSelection", "memoryRetrieval",
}

var providerP2InputKeys = []string{
	"failedExecutionGraph", "graphEvidence", "runEvidenceSummary", "processEvidenceTimeline", "currentProgressDigest", "workspaceFileManifest", "workspaceManifest", "previousResults",
}

func enforceProviderRequestBudget(request llmmodels.ProviderRequest) llmmodels.ProviderRequest {
	return enforceProviderRequestBudgetLimit(request, 0)
}

func enforceProviderRequestBudgetLimit(request llmmodels.ProviderRequest, maxBytes int) llmmodels.ProviderRequest {
	baseProfile := normalizedProviderContextProfile(request.Config.ContextProfile)
	profile, stageBudgetRatio, stageBudgetSource := stageAwareProviderContextProfile(baseProfile, request.Input)
	if maxBytes <= 0 {
		maxBytes = profile.BytesHardLimit
	} else if profile.BytesHardLimit > 0 && profile.BytesHardLimit < maxBytes {
		maxBytes = profile.BytesHardLimit
	}
	if maxBytes <= 0 {
		maxBytes = defaultProviderRequestBudgetBytes
	}
	maxTokens := profile.MaxInputTokens
	rules := []string{"preserve identifiers, schema, config, runtime capabilities, goal, DoD, and governance decisions"}
	sourceRefs := []string{}
	includedSections := []string{"p0"}
	normalizedSections := []string{}
	compressedSections := []string{}
	omittedSections := []string{}
	protectedFields := providerProtectedContextFields(request.Input)
	request.ContextBudget = nil
	shrinkLevel := 0
	adaptiveLevel := adaptiveShrinkLevelFromInput(request.Input)
	protected := captureProtectedGoalDoD(request.Input)
	protectedRepairTargets := captureProtectedProviderRepairTargets(request.Input)
	protectedRuntimeContract := captureProtectedProviderRuntimeContract(request.Input, adaptiveLevel)
	protectedCoverageWorkItemContext := captureProtectedProviderCoverageWorkItemContext(request.Input, adaptiveLevel)
	protectedStagePacket := captureProtectedProviderStagePacket(request.Input)
	restoreProtected := func(input map[string]any) map[string]any {
		input = restoreProtectedGoalDoD(input, protected)
		input = restoreProtectedProviderRepairTargets(input, protectedRepairTargets)
		input = restoreProtectedProviderRuntimeContract(input, protectedRuntimeContract)
		input = restoreProtectedProviderCoverageWorkItemContext(input, protectedCoverageWorkItemContext)
		input = restoreProtectedProviderStagePacket(input, protectedStagePacket)
		return normalizeAdaptiveProviderRecoveryTail(input, adaptiveLevel)
	}
	if providerInputHasCurrentActionContract(request.Input) {
		sourceRefs = append(sourceRefs, providerInputSourceRefs(request.Input)...)
		request.Input = narrowProviderInputToCurrentAction(request.Input)
		request.Input = restoreProtected(request.Input)
		rules = append(rules, "narrow provider input to current action contract")
		compressedSections = append(compressedSections, "current_action")
		shrinkLevel = max(shrinkLevel, 1)
	}
	if adaptiveLevel > 0 {
		sourceRefs = append(sourceRefs, providerInputSourceRefs(request.Input)...)
		request.Input = applyAdaptiveShrinkLevel(request.Input, adaptiveLevel)
		request.Input = restoreProtected(request.Input)
		rules = append(rules, adaptiveShrinkRuleFromInput(request.Input))
		compressedSections = append(compressedSections, "adaptive")
		shrinkLevel = adaptiveLevel
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		sourceRefs = append(sourceRefs, providerInputSourceRefs(request.Input)...)
		request.Input = budgetProviderInput(request.Input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 4)
		request.Input = restoreProtected(request.Input)
		rules = append(rules, "compact provider input")
		compressedSections = append(compressedSections, "p2")
		shrinkLevel = max(shrinkLevel, 1)
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		sourceRefs = append(sourceRefs, providerInputSourceRefs(request.Input)...)
		request.Input = compactProviderExecutionContext(request.Input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries)
		request.Input = restoreProtected(request.Input)
		rules = append(rules, "summarize execution context transport fields")
		compressedSections = append(compressedSections, "p1")
		shrinkLevel = max(shrinkLevel, 2)
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		sourceRefs = append(sourceRefs, providerInputSourceRefs(request.Input)...)
		request.Input = prioritizedProviderInput(request.Input, true)
		request.Input = restoreProtected(request.Input)
		rules = append(rules, "apply priority compaction (P0/P1/P2)")
		compressedSections = append(compressedSections, "p1", "p2")
		shrinkLevel = max(shrinkLevel, 3)
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		sourceRefs = append(sourceRefs, providerInputSourceRefs(request.Input)...)
		request.Input = minimalProviderInput(request.Input)
		request.Input = restoreProtected(request.Input)
		rules = append(rules, "fallback to stage-aware minimal provider input (P0/P1)")
		compressedSections = append(compressedSections, "p2", "older_p1")
		shrinkLevel = max(shrinkLevel, 4)
	}
	goalDoDCompressed := !isProtectedGoalDoDEqual(request.Input, protected)
	actual := providerRequestJSONBytes(request)
	estimatedInputTokens := estimateInputTokens(actual, profile.EstimatedTokensPerByte)
	estimatedOutputTokens := min(profile.ReservedOutputTokens, max(0, profile.ContextWindowTokens-estimatedInputTokens))
	actualTokens := estimatedInputTokens + estimatedOutputTokens
	overBudget := actual > maxBytes || (maxTokens > 0 && estimatedInputTokens > maxTokens)
	p0Overflow := false
	if overBudget && hasProtectedGoalDoD(protected) {
		probe := request
		probe.Input = withoutProtectedGoalDoD(request.Input)
		p0Overflow = !exceedsProviderBudget(probe, maxBytes, maxTokens, profile.EstimatedTokensPerByte)
	}
	if p0Overflow {
		rules = append(rules, "p0 overflow: goal/dod contract preserved without compression")
	}
	integrityWarnings := providerContextIntegrityWarnings(request.Input, protected, protectedRepairTargets, protectedRuntimeContract, protectedCoverageWorkItemContext, protectedStagePacket, profile, overBudget)
	contextGateAction, contextGateReason := providerContextGateDecision(goalDoDCompressed, overBudget, p0Overflow, integrityWarnings)
	truncated := len(rules) > 1 || overBudget
	request.ContextBudget = &llmmodels.ProviderContextBudget{
		MaxBytes:              maxBytes,
		ActualBytes:           actual,
		MaxTokens:             maxTokens,
		EstimatedInputTokens:  estimatedInputTokens,
		EstimatedOutputTokens: estimatedOutputTokens,
		ActualTokens:          actualTokens,
		ShrinkLevel:           shrinkLevel,
		Truncated:             truncated,
		Rules:                 rules,
		SourceRefs:            uniqueStrings(sourceRefs),
		IncludedSections:      uniqueStrings(includedSections),
		NormalizedSections:    uniqueStrings(normalizedSections),
		CompressedSections:    uniqueStrings(compressedSections),
		OmittedSections:       uniqueStrings(omittedSections),
		GoalDoDCompressed:     goalDoDCompressed,
		OverBudget:            overBudget,
		P0Overflow:            p0Overflow,
		ProfileMode:           profile.ProfileMode,
		CapabilitySource:      profile.CapabilitySource,
		ContextUtilization:    profile.ContextUtilizationRatio,
		MaxRequestBytes:       profile.MaxRequestBytes,
		StageBudgetRatio:      stageBudgetRatio,
		StageBudgetSource:     stageBudgetSource,
		TracePolicy:           providerContextTracePolicy,
		PromptCaptured:        false,
		Estimator:             "bytes_ratio",
		ProtectedFields:       uniqueStrings(protectedFields),
		IntegrityWarnings:     uniqueStrings(integrityWarnings),
		ContextGateAction:     contextGateAction,
		ContextGateReason:     contextGateReason,
		Profile:               profile,
	}
	return request
}

func stageAwareProviderContextProfile(profile llmmodels.ProviderContextProfile, input map[string]any) (llmmodels.ProviderContextProfile, float64, string) {
	profile = normalizedProviderContextProfile(profile)
	ratio, source := providerContextStageBudgetRatio(input)
	if ratio <= 0 || ratio >= 1 {
		return profile, 1, source
	}
	profile.MaxInputTokens = max(1, int(float64(profile.MaxInputTokens)*ratio))
	profile.BytesHardLimit = max(1, int(float64(profile.BytesHardLimit)*ratio))
	return profile, ratio, source
}

func providerContextStageBudgetRatio(input map[string]any) (float64, string) {
	if boolValue(input["runtimeRepairFollowUp"]) {
		return 1, "runtime_repair"
	}
	if boolValue(input["providerRetryFollowUp"]) {
		switch strings.TrimSpace(stringValue(input["failureClass"])) {
		case "context_too_large", "context_integrity_failed":
			return 1, "provider_retry_context_contract"
		}
		return 0.5, "provider_retry"
	}
	stage := strings.TrimSpace(strings.ToLower(firstNonEmptyProviderContextString(
		stringValue(input["runtimeStage"]),
		stringValue(input["currentSoulStage"]),
	)))
	switch stage {
	case "planning":
		return 1, "stage_planning"
	case "execution":
		return 0.8, "stage_execution"
	case "validation", "verification":
		return 0.55, "stage_validation"
	case "artifact", "result", "final":
		return 0.5, "stage_artifact"
	case "decision", "approval":
		return 0.6, "stage_governance"
	default:
		return 1, "default"
	}
}

func firstNonEmptyProviderContextString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func exceedsProviderBudget(request llmmodels.ProviderRequest, maxBytes, maxTokens int, tokensPerByte float64) bool {
	actualBytes := providerRequestJSONBytes(request)
	if actualBytes > maxBytes {
		return true
	}
	if maxTokens <= 0 {
		return false
	}
	return estimateInputTokens(actualBytes, tokensPerByte) > maxTokens
}

func estimateInputTokens(actualBytes int, tokensPerByte float64) int {
	if actualBytes <= 0 {
		return 0
	}
	if tokensPerByte <= 0 {
		tokensPerByte = defaultEstimatedTokensPerByte
	}
	ret := int(float64(actualBytes) * tokensPerByte)
	if float64(ret) < float64(actualBytes)*tokensPerByte {
		ret++
	}
	return max(ret, 1)
}

func adaptiveShrinkLevelFromInput(input map[string]any) int {
	if boolValue(input["providerRetryFollowUp"]) &&
		boolValue(input["stateChangingOutputRequired"]) &&
		strings.TrimSpace(stringValue(input["stateChangingRecoveryMode"])) == "direct_state_change" {
		return 4
	}
	if boolValue(input["providerRetryFollowUp"]) {
		if !providerRetryFailureClassNeedsAdaptiveShrink(strings.TrimSpace(stringValue(input["failureClass"]))) {
			return 0
		}
		attempt := intValue(input["providerRetryAttempt"])
		switch {
		case attempt >= 2:
			return 4
		case attempt == 1:
			return 3
		default:
			return 0
		}
	}
	if !boolValue(input["runtimeRepairFollowUp"]) &&
		!boolValue(input["contractViolationRetry"]) &&
		len(recordFromAny(input["contractViolationRecovery"])) == 0 {
		return 0
	}
	attempt := intValue(input["runtimeRepairAttempt"])
	switch {
	case attempt >= 3:
		return 4
	case attempt == 2:
		return 3
	case attempt == 1:
		return 2
	default:
		return 0
	}
}

func adaptiveShrinkRuleFromInput(input map[string]any) string {
	if boolValue(input["providerRetryFollowUp"]) {
		return "adaptive shrink based on provider retry"
	}
	return "adaptive shrink based on runtime recovery lineage"
}

func providerRetryFailureClassNeedsAdaptiveShrink(failureClass string) bool {
	switch strings.TrimSpace(failureClass) {
	case "provider_timeout", "provider_stream_incomplete":
		return true
	default:
		return false
	}
}

func applyAdaptiveShrinkLevel(input map[string]any, level int) map[string]any {
	var out map[string]any
	switch {
	case level >= 4:
		out = minimalProviderInput(input)
	case level == 3:
		out = prioritizedProviderInput(input, true)
	case level == 2:
		out = compactProviderExecutionContext(input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries)
	case level == 1:
		out = budgetProviderInput(input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 4)
	default:
		out = input
	}
	return restoreProviderCoverageWorkItemContext(out)
}

func providerRequestJSONBytes(request llmmodels.ProviderRequest) int {
	payload, err := json.Marshal(request)
	if err != nil {
		return 0
	}
	return len(payload)
}

func budgetProviderInput(input map[string]any, stringLimit, itemLimit, entryLimit, maxDepth int) map[string]any {
	if len(input) == 0 {
		return nil
	}
	value, _ := budgetProviderValue(input, 0, stringLimit, itemLimit, entryLimit, maxDepth).(map[string]any)
	return value
}

func budgetProviderValue(value any, depth int, stringLimit, itemLimit, entryLimit, maxDepth int) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return truncateProviderBudgetString(typed, stringLimit)
	case map[string]any:
		if depth >= maxDepth {
			return providerBudgetLeafSummary(typed, stringLimit, entryLimit)
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, min(len(keys), entryLimit))
		for _, key := range keys {
			if len(out) >= entryLimit {
				break
			}
			out[key] = budgetProviderValue(typed[key], depth+1, stringLimit, itemLimit, entryLimit, maxDepth)
		}
		if len(keys) > entryLimit {
			out["_truncatedEntryCount"] = len(keys) - entryLimit
		}
		return out
	case []map[string]any:
		limit := min(len(typed), itemLimit)
		out := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			out = append(out, budgetProviderValue(typed[idx], depth+1, stringLimit, itemLimit, entryLimit, maxDepth))
		}
		return out
	case []any:
		limit := min(len(typed), itemLimit)
		out := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			out = append(out, budgetProviderValue(typed[idx], depth+1, stringLimit, itemLimit, entryLimit, maxDepth))
		}
		return out
	case []string:
		limit := min(len(typed), itemLimit)
		out := make([]string, 0, limit)
		for idx := 0; idx < limit; idx++ {
			out = append(out, truncateProviderBudgetString(typed[idx], stringLimit))
		}
		return out
	default:
		return typed
	}
}

func providerBudgetLeafSummary(input map[string]any, stringLimit, entryLimit int) map[string]any {
	if len(input) == 0 {
		return nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]any, min(len(keys), entryLimit))
	for _, key := range keys {
		if len(out) >= entryLimit {
			break
		}
		switch typed := input[key].(type) {
		case string:
			out[key] = truncateProviderBudgetString(typed, stringLimit)
		case []string:
			out[key] = len(typed)
		case []any:
			out[key] = len(typed)
		case []map[string]any:
			out[key] = len(typed)
		case map[string]any:
			out[key] = len(typed)
		default:
			out[key] = typed
		}
	}
	return out
}

func compactProviderExecutionContext(input map[string]any, stringLimit, itemLimit, entryLimit int) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := budgetProviderInput(input, stringLimit, itemLimit, entryLimit, 4)
	out = restoreProviderCoverageWorkItemContext(out)
	executionContext := recordFromAny(out["executionContext"])
	if len(executionContext) > 0 {
		compactSoulRouting(executionContext)
		compactNamedMapSlice(executionContext, "governanceHistory", itemLimit)
		if context := recordFromAny(executionContext["coverageWorkItemContext"]); len(context) > 0 {
			executionContext["coverageWorkItemContext"] = compactProviderCoverageWorkItemContext(context)
		}
		out["executionContext"] = executionContext
	}
	compactSoulRouting(out)
	return out
}

func restoreProviderCoverageWorkItemContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	if context := recordFromAny(input["coverageWorkItemContext"]); len(context) > 0 {
		input["coverageWorkItemContext"] = compactProviderCoverageWorkItemContext(context)
	}
	if executionContext := recordFromAny(input["executionContext"]); len(executionContext) > 0 {
		if context := recordFromAny(executionContext["coverageWorkItemContext"]); len(context) > 0 {
			executionContext["coverageWorkItemContext"] = compactProviderCoverageWorkItemContext(context)
			input["executionContext"] = executionContext
		}
	}
	if executionContextPackage := recordFromAny(input["executionContextPackage"]); len(executionContextPackage) > 0 {
		if context := recordFromAny(executionContextPackage["coverageWorkItemContext"]); len(context) > 0 {
			executionContextPackage["coverageWorkItemContext"] = compactProviderCoverageWorkItemContext(context)
			input["executionContextPackage"] = executionContextPackage
		}
	}
	if repairActionPackage := recordFromAny(input["repairActionPackage"]); len(repairActionPackage) > 0 {
		if context := recordFromAny(repairActionPackage["coverageWorkItemContext"]); len(context) > 0 {
			repairActionPackage["coverageWorkItemContext"] = compactProviderCoverageWorkItemContext(context)
			input["repairActionPackage"] = repairActionPackage
		}
	}
	return input
}

func compactProviderCoverageWorkItemContext(context map[string]any) map[string]any {
	if len(context) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"schemaVersion", "definitionRevision", "coverageBaselineHash", "fullLedgerRef"} {
		if value, ok := context[key]; ok {
			out[key] = value
		}
	}
	if summary := recordFromAny(context["summary"]); len(summary) > 0 {
		out["summary"] = summary
	}
	if focus := compactProviderCoverageWorkItemFocus(recordFromAny(context["currentFocus"])); len(focus) > 0 {
		out["currentFocus"] = focus
	}
	if focuses := compactProviderCoverageWorkItemFocuses(mapSliceFromAny(context["currentFocusItems"]), 3); len(focuses) > 0 {
		out["currentFocusItems"] = focuses
	}
	if unresolved := compactProviderCoverageWorkItemSummaries(mapSliceFromAny(context["unresolvedItems"]), 8); len(unresolved) > 0 {
		out["unresolvedItems"] = unresolved
	}
	if ledger := compactProviderCoverageWorkItemSummaries(mapSliceFromAny(context["ledgerSummary"]), 12); len(ledger) > 0 {
		out["ledgerSummary"] = ledger
	}
	for _, key := range []string{"omittedCount", "excludedCount", "historicalCount", "omittedStatusDistribution", "omittedSourceScopeDistribution"} {
		if value, ok := context[key]; ok {
			out[key] = value
		}
	}
	return out
}

func compactProviderCoverageWorkItemFocuses(items []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if focus := compactProviderCoverageWorkItemFocus(item); len(focus) > 0 {
			ret = append(ret, focus)
		}
	}
	return ret
}

func compactProviderCoverageWorkItemSummaries(items []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if summary := compactProviderCoverageWorkItemSummary(item); len(summary) > 0 {
			ret = append(ret, summary)
		}
	}
	return ret
}

func compactProviderCoverageWorkItemFocus(item map[string]any) map[string]any {
	focus := compactProviderCoverageWorkItemSummary(item)
	if len(focus) == 0 {
		return nil
	}
	for _, key := range []string{"acceptedEvidenceRefs", "rejectedEvidenceRefs", "requiredEvidenceKinds", "missingEvidenceKinds", "nextAllowedConvergenceStates"} {
		focus[key] = stringSliceFromAny(item[key])
	}
	if value, ok := item["acceptedEvidenceByKind"]; ok {
		focus["acceptedEvidenceByKind"] = value
	}
	if value, ok := item["rejectedEvidenceByKind"]; ok {
		focus["rejectedEvidenceByKind"] = value
	}
	if value, ok := item["coverageBoundary"]; ok {
		focus["coverageBoundary"] = value
	}
	return focus
}

func compactProviderCoverageWorkItemSummary(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"id", "definitionRevision", "source", "sourceScope", "sourceRequirementId", "sourcePath",
		"normalizedSourceKey", "targetText", "status", "currentGap", "omittedFromCurrentFocus", "omittedReason",
	} {
		if value, ok := item[key]; ok {
			out[key] = value
		}
	}
	for _, key := range []string{"acceptedEvidenceRefs", "rejectedEvidenceRefs", "requiredEvidenceKinds", "missingEvidenceKinds"} {
		if _, ok := item[key]; ok {
			out[key] = stringSliceFromAny(item[key])
		}
	}
	return out
}

func compactSoulRouting(input map[string]any) {
	if len(input) == 0 {
		return
	}
	if soul := recordFromAny(input["currentSoul"]); len(soul) > 0 {
		input["currentSoul"] = compactSoulMap(soul)
	}
	if soul := recordFromAny(input["soul"]); len(soul) > 0 {
		input["soul"] = compactSoulMap(soul)
	}
	if binding := recordFromAny(input["currentSoulBinding"]); len(binding) > 0 {
		if soul := recordFromAny(binding["soul"]); len(soul) > 0 {
			binding["soul"] = compactSoulMap(soul)
		}
		input["currentSoulBinding"] = binding
	}
	routing := recordFromAny(input["soulRouting"])
	if len(routing) == 0 {
		return
	}
	bindings := recordFromAny(routing["bindings"])
	for stage, value := range bindings {
		binding := recordFromAny(value)
		if len(binding) == 0 {
			continue
		}
		if soul := recordFromAny(binding["soul"]); len(soul) > 0 {
			binding["soul"] = compactSoulMap(soul)
		}
		bindings[stage] = binding
	}
	routing["bindings"] = bindings
	input["soulRouting"] = routing
}

func compactSoulMap(soul map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"id", "name", "description", "allowedToolKinds", "allowedMCPServerRefs", "allowedSkillRefs", "businessDomains"} {
		if value, ok := soul[key]; ok {
			out[key] = budgetProviderValue(value, 0, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 3)
		}
	}
	if md := strings.TrimSpace(stringValue(soul["soulMd"])); md != "" {
		out["soulMdSummary"] = truncateProviderBudgetString(md, 1024)
	}
	return out
}

func compactNamedMapSlice(input map[string]any, key string, limit int) {
	items, ok := input[key].([]any)
	if !ok || len(items) <= limit {
		return
	}
	input[key] = items[:limit]
	input[key+"TruncatedCount"] = len(items) - limit
}

func minimalProviderInput(input map[string]any) map[string]any {
	return prioritizedProviderInput(input, false)
}

func providerInputHasCurrentActionContract(input map[string]any) bool {
	return len(recordFromAny(input["currentActionContract"])) > 0
}

func narrowProviderInputToCurrentAction(input map[string]any) map[string]any {
	if !providerInputHasCurrentActionContract(input) {
		return input
	}
	return prioritizedProviderInput(input, false)
}

func prioritizedProviderInput(input map[string]any, includeP2 bool) map[string]any {
	out := map[string]any{}
	for _, key := range providerP0InputKeys {
		if value, ok := input[key]; ok {
			maxDepth := 2
			if key == "coverageWorkItemContext" || key == "stagePacket" || key == "evidenceLedger" {
				maxDepth = 4
			}
			out[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, providerBudgetAggressiveItems, providerBudgetAggressiveEntries, maxDepth)
		}
	}
	for _, key := range providerP1InputKeys {
		if value, ok := input[key]; ok {
			out[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, providerBudgetAggressiveItems, providerBudgetAggressiveEntries, 2)
		}
	}
	if includeP2 {
		for _, key := range providerP2InputKeys {
			if value, ok := input[key]; ok {
				out[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, providerBudgetAggressiveItems, providerBudgetAggressiveEntries, 2)
			}
		}
	}
	for _, key := range []string{"failureTargetRepairContext", "patchRepairContext", "repairActionPackage"} {
		if value, ok := input[key]; ok {
			out[key] = budgetProviderValue(value, 0, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 4)
		}
	}
	stage := strings.TrimSpace(strings.ToLower(stringValue(input["currentSoulStage"])))
	if executionContext := recordFromAny(input["executionContext"]); len(executionContext) > 0 {
		compact := map[string]any{}
		for _, key := range stageAwareExecutionContextKeys(stage, includeP2) {
			if value, ok := executionContext[key]; ok {
				maxDepth := 2
				if key == "coverageWorkItemContext" {
					maxDepth = 4
				}
				compact[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, providerBudgetAggressiveItems, providerBudgetAggressiveEntries, maxDepth)
			}
		}
		compactNamedMapSlice(compact, "governanceHistory", providerBudgetAggressiveItems)
		out["executionContext"] = compact
	}
	compactSoulRouting(out)
	return out
}

func stageAwareExecutionContextKeys(stage string, includeP2 bool) []string {
	keys := []string{"goal", "goals", "dod", "dods", "dialogLanguage", "goalDefinitionPolicy", "doDDefinitionPolicy", "coverageWorkItemContext"}
	switch stage {
	case "planning":
		keys = append(keys, "goals", "dods", "skillSelection", "memoryRetrieval", "workspaceManifest", "workspaceFileManifest")
	case "execution":
		keys = append(keys, "skillSelection", "memoryRetrieval", "goals", "dods")
	case "decision":
		keys = append(keys, "governanceHistory", "memoryRetrieval", "goals", "dods")
	case "approval":
		keys = append(keys, "governanceHistory", "memoryRetrieval", "goals", "dods")
	case "validation":
		keys = append(keys, "dods", "governanceHistory", "memoryRetrieval")
	case "artifact":
		keys = append(keys, "dods", "governanceHistory", "memoryRetrieval")
	default:
		keys = append(keys, "goals", "dods", "governanceHistory", "memoryRetrieval", "skillSelection")
	}
	if includeP2 {
		keys = append(keys, "graphEvidence", "runEvidenceSummary", "failedExecutionGraph")
	}
	return uniqueStrings(keys)
}

func truncateProviderBudgetString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func providerInputSourceRefs(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	refs := make([]string, 0, 12)
	for key := range input {
		switch strings.TrimSpace(key) {
		case "executionContext", "executionContextPackage", "workspaceFileManifest", "workspaceManifest",
			"graphEvidence", "runEvidenceSummary", "processEvidenceTimeline", "currentProgressDigest", "failedExecutionGraph", "failedStepInput", "failedStepOutput",
			"decisionResolution", "approvalResolution", "memoryRetrieval", "skillSelection", "coverageWorkItemContext", "evidenceLedger", "currentActionContract":
			refs = append(refs, "input."+key)
		}
	}
	if executionContext := recordFromAny(input["executionContext"]); len(executionContext) > 0 {
		for key := range executionContext {
			switch strings.TrimSpace(key) {
			case "goal", "goals", "dod", "dods", "governanceHistory", "memoryRetrieval", "skillSelection", "coverageWorkItemContext":
				refs = append(refs, "input.executionContext."+key)
			}
		}
	}
	if executionContextPackage := recordFromAny(input["executionContextPackage"]); len(executionContextPackage) > 0 {
		for key := range executionContextPackage {
			switch strings.TrimSpace(key) {
			case "processEvidenceTimeline", "currentProgressDigest", "recentFailureSummary", "selectedContext", "coverageWorkItemContext", "evidenceLedger":
				refs = append(refs, "input.executionContextPackage."+key)
			}
		}
	}
	if repairActionPackage := recordFromAny(input["repairActionPackage"]); len(repairActionPackage) > 0 {
		for key := range repairActionPackage {
			switch strings.TrimSpace(key) {
			case "currentExecutionProgress", "currentFailure", "lastFailureDiagnostics", "previousRejectedOutputSummary", "coverageTargetSet", "coverageWorkItemContext":
				refs = append(refs, "input.repairActionPackage."+key)
			}
		}
	}
	return uniqueStrings(refs)
}

func providerProtectedContextFields(input map[string]any) []string {
	fields := []string{}
	for _, key := range []string{"goal", "goals", "dod", "dods"} {
		if _, ok := input[key]; ok {
			fields = append(fields, "input."+key)
		}
	}
	executionContext := recordFromAny(input["executionContext"])
	for _, key := range []string{"goal", "goals", "dod", "dods", "goalDefinitionPolicy", "doDDefinitionPolicy", "definitionChangeImpact"} {
		if _, ok := executionContext[key]; ok {
			fields = append(fields, "input.executionContext."+key)
		}
	}
	executionContextPackage := recordFromAny(input["executionContextPackage"])
	for _, key := range []string{"goal", "goals", "dod", "dods", "goalDefinitionPolicy", "doDDefinitionPolicy", "definitionChangeImpact", "executionContract", "stage", "stageMode"} {
		if _, ok := executionContextPackage[key]; ok {
			fields = append(fields, "input.executionContextPackage."+key)
		}
	}
	for _, key := range []string{"failureTargetRepairContext", "patchRepairContext"} {
		if len(providerSafeRepairSnapshotsFromValue(input[key])) > 0 {
			fields = append(fields, "input."+key)
		}
	}
	for _, key := range []string{
		"assessmentOnly", "assessmentOnlyReason", "assessmentOnlyContractCorrection",
		"graphExpansionLimitFollowUp", "graphFollowUpExpansionCount", "graphFollowUpExpansionLimit",
		"providerRetryAssessmentOnly",
		"runtimeStage", "runtimeStageMode", "currentRuntimeStage", "currentRuntimeStageMode", "currentSoulStage",
		"currentActionContract",
		"coverageWorkItemContext",
		"evidenceLedger",
	} {
		if _, ok := input[key]; ok {
			fields = append(fields, "input."+key)
		}
	}
	if packet := recordFromAny(input["stagePacket"]); len(packet) > 0 {
		fields = append(fields, "input.stagePacket")
		for _, key := range []string{
			"schemaVersion",
			"workspaceId",
			"runId",
			"stepId",
			"definition",
			"activeStage",
			"activeFocus",
			"nextAction",
			"requiredOutput",
			"allowedActions",
			"forbiddenActions",
			"requiredEvidenceKinds",
			"acceptedEvidenceRefs",
			"rejectedEvidenceRefs",
			"currentFailure",
			"currentGap",
			"processEvidenceTimeline",
			"targetSnapshots",
			"idempotencyKey",
		} {
			if _, ok := packet[key]; ok {
				fields = append(fields, "input.stagePacket."+key)
			}
		}
	}
	if current := recordFromAny(input["currentActionContract"]); len(current) > 0 {
		fields = append(fields, "input.currentActionContract")
		for _, key := range []string{
			"schemaVersion",
			"workspaceId",
			"runId",
			"stepId",
			"definitionRevision",
			"nextAction",
			"currentFocus",
			"requiredOutput",
			"targetSnapshots",
			"outputDecisionContract",
			"latestRejectedDiagnostic",
		} {
			if _, ok := current[key]; ok {
				fields = append(fields, "input.currentActionContract."+key)
			}
		}
	}
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		for _, key := range []string{"coverageTargetSet", "currentExecutionProgress", "currentFailure", "lastFailureDiagnostics", "requiredOutput", "outputDecisionContract", "lastRejectedOutputContract", "targetSnapshots"} {
			if _, ok := pkg[key]; ok {
				fields = append(fields, "input.repairActionPackage."+key)
			}
		}
	}
	if len(coverageWorkItemContextFromProviderInput(input)) > 0 {
		fields = append(fields, "input.coverageWorkItemContext")
	}
	return uniqueStrings(fields)
}

func providerContextIntegrityWarnings(input map[string]any, protected protectedGoalDoD, repairTargets protectedProviderRepairTargets, runtimeContract protectedProviderRuntimeContract, coverageContext protectedProviderCoverageWorkItemContext, stagePacket protectedProviderStagePacket, profile llmmodels.ProviderContextProfile, overBudget bool) []string {
	warnings := []string{}
	if !isProtectedGoalDoDEqual(input, protected) {
		warnings = append(warnings, "protected_goal_dod_changed_after_context_budget")
	}
	if !isProtectedProviderRuntimeContractEqual(input, runtimeContract) {
		warnings = append(warnings, "protected_runtime_contract_changed_after_context_budget")
	}
	if !isProtectedProviderCoverageWorkItemContextEqual(input, coverageContext) {
		warnings = append(warnings, "protected_coverage_work_item_context_changed_after_context_budget")
	}
	if !isProtectedProviderStagePacketEqual(input, stagePacket) {
		warnings = append(warnings, "protected_stage_packet_changed_after_context_budget")
	}
	if overBudget && hasProtectedGoalDoD(protected) {
		warnings = append(warnings, "request_still_over_budget_with_protected_goal_dod")
	}
	pkg := recordFromAny(input["repairActionPackage"])
	if (boolValue(input["coverageRepairRequired"]) || boolValue(pkg["coverageRepairRequired"])) && len(coverageTargetsFromProviderInput(input)) == 0 {
		warnings = append(warnings, "coverage_repair_required_without_coverage_target_set")
	}
	if len(pkg) > 0 {
		required := recordFromAny(pkg["requiredOutput"])
		if boolValue(required["mustDeclareCoverageTargets"]) && len(coverageTargetsFromProviderInput(input)) == 0 {
			warnings = append(warnings, "must_declare_coverage_targets_without_target_set")
		}
		if providerInputRequiresRepairActionPackageRequiredOutput(input, pkg) && len(required) == 0 {
			warnings = append(warnings, "repair_action_package_required_output_missing")
		}
	}
	if boolValue(input["runtimeRepairFollowUp"]) && !providerInputHasCurrentFailure(input) {
		warnings = append(warnings, "runtime_repair_current_failure_missing")
	}
	if providerInputRequiresProcessEvidence(input) && !providerInputHasProcessEvidence(input) {
		warnings = append(warnings, "runtime_repair_process_evidence_missing")
	}
	warnings = append(warnings, providerCoverageWorkItemWarnings(input)...)
	warnings = append(warnings, providerSemanticContextWarnings(input)...)
	warnings = append(warnings, missingProtectedRepairSnapshotWarnings(input, repairTargets)...)
	return uniqueStrings(warnings)
}

func providerInputRequiresRepairActionPackageRequiredOutput(input map[string]any, pkg map[string]any) bool {
	if len(input) == 0 || len(pkg) == 0 {
		return false
	}
	contract := recordFromAny(input["convergenceContract"])
	if boolValue(input["assessmentOnly"]) ||
		stringValue(input["repairObjective"]) == "complete_dod_goal_coverage_assessment" ||
		stringValue(contract["state"]) == "need_assessment" ||
		stringValue(contract["state"]) == "need_governance" {
		return false
	}
	return boolValue(input["runtimeRepairFollowUp"]) ||
		boolValue(input["stateChangingOutputRequired"]) ||
		boolValue(input["concreteFailureEvidenceRequired"]) ||
		boolValue(input["boundedValidationRequired"]) ||
		boolValue(input["evidenceRepairRequired"]) ||
		boolValue(pkg["evidenceAcquisitionRequired"])
}

func providerSemanticContextWarnings(input map[string]any) []string {
	warnings := []string{}
	if len(input) == 0 {
		return nil
	}
	selected := recordFromAny(recordFromAny(input["executionContextPackage"])["selectedContext"])
	if len(selected) == 0 {
		selected = recordFromAny(input["selectedContext"])
	}
	retrieval := recordFromAny(selected["memoryRetrieval"])
	memories := mapSliceFromAny(selected["memories"])
	selectedIDs := stringSetFromProviderValue(retrieval["selectedMemoryIds"])
	excludedIDs := stringSetFromProviderValue(retrieval["excludedMemoryIds"])
	if len(memories) > 0 && len(retrieval) > 0 && len(selectedIDs) == 0 {
		warnings = append(warnings, "selected_context_memories_without_selected_memory_ids")
	}
	for _, memory := range memories {
		id := strings.TrimSpace(firstProviderString(memory["id"], memory["ID"]))
		if id == "" {
			continue
		}
		if excludedIDs[id] || (len(selectedIDs) > 0 && !selectedIDs[id]) {
			warnings = append(warnings, "selected_context_memories_include_unselected_or_excluded_memory")
			break
		}
	}
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		for _, target := range stringSliceFromAny(pkg["repairTargetSet"]) {
			if !providerRepairTargetRefAdmissible(target) {
				warnings = append(warnings, "inadmissible_repair_target_set")
				break
			}
		}
		for _, item := range mapSliceFromAny(pkg["targetSnapshots"]) {
			if !providerRepairTargetRefAdmissible(firstProviderString(item["targetRef"])) || boolValue(item["isBinary"]) {
				warnings = append(warnings, "inadmissible_repair_target_snapshot")
				break
			}
		}
	}
	return warnings
}

func providerContextGateDecision(goalDoDCompressed, overBudget, p0Overflow bool, integrityWarnings []string) (string, string) {
	switch {
	case goalDoDCompressed:
		return providerContextGateFailFast, "protected Goal/DoD contract changed during context budgeting"
	case p0Overflow:
		return providerContextGateFailFast, "P0 contract exceeds provider context budget"
	case overBudget:
		return providerContextGateFailFast, "provider request remains over context budget after compaction"
	}
	for _, warning := range integrityWarnings {
		warning = strings.TrimSpace(warning)
		if action := providerContextGateActionForWarning(warning); action != "" && action != providerContextGatePass {
			return action, warning
		}
	}
	return providerContextGatePass, ""
}

func providerContextGateActionForWarning(warning string) string {
	switch strings.TrimSpace(warning) {
	case "":
		return providerContextGatePass
	case "protected_failure_target_snapshot_missing_after_context_budget",
		"protected_patch_repair_snapshot_missing_after_context_budget",
		"protected_repair_action_target_snapshot_missing_after_context_budget",
		"selected_context_memories_without_selected_memory_ids",
		"selected_context_memories_include_unselected_or_excluded_memory":
		return providerContextGateReadRequired
	case "coverage_work_item_unresolved_without_current_focus",
		"coverage_work_item_current_focus_missing_minimum_fields":
		return providerContextGateDegradeToAssessment
	case "runtime_repair_current_failure_missing",
		"runtime_repair_process_evidence_missing":
		return providerContextGateBlockerRequired
	case "coverage_repair_required_without_coverage_target_set",
		"must_declare_coverage_targets_without_target_set":
		return providerContextGateReplanRequired
	case "protected_goal_dod_changed_after_context_budget",
		"protected_runtime_contract_changed_after_context_budget",
		"protected_coverage_work_item_context_changed_after_context_budget",
		"protected_stage_packet_changed_after_context_budget",
		"request_still_over_budget_with_protected_goal_dod",
		"repair_action_package_required_output_missing",
		"inadmissible_repair_target_set",
		"inadmissible_repair_target_snapshot":
		return providerContextGateFailFast
	default:
		return providerContextGateFailFast
	}
}

func providerInputRequiresProcessEvidence(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if boolValue(input["runtimeRepairFollowUp"]) {
		return true
	}
	if boolValue(input["contractViolationRetry"]) || len(recordFromAny(input["contractViolationRecovery"])) > 0 {
		return true
	}
	if len(recordFromAny(recordFromAny(input["executionContext"])["replanContinuation"])) > 0 ||
		len(recordFromAny(recordFromAny(input["executionContextPackage"])["replanContinuation"])) > 0 {
		return true
	}
	if strings.TrimSpace(stringValue(input["failedStepId"])) != "" ||
		len(recordFromAny(input["failedStepInput"])) > 0 ||
		len(recordFromAny(input["failedStepOutput"])) > 0 ||
		len(recordFromAny(input["lastFailureDiagnostics"])) > 0 {
		return true
	}
	return false
}

func providerInputHasProcessEvidence(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if len(mapSliceFromAny(input["processEvidenceTimeline"])) > 0 && len(recordFromAny(input["currentProgressDigest"])) > 0 {
		return true
	}
	if packet := recordFromAny(input["stagePacket"]); len(packet) > 0 &&
		len(mapSliceFromAny(packet["processEvidenceTimeline"])) > 0 &&
		(len(recordFromAny(packet["currentFailure"])) > 0 ||
			len(recordFromAny(packet["currentGap"])) > 0 ||
			len(recordFromAny(packet["nextAction"])) > 0) {
		return true
	}
	pkg := recordFromAny(input["executionContextPackage"])
	if len(mapSliceFromAny(pkg["processEvidenceTimeline"])) > 0 && len(recordFromAny(pkg["currentProgressDigest"])) > 0 {
		return true
	}
	replan := recordFromAny(pkg["replanContinuation"])
	if len(mapSliceFromAny(replan["processEvidenceTimeline"])) > 0 &&
		len(recordFromAny(replan["currentProgressDigest"])) > 0 &&
		len(recordFromAny(replan["failureContext"])) > 0 {
		return true
	}
	if len(recordFromAny(input["currentProgressDigest"])) > 0 &&
		len(recordFromAny(input["failedStepInput"])) > 0 &&
		len(recordFromAny(input["failedStepOutput"])) > 0 {
		return true
	}
	repairPkg := recordFromAny(input["repairActionPackage"])
	if len(recordFromAny(repairPkg["currentExecutionProgress"])) > 0 &&
		(len(recordFromAny(repairPkg["currentFailure"])) > 0 ||
			len(recordFromAny(repairPkg["previousRejectedOutputSummary"])) > 0 ||
			len(mapSliceFromAny(repairPkg["coverageTargetSet"])) > 0) {
		return true
	}
	return false
}

func providerInputHasCurrentFailure(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if len(recordFromAny(input["currentFailure"])) > 0 {
		return true
	}
	if len(recordFromAny(recordFromAny(input["repairActionPackage"])["currentFailure"])) > 0 {
		return true
	}
	for _, container := range []map[string]any{
		recordFromAny(input["stagePacket"]),
		recordFromAny(recordFromAny(input["executionContext"])["stagePacket"]),
		recordFromAny(recordFromAny(input["executionContextPackage"])["stagePacket"]),
	} {
		if len(recordFromAny(container["currentFailure"])) > 0 {
			return true
		}
	}
	for _, key := range []string{"failedStepId", "failureClass", "failureSignature", "failureReason"} {
		if strings.TrimSpace(stringValue(input[key])) != "" {
			return true
		}
	}
	failedOutput := recordFromAny(input["failedStepOutput"])
	for _, key := range []string{"error", "reason", "failureClass", "failureSignature", "stage"} {
		if strings.TrimSpace(stringValue(failedOutput[key])) != "" {
			return true
		}
	}
	diagnostics := recordFromAny(input["lastFailureDiagnostics"])
	for _, key := range []string{"error", "failureClass", "failureSignature", "stage"} {
		if strings.TrimSpace(stringValue(diagnostics[key])) != "" {
			return true
		}
	}
	return false
}

func coverageTargetsFromProviderInput(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	if targets := mapSliceFromAny(input["coverageTargetSet"]); len(targets) > 0 {
		return targets
	}
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		if targets := mapSliceFromAny(pkg["coverageTargetSet"]); len(targets) > 0 {
			return targets
		}
	}
	return nil
}

func providerCoverageWorkItemWarnings(input map[string]any) []string {
	context := coverageWorkItemContextFromProviderInput(input)
	if len(context) == 0 {
		return nil
	}
	warnings := []string{}
	summary := recordFromAny(context["summary"])
	unresolvedCount := 0
	for _, key := range []string{"open", "in_progress", "evidence_ready", "failed"} {
		unresolvedCount += intProviderValue(summary[key])
	}
	if unresolvedCount == 0 {
		unresolvedCount = len(mapSliceFromAny(context["unresolvedItems"]))
	}
	focuses := providerCoverageWorkItemFocuses(context)
	if unresolvedCount > 0 && len(focuses) == 0 {
		warnings = append(warnings, "coverage_work_item_unresolved_without_current_focus")
	}
	for _, focus := range focuses {
		missing := []string{}
		for _, key := range []string{"id", "definitionRevision", "sourceScope", "targetText", "currentGap", "status"} {
			if strings.TrimSpace(stringValue(focus[key])) == "" {
				missing = append(missing, key)
			}
		}
		if strings.TrimSpace(stringValue(focus["normalizedSourceKey"])) == "" &&
			strings.TrimSpace(stringValue(focus["sourceRequirementId"])) == "" &&
			strings.TrimSpace(stringValue(focus["sourcePath"])) == "" {
			missing = append(missing, "sourceRef")
		}
		if len(stringSliceFromAny(focus["missingEvidenceKinds"])) == 0 && len(stringSliceFromAny(focus["requiredEvidenceKinds"])) == 0 {
			missing = append(missing, "missingEvidenceKinds")
		}
		if _, ok := focus["acceptedEvidenceRefs"]; !ok {
			missing = append(missing, "acceptedEvidenceRefs")
		}
		if _, ok := focus["rejectedEvidenceRefs"]; !ok {
			missing = append(missing, "rejectedEvidenceRefs")
		}
		if len(stringSliceFromAny(focus["nextAllowedConvergenceStates"])) == 0 {
			missing = append(missing, "nextAllowedConvergenceStates")
		}
		if len(missing) > 0 {
			warnings = append(warnings, "coverage_work_item_current_focus_missing_minimum_fields")
			break
		}
	}
	return uniqueStrings(warnings)
}

func providerCoverageWorkItemFocuses(context map[string]any) []map[string]any {
	if len(context) == 0 {
		return nil
	}
	if focus := recordFromAny(context["currentFocus"]); len(focus) > 0 {
		return []map[string]any{focus}
	}
	if focuses := mapSliceFromAny(context["currentFocus"]); len(focuses) > 0 {
		return focuses
	}
	if focuses := mapSliceFromAny(context["currentFocusItems"]); len(focuses) > 0 {
		return focuses
	}
	return nil
}

func coverageWorkItemContextFromProviderInput(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		input,
		recordFromAny(input["executionContext"]),
		recordFromAny(input["executionContextPackage"]),
		recordFromAny(input["repairActionPackage"]),
	} {
		if context := recordFromAny(source["coverageWorkItemContext"]); len(context) > 0 {
			return context
		}
	}
	return nil
}

func intProviderValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	default:
		return 0
	}
}

func missingProtectedRepairSnapshotWarnings(input map[string]any, protected protectedProviderRepairTargets) []string {
	warnings := []string{}
	if protected.FailureTargetRepairContext != nil && len(providerSafeRepairSnapshotsFromValue(input["failureTargetRepairContext"])) == 0 {
		warnings = append(warnings, "protected_failure_target_snapshot_missing_after_context_budget")
	}
	if protected.PatchRepairContext != nil && len(providerSafeRepairSnapshotsFromValue(input["patchRepairContext"])) == 0 {
		warnings = append(warnings, "protected_patch_repair_snapshot_missing_after_context_budget")
	}
	if protected.RepairActionPackage != nil && protected.RepairActionTargetSnapshots != nil {
		pkg := recordFromAny(input["repairActionPackage"])
		if len(providerSafeRepairSnapshotsFromValue(pkg["targetSnapshots"])) == 0 {
			warnings = append(warnings, "protected_repair_action_target_snapshot_missing_after_context_budget")
		}
	}
	return warnings
}

type protectedGoalDoD struct {
	TopLevel         map[string]any
	ExecutionContext map[string]any
	Package          map[string]any
}

type protectedProviderRepairTargets struct {
	FailureTargetRepairContext  any
	PatchRepairContext          any
	RepairActionPackage         any
	RepairActionTargetSnapshots any
}

type protectedProviderRuntimeContract struct {
	TopLevel map[string]any
}

type protectedProviderCoverageWorkItemContext struct {
	TopLevel         map[string]any
	ExecutionContext map[string]any
	Package          map[string]any
	RepairPackage    map[string]any
}

type protectedProviderStagePacket struct {
	Packet map[string]any
}

func captureProtectedGoalDoD(input map[string]any) protectedGoalDoD {
	ret := protectedGoalDoD{
		TopLevel:         map[string]any{},
		ExecutionContext: map[string]any{},
		Package:          map[string]any{},
	}
	for _, key := range []string{"goal", "goals", "dod", "dods"} {
		if value, ok := input[key]; ok {
			if cloned, success := cloneAny(value); success {
				ret.TopLevel[key] = cloned
			}
		}
	}
	executionContext := recordFromAny(input["executionContext"])
	for _, key := range []string{"goal", "goals", "dod", "dods", "goalDefinitionPolicy", "doDDefinitionPolicy", "definitionChangeImpact"} {
		if value, ok := executionContext[key]; ok {
			if cloned, success := cloneAny(value); success {
				ret.ExecutionContext[key] = cloned
			}
		}
	}
	executionContextPackage := recordFromAny(input["executionContextPackage"])
	for _, key := range []string{"goal", "goals", "dod", "dods", "goalDefinitionPolicy", "doDDefinitionPolicy", "definitionChangeImpact", "executionContract", "stage", "stageMode"} {
		if value, ok := executionContextPackage[key]; ok {
			if cloned, success := cloneAny(value); success {
				ret.Package[key] = cloned
			}
		}
	}
	return ret
}

func captureProtectedProviderCoverageWorkItemContext(input map[string]any, adaptiveLevel int) protectedProviderCoverageWorkItemContext {
	ret := protectedProviderCoverageWorkItemContext{}
	if len(input) == 0 {
		return ret
	}
	if adaptiveLevel >= 2 {
		if context := coverageWorkItemContextFromProviderInput(input); len(context) > 0 {
			ret.TopLevel = compactProviderCoverageWorkItemContextForAdaptiveRetry(context, adaptiveLevel)
		}
		return ret
	}
	if context := recordFromAny(input["coverageWorkItemContext"]); len(context) > 0 {
		ret.TopLevel = compactProviderCoverageWorkItemContext(context)
	}
	if executionContext := recordFromAny(input["executionContext"]); len(executionContext) > 0 {
		if context := recordFromAny(executionContext["coverageWorkItemContext"]); len(context) > 0 {
			ret.ExecutionContext = compactProviderCoverageWorkItemContext(context)
		}
	}
	if executionContextPackage := recordFromAny(input["executionContextPackage"]); len(executionContextPackage) > 0 {
		if context := recordFromAny(executionContextPackage["coverageWorkItemContext"]); len(context) > 0 {
			ret.Package = compactProviderCoverageWorkItemContext(context)
		}
	}
	if repairActionPackage := recordFromAny(input["repairActionPackage"]); len(repairActionPackage) > 0 {
		if context := recordFromAny(repairActionPackage["coverageWorkItemContext"]); len(context) > 0 {
			ret.RepairPackage = compactProviderCoverageWorkItemContext(context)
		}
	}
	return ret
}

func captureProtectedProviderStagePacket(input map[string]any) protectedProviderStagePacket {
	ret := protectedProviderStagePacket{}
	packet := recordFromAny(input["stagePacket"])
	if len(packet) == 0 {
		return ret
	}
	if providerInputHasCurrentActionContract(input) {
		ret.Packet = compactProtectedProviderStagePacketForCurrentAction(packet)
		return ret
	}
	ret.Packet = compactProtectedProviderStagePacket(packet)
	return ret
}

func compactProtectedProviderStagePacketForCurrentAction(packet map[string]any) map[string]any {
	if len(packet) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"schemaVersion", "workspaceId", "runId", "stepId", "idempotencyKey"} {
		copyMapValue(out, key, packet[key])
	}
	if snapshots := compactProtectedProviderStagePacketSnapshots(mapSliceFromAny(packet["targetSnapshots"])); len(snapshots) > 0 {
		out["targetSnapshots"] = snapshots
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compactProtectedProviderStagePacket(packet map[string]any) map[string]any {
	if len(packet) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"schemaVersion",
		"workspaceId",
		"runId",
		"stepId",
		"activeStage",
		"idempotencyKey",
	} {
		copyMapValue(out, key, packet[key])
	}
	for _, key := range []string{
		"definition",
		"activeFocus",
		"nextAction",
		"requiredOutput",
		"currentFailure",
		"currentGap",
	} {
		copyMapValue(out, key, packet[key])
	}
	for _, key := range []string{
		"allowedActions",
		"forbiddenActions",
		"requiredEvidenceKinds",
		"acceptedEvidenceRefs",
		"rejectedEvidenceRefs",
	} {
		if values := stringSliceFromAny(packet[key]); len(values) > 0 {
			out[key] = values
		}
	}
	if snapshots := compactProtectedProviderStagePacketSnapshots(mapSliceFromAny(packet["targetSnapshots"])); len(snapshots) > 0 {
		out["targetSnapshots"] = snapshots
	}
	if timeline := mapSliceFromAny(packet["processEvidenceTimeline"]); len(timeline) > 0 {
		out["processEvidenceTimeline"] = budgetProviderValue(timeline, 0, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 3)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compactProtectedProviderStagePacketSnapshots(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		next := compactStagePacketTargetSnapshotForPrompt(item)
		if len(next) == 0 {
			continue
		}
		ret = append(ret, next)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func captureProtectedProviderRepairTargets(input map[string]any) protectedProviderRepairTargets {
	ret := protectedProviderRepairTargets{}
	if len(input) == 0 {
		return ret
	}
	if items := providerSafeRepairSnapshotsFromValue(input["failureTargetRepairContext"]); len(items) > 0 {
		ret.FailureTargetRepairContext = items
	}
	if items := providerSafeRepairSnapshotsFromValue(input["patchRepairContext"]); len(items) > 0 {
		ret.PatchRepairContext = items
	}
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		pkgOut := map[string]any{}
		for _, key := range []string{"requiredOutput", "outputDecisionContract", "lastRejectedOutputContract", "coverageRepairRequired", "coverageTargetSet", "currentExecutionProgress", "currentFailure", "lastFailureDiagnostics", "structuredFailureSignals", "patchRepairSuggestions", "previousRejectedOutputSummary", "strategyReassessment"} {
			if value, ok := pkg[key]; ok {
				pkgOut[key] = budgetProviderValue(value, 0, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 4)
			}
		}
		if targets := providerSafeRepairSnapshotsFromValue(pkg["targetSnapshots"]); len(targets) > 0 {
			pkgOut["targetSnapshots"] = targets
			ret.RepairActionTargetSnapshots = targets
		}
		if len(pkgOut) > 0 {
			ret.RepairActionPackage = pkgOut
		}
	}
	return ret
}

func captureProtectedProviderRuntimeContract(input map[string]any, adaptiveLevel int) protectedProviderRuntimeContract {
	ret := protectedProviderRuntimeContract{TopLevel: map[string]any{}}
	if len(input) == 0 {
		return ret
	}
	for _, key := range []string{
		"assessmentOnly", "assessmentOnlyReason", "assessmentOnlyContractCorrection",
		"graphExpansionLimitFollowUp", "graphFollowUpExpansionCount", "graphFollowUpExpansionLimit",
		"providerRetryAssessmentOnly",
		"runtimeStage", "runtimeStageMode", "currentRuntimeStage", "currentRuntimeStageMode", "currentSoulStage",
		"contractViolationRecovery",
		"currentActionContract",
	} {
		if value, ok := input[key]; ok {
			if key == "contractViolationRecovery" && adaptiveLevel >= 2 {
				value = compactProviderContractViolationRecovery(recordFromAny(value), adaptiveLevel)
			}
			if cloned, success := cloneAny(value); success {
				ret.TopLevel[key] = cloned
			}
		}
	}
	return ret
}

func compactProviderCoverageWorkItemContextForAdaptiveRetry(context map[string]any, adaptiveLevel int) map[string]any {
	out := compactProviderCoverageWorkItemContext(context)
	if len(out) == 0 {
		return nil
	}
	adjacentLimit := 2
	ledgerLimit := 3
	if adaptiveLevel >= 3 {
		adjacentLimit = 1
		ledgerLimit = 1
	}
	if adaptiveLevel >= 4 {
		adjacentLimit = 0
		ledgerLimit = 0
	}
	if adjacentLimit == 0 {
		delete(out, "currentFocusItems")
		delete(out, "unresolvedItems")
	} else {
		out["currentFocusItems"] = compactProviderCoverageWorkItemFocuses(mapSliceFromAny(out["currentFocusItems"]), adjacentLimit)
		out["unresolvedItems"] = compactProviderCoverageWorkItemSummaries(mapSliceFromAny(out["unresolvedItems"]), adjacentLimit)
	}
	if ledgerLimit == 0 {
		delete(out, "ledgerSummary")
	} else {
		out["ledgerSummary"] = compactProviderCoverageWorkItemSummaries(mapSliceFromAny(out["ledgerSummary"]), ledgerLimit)
	}
	return out
}

func compactProviderContractViolationRecovery(recovery map[string]any, adaptiveLevel int) map[string]any {
	if len(recovery) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"id", "sourceStepId", "stepId", "violationType", "violatedRule", "recoverability",
		"retryHint", "correctionProfileId", "correctionMode", "targetScope", "attempt", "maxAttempts",
		"signatureRepeatCount", "budgetScope", "actualOutputSummary", "correctionContract",
	} {
		if value, ok := recovery[key]; ok {
			out[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, providerBudgetAggressiveItems, providerBudgetAggressiveEntries, 3)
		}
	}
	if adaptiveLevel == 2 {
		if value, ok := recovery["rejectedOutputRef"]; ok {
			out["rejectedOutputRef"] = value
		}
	}
	return out
}

func normalizeAdaptiveProviderRecoveryTail(input map[string]any, adaptiveLevel int) map[string]any {
	if len(input) == 0 || adaptiveLevel < 2 {
		return input
	}
	if context := coverageWorkItemContextFromProviderInput(input); len(context) > 0 {
		input["coverageWorkItemContext"] = compactProviderCoverageWorkItemContextForAdaptiveRetry(context, adaptiveLevel)
	}
	for _, containerKey := range []string{"executionContext", "executionContextPackage"} {
		container := recordFromAny(input[containerKey])
		if len(container) == 0 {
			continue
		}
		delete(container, "coverageWorkItemContext")
		input[containerKey] = container
	}
	if recovery := recordFromAny(input["contractViolationRecovery"]); len(recovery) > 0 {
		input["contractViolationRecovery"] = compactProviderContractViolationRecovery(recovery, adaptiveLevel)
	}
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		input["repairActionPackage"] = compactProviderRepairActionPackageForAdaptiveRetry(pkg, adaptiveLevel)
	}
	for _, key := range []string{"processEvidenceTimeline", "evidenceLedger", "failedExecutionGraph", "graphEvidence", "previousResults"} {
		delete(input, key)
	}
	return input
}

func compactProviderRepairActionPackageForAdaptiveRetry(pkg map[string]any, adaptiveLevel int) map[string]any {
	if len(pkg) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"repairObjective", "requiredOutput", "outputDecisionContract", "coverageRepairRequired",
		"currentFailure", "structuredFailureSignals", "targetSnapshots", "repairTargetSet",
		"evidenceAcquisitionRequired", "boundedValidationRequired", "validationRoute",
	} {
		if value, ok := pkg[key]; ok {
			out[key] = value
		}
	}
	coverageLimit := 3
	if adaptiveLevel >= 3 {
		coverageLimit = 2
	}
	if adaptiveLevel >= 4 {
		coverageLimit = 1
	}
	if targets := compactProviderCoverageWorkItemSummaries(mapSliceFromAny(pkg["coverageTargetSet"]), coverageLimit); len(targets) > 0 {
		out["coverageTargetSet"] = targets
	}
	if progress := compactProviderCurrentProgressForAdaptiveRetry(recordFromAny(pkg["currentExecutionProgress"]), adaptiveLevel); len(progress) > 0 {
		out["currentExecutionProgress"] = progress
	}
	if adaptiveLevel == 2 {
		for _, key := range []string{"previousRejectedOutputSummary", "strategyReassessment"} {
			if value, ok := pkg[key]; ok {
				out[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, 2, providerBudgetAggressiveEntries, 2)
			}
		}
	}
	return out
}

func compactProviderCurrentProgressForAdaptiveRetry(progress map[string]any, adaptiveLevel int) map[string]any {
	if len(progress) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"failureReason", "failureClass", "failureSignature", "currentGap", "currentFailure", "lastObservedFailure"} {
		if value, ok := progress[key]; ok {
			out[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, 2, providerBudgetAggressiveEntries, 2)
		}
	}
	if adaptiveLevel <= 3 {
		for _, key := range []string{"latestActionEvidence", "completedGraphEvidence"} {
			items := mapSliceFromAny(progress[key])
			if len(items) > 0 {
				out[key] = budgetProviderValue(items[:1], 0, providerBudgetAggressiveBytes, 1, providerBudgetAggressiveEntries, 2)
			}
		}
	}
	return out
}

func restoreProtectedProviderRepairTargets(input map[string]any, protected protectedProviderRepairTargets) map[string]any {
	if len(input) == 0 {
		input = map[string]any{}
	}
	if protected.FailureTargetRepairContext != nil {
		input["failureTargetRepairContext"] = protected.FailureTargetRepairContext
	}
	if protected.PatchRepairContext != nil {
		input["patchRepairContext"] = protected.PatchRepairContext
	}
	if protected.RepairActionPackage != nil {
		input["repairActionPackage"] = protected.RepairActionPackage
	}
	return input
}

func restoreProtectedProviderRuntimeContract(input map[string]any, protected protectedProviderRuntimeContract) map[string]any {
	if len(input) == 0 {
		input = map[string]any{}
	}
	for key, value := range protected.TopLevel {
		input[key] = value
	}
	return input
}

func restoreProtectedProviderCoverageWorkItemContext(input map[string]any, protected protectedProviderCoverageWorkItemContext) map[string]any {
	if len(input) == 0 {
		input = map[string]any{}
	}
	if len(protected.TopLevel) > 0 {
		input["coverageWorkItemContext"] = protected.TopLevel
	}
	if len(protected.ExecutionContext) > 0 {
		executionContext := recordFromAny(input["executionContext"])
		if executionContext == nil {
			executionContext = map[string]any{}
		}
		executionContext["coverageWorkItemContext"] = protected.ExecutionContext
		input["executionContext"] = executionContext
	}
	if len(protected.Package) > 0 {
		executionContextPackage := recordFromAny(input["executionContextPackage"])
		if executionContextPackage == nil {
			executionContextPackage = map[string]any{}
		}
		executionContextPackage["coverageWorkItemContext"] = protected.Package
		input["executionContextPackage"] = executionContextPackage
	}
	if len(protected.RepairPackage) > 0 {
		repairActionPackage := recordFromAny(input["repairActionPackage"])
		if repairActionPackage == nil {
			repairActionPackage = map[string]any{}
		}
		repairActionPackage["coverageWorkItemContext"] = protected.RepairPackage
		input["repairActionPackage"] = repairActionPackage
	}
	return restoreProviderCoverageWorkItemContext(input)
}

func restoreProtectedProviderStagePacket(input map[string]any, protected protectedProviderStagePacket) map[string]any {
	if len(protected.Packet) == 0 {
		return input
	}
	if len(input) == 0 {
		input = map[string]any{}
	}
	input["stagePacket"] = protected.Packet
	return input
}

func providerSafeRepairSnapshotsFromValue(value any) []any {
	items := providerRepairSnapshotItemsFromValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]any, 0, len(items))
	for _, item := range items {
		if !providerSafeRepairSnapshotContent(item) {
			continue
		}
		if cloned, ok := cloneAny(item); ok {
			if mapped, ok := cloned.(map[string]any); ok {
				ret = append(ret, mapped)
			}
		}
	}
	return ret
}

func providerRepairSnapshotItemsFromValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped := recordFromAny(item); len(mapped) > 0 {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

func providerSafeRepairSnapshotContent(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	if strings.TrimSpace(rawStringValue(item["content"])) == "" || strings.TrimSpace(stringValue(item["contentHash"])) == "" {
		return false
	}
	return boolValue(item["contentComplete"]) &&
		boolValue(item["safeForFileWrite"]) &&
		!boolValue(item["redacted"]) &&
		!boolValue(item["truncated"]) &&
		!boolValue(item["isBinary"])
}

func restoreProtectedGoalDoD(input map[string]any, protected protectedGoalDoD) map[string]any {
	if len(input) == 0 {
		input = map[string]any{}
	}
	for key, value := range protected.TopLevel {
		input[key] = value
	}
	executionContext := recordFromAny(input["executionContext"])
	if executionContext == nil {
		executionContext = map[string]any{}
	}
	for key, value := range protected.ExecutionContext {
		executionContext[key] = value
	}
	if len(executionContext) > 0 {
		input["executionContext"] = executionContext
	}
	if len(protected.Package) > 0 {
		executionContextPackage := recordFromAny(input["executionContextPackage"])
		if executionContextPackage == nil {
			executionContextPackage = map[string]any{}
		}
		for key, value := range protected.Package {
			executionContextPackage[key] = value
		}
		input["executionContextPackage"] = executionContextPackage
	}
	return input
}

func isProtectedGoalDoDEqual(input map[string]any, protected protectedGoalDoD) bool {
	current := captureProtectedGoalDoD(input)
	return valuesEqual(current.TopLevel, protected.TopLevel) &&
		valuesEqual(current.ExecutionContext, protected.ExecutionContext) &&
		valuesEqual(current.Package, protected.Package)
}

func hasProtectedGoalDoD(protected protectedGoalDoD) bool {
	return len(protected.TopLevel) > 0 || len(protected.ExecutionContext) > 0 || len(protected.Package) > 0
}

func isProtectedProviderRuntimeContractEqual(input map[string]any, protected protectedProviderRuntimeContract) bool {
	if len(protected.TopLevel) == 0 {
		return true
	}
	current := captureProtectedProviderRuntimeContract(input, 0)
	return valuesEqual(current.TopLevel, protected.TopLevel)
}

func isProtectedProviderCoverageWorkItemContextEqual(input map[string]any, protected protectedProviderCoverageWorkItemContext) bool {
	if len(protected.TopLevel) == 0 &&
		len(protected.ExecutionContext) == 0 &&
		len(protected.Package) == 0 &&
		len(protected.RepairPackage) == 0 {
		return true
	}
	current := captureProtectedProviderCoverageWorkItemContext(input, 0)
	if len(protected.TopLevel) > 0 {
		return valuesEqual(current.TopLevel, protected.TopLevel)
	}
	return valuesEqual(current.TopLevel, protected.TopLevel) &&
		valuesEqual(current.ExecutionContext, protected.ExecutionContext) &&
		valuesEqual(current.Package, protected.Package) &&
		valuesEqual(current.RepairPackage, protected.RepairPackage)
}

func isProtectedProviderStagePacketEqual(input map[string]any, protected protectedProviderStagePacket) bool {
	if len(protected.Packet) == 0 {
		return true
	}
	current := captureProtectedProviderStagePacket(input)
	return valuesEqual(current.Packet, protected.Packet)
}

func withoutProtectedGoalDoD(input map[string]any) map[string]any {
	clonedInput := map[string]any{}
	if cloned, ok := cloneAny(input); ok {
		if typed, ok := cloned.(map[string]any); ok {
			clonedInput = typed
		}
	}
	if len(clonedInput) == 0 {
		clonedInput = map[string]any{}
		for key, value := range input {
			clonedInput[key] = value
		}
	}
	for _, key := range []string{"goal", "goals", "dod", "dods"} {
		delete(clonedInput, key)
	}
	executionContext := recordFromAny(clonedInput["executionContext"])
	if len(executionContext) > 0 {
		for _, key := range []string{"goal", "goals", "dod", "dods", "goalDefinitionPolicy", "doDDefinitionPolicy", "definitionChangeImpact"} {
			delete(executionContext, key)
		}
		clonedInput["executionContext"] = executionContext
	}
	executionContextPackage := recordFromAny(clonedInput["executionContextPackage"])
	if len(executionContextPackage) > 0 {
		for _, key := range []string{"goal", "goals", "dod", "dods", "goalDefinitionPolicy", "doDDefinitionPolicy", "definitionChangeImpact", "executionContract", "stage", "stageMode"} {
			delete(executionContextPackage, key)
		}
		clonedInput["executionContextPackage"] = executionContextPackage
	}
	return clonedInput
}

func valuesEqual(left, right map[string]any) bool {
	leftPayload, leftErr := json.Marshal(left)
	rightPayload, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftPayload) == string(rightPayload)
}

func stringSetFromProviderValue(value any) map[string]bool {
	ret := map[string]bool{}
	for _, item := range stringSliceFromAny(value) {
		item = strings.TrimSpace(item)
		if item != "" {
			ret[item] = true
		}
	}
	return ret
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(firstProviderString(item)); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		if text := strings.TrimSpace(firstProviderString(value)); text != "" {
			return []string{text}
		}
		return nil
	}
}

func firstProviderString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case fmt.Stringer:
			if text := strings.TrimSpace(typed.String()); text != "" {
				return text
			}
		}
	}
	return ""
}

func providerRepairTargetRefAdmissible(value string) bool {
	value = strings.TrimSpace(strings.Trim(value, `"'()[]{}<>;,`))
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") || strings.Contains(value, "://") || strings.Contains(value, " ") || strings.Contains(value, ":") {
		return false
	}
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	clean := strings.Trim(value, "/")
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	lower := strings.ToLower(clean)
	if strings.Contains(lower, ".workorch-data/") || strings.Contains(lower, ".workorch-workspaces/") ||
		strings.HasPrefix(lower, ".workorch-data") || strings.HasPrefix(lower, ".workorch-workspaces") {
		return false
	}
	parts := strings.Split(lower, "/")
	if len(parts) >= 3 {
		switch parts[0] {
		case "home", "users", "var", "tmp", "private", "opt", "usr":
			return false
		}
	}
	base := parts[len(parts)-1]
	if base == "llmexecutiondecision.plan" || strings.HasPrefix(base, "llmexecutiondecision.") {
		return false
	}
	switch base {
	case "request.json", "response.json", "repair-response.json", "usage.json", "metadata.json", "output.json", "input.json", "run.json":
		return false
	default:
		return true
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}
