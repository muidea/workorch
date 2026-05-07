package llm

import (
	"encoding/json"
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
)

var providerP0InputKeys = []string{
	"workspaceId", "goalId", "doDId", "previousStepId", "currentSoulStage",
	"runtimeRepairFollowUp", "runtimeRepairAttempt", "providerRetryFollowUp", "providerRetryAttempt",
	"completionFollowUp", "graphFollowUp", "failureReason", "failureClass", "failureSignature",
	"decisionResolution", "approvalResolution",
}

var providerP1InputKeys = []string{
	"failedStepInput", "failedStepOutput", "skillSelection", "memoryRetrieval",
}

var providerP2InputKeys = []string{
	"failedExecutionGraph", "graphEvidence", "workspaceFileManifest", "workspaceManifest", "previousResults",
}

func enforceProviderRequestBudget(request llmmodels.ProviderRequest) llmmodels.ProviderRequest {
	profile := normalizedProviderContextProfile(request.Config.ContextProfile)
	return enforceProviderRequestBudgetLimit(request, profile.BytesHardLimit)
}

func enforceProviderRequestBudgetLimit(request llmmodels.ProviderRequest, maxBytes int) llmmodels.ProviderRequest {
	profile := normalizedProviderContextProfile(request.Config.ContextProfile)
	if maxBytes <= 0 {
		maxBytes = profile.BytesHardLimit
	}
	if maxBytes <= 0 {
		maxBytes = defaultProviderRequestBudgetBytes
	}
	maxTokens := profile.MaxInputTokens
	rules := []string{"preserve identifiers, schema, config, runtime capabilities, goal, DoD, and governance decisions"}
	request.ContextBudget = nil
	shrinkLevel := 0
	if adaptiveLevel := adaptiveShrinkLevelFromInput(request.Input); adaptiveLevel > 0 {
		request.Input = applyAdaptiveShrinkLevel(request.Input, adaptiveLevel)
		rules = append(rules, "adaptive shrink based on provider retry")
		shrinkLevel = adaptiveLevel
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		request.Input = budgetProviderInput(request.Input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 4)
		rules = append(rules, "compact provider input")
		shrinkLevel = max(shrinkLevel, 1)
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		request.Input = compactProviderExecutionContext(request.Input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries)
		rules = append(rules, "summarize execution context transport fields")
		shrinkLevel = max(shrinkLevel, 2)
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		request.Input = prioritizedProviderInput(request.Input, true)
		rules = append(rules, "apply priority compaction (P0/P1/P2)")
		shrinkLevel = max(shrinkLevel, 3)
	}
	if exceedsProviderBudget(request, maxBytes, maxTokens, profile.EstimatedTokensPerByte) {
		request.Input = minimalProviderInput(request.Input)
		rules = append(rules, "fallback to stage-aware minimal provider input (P0/P1)")
		shrinkLevel = max(shrinkLevel, 4)
	}
	actual := providerRequestJSONBytes(request)
	estimatedInputTokens := estimateInputTokens(actual, profile.EstimatedTokensPerByte)
	estimatedOutputTokens := min(profile.ReservedOutputTokens, max(0, profile.ContextWindowTokens-estimatedInputTokens))
	actualTokens := estimatedInputTokens + estimatedOutputTokens
	truncated := len(rules) > 1 || actual > maxBytes || (maxTokens > 0 && estimatedInputTokens > maxTokens)
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
		Estimator:             "bytes_ratio",
		Profile:               profile,
	}
	return request
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
	if !boolValue(input["providerRetryFollowUp"]) {
		return 0
	}
	if strings.TrimSpace(stringValue(input["failureClass"])) != "provider_timeout" {
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

func applyAdaptiveShrinkLevel(input map[string]any, level int) map[string]any {
	switch {
	case level >= 4:
		return minimalProviderInput(input)
	case level == 3:
		return prioritizedProviderInput(input, true)
	case level == 2:
		return compactProviderExecutionContext(input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries)
	case level == 1:
		return budgetProviderInput(input, providerBudgetStringBytes, providerBudgetSliceItems, providerBudgetMapEntries, 4)
	default:
		return input
	}
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
	executionContext := recordFromAny(out["executionContext"])
	if len(executionContext) > 0 {
		compactSoulRouting(executionContext)
		compactNamedMapSlice(executionContext, "governanceHistory", itemLimit)
		compactNamedMapSlice(executionContext, "goals", itemLimit)
		compactNamedMapSlice(executionContext, "dods", itemLimit)
		out["executionContext"] = executionContext
	}
	compactSoulRouting(out)
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

func prioritizedProviderInput(input map[string]any, includeP2 bool) map[string]any {
	out := map[string]any{}
	for _, key := range providerP0InputKeys {
		if value, ok := input[key]; ok {
			out[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, providerBudgetAggressiveItems, providerBudgetAggressiveEntries, 2)
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
	stage := strings.TrimSpace(strings.ToLower(stringValue(input["currentSoulStage"])))
	if executionContext := recordFromAny(input["executionContext"]); len(executionContext) > 0 {
		compact := map[string]any{}
		for _, key := range stageAwareExecutionContextKeys(stage, includeP2) {
			if value, ok := executionContext[key]; ok {
				compact[key] = budgetProviderValue(value, 0, providerBudgetAggressiveBytes, providerBudgetAggressiveItems, providerBudgetAggressiveEntries, 2)
			}
		}
		compactNamedMapSlice(compact, "governanceHistory", providerBudgetAggressiveItems)
		compactNamedMapSlice(compact, "goals", providerBudgetAggressiveItems)
		compactNamedMapSlice(compact, "dods", providerBudgetAggressiveItems)
		out["executionContext"] = compact
	}
	compactSoulRouting(out)
	return out
}

func stageAwareExecutionContextKeys(stage string, includeP2 bool) []string {
	keys := []string{"goal", "dod", "dialogLanguage", "goalDefinitionPolicy", "doDDefinitionPolicy"}
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
		keys = append(keys, "graphEvidence", "failedExecutionGraph")
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
