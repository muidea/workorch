package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: llm_context_budget_trace.go

func llmContextBudgetTraceFromStep(step projectioncontract.StepView, budget workorcheventpayload.Payload, usage workorcheventpayload.Payload) projectioncontract.LLMContextBudgetTrace {
	return projectioncontract.LLMContextBudgetTrace{
		StepID:                step.ID,
		ProviderID:            llmContextBudgetProviderID(budget, usage),
		Model:                 llmContextBudgetModel(budget, usage),
		CapabilityRevision:    llmContextBudgetCapabilityRevision(budget),
		ConfigurationRevision: llmContextBudgetConfigurationRevision(budget),
		MaxBytes:              llmContextBudgetMaxBytes(budget),
		ActualBytes:           llmContextBudgetActualBytes(budget),
		MaxTokens:             llmContextBudgetMaxTokens(budget),
		EstimatedInputTokens:  llmContextBudgetEstimatedInputTokens(budget),
		EstimatedOutputTokens: llmContextBudgetEstimatedOutputTokens(budget),
		ActualTokens:          llmContextBudgetActualTokens(budget),
		ShrinkLevel:           llmContextBudgetShrinkLevel(budget),
		Truncated:             llmContextBudgetTruncated(budget),
		Rules:                 llmContextBudgetRules(budget),
		SourceRefs:            llmContextBudgetSourceRefs(budget),
		IncludedSections:      llmContextBudgetIncludedSections(budget),
		NormalizedSections:    llmContextBudgetNormalizedSections(budget),
		CompressedSections:    llmContextBudgetCompressedSections(budget),
		OmittedSections:       llmContextBudgetOmittedSections(budget),
		GoalDoDCompressed:     llmContextBudgetGoalDoDCompressed(budget),
		OverBudget:            llmContextBudgetOverBudget(budget),
		P0Overflow:            llmContextBudgetP0Overflow(budget),
		ProfileMode:           llmContextBudgetProfileMode(budget),
		CapabilitySource:      llmContextBudgetCapabilitySource(budget),
		ContextUtilization:    llmContextBudgetContextUtilization(budget),
		MaxRequestBytes:       llmContextBudgetMaxRequestBytes(budget),
		StageBudgetRatio:      llmContextBudgetStageBudgetRatio(budget),
		StageBudgetSource:     llmContextBudgetStageBudgetSource(budget),
		TracePolicy:           llmContextBudgetTracePolicy(budget),
		PromptCaptured:        llmContextBudgetPromptCaptured(budget),
		Estimator:             llmContextBudgetEstimator(budget),
		ProviderSubmitted:     llmContextBudgetProviderSubmitted(budget),
		ContextGateAction:     llmContextBudgetContextGateAction(budget),
		ContextGateReason:     llmContextBudgetContextGateReason(budget),
		BudgetFailureClass:    llmContextBudgetBudgetFailureClass(budget),
		RedactedPaths:         llmContextBudgetRedactedPaths(budget),
		ProtectedFields:       llmContextBudgetProtectedFields(budget),
		IntegrityWarnings:     llmContextBudgetIntegrityWarnings(budget),
		SectionHashes:         llmContextBudgetSectionHashes(budget),
		Profile:               llmContextBudgetProfile(budget),
		CachePlan:             llmContextBudgetCachePlan(budget),
	}
}

// source: llm_usage_cache_metrics.go

type llmUsageCacheMetrics struct {
	cacheReadKnown           bool
	cacheWriteKnown          bool
	cacheReadTokens          int
	cacheWriteTokens         int
	cacheCreationTokens      int
	cacheableInputTokens     int
	nonCacheableInputTokens  int
	cacheEstimatedEntry      bool
	cacheProviderNativeEntry bool
}

func llmUsagePayloadInt(payload workorcheventpayload.Payload, key string) int {
	value, _ := payload.Int64(key)
	return int(value)
}

func llmUsagePayloadBool(payload workorcheventpayload.Payload, key string) bool {
	value, _ := payload.Bool(key)
	return value
}

func llmUsageCacheMetricsFromTrace(item projectioncontract.LLMUsageTrace) llmUsageCacheMetrics {
	ret := llmUsageCacheMetrics{
		cacheReadKnown:          llmUsageCacheMeasurementKnown(item.CacheUsage, "cacheReadTokens"),
		cacheWriteKnown:         llmUsageCacheMeasurementKnown(item.CacheUsage, "cacheWriteTokens"),
		cacheReadTokens:         llmUsagePayloadInt(item.CacheUsage, "cacheReadTokens"),
		cacheWriteTokens:        llmUsagePayloadInt(item.CacheUsage, "cacheWriteTokens"),
		cacheCreationTokens:     llmUsagePayloadInt(item.CacheUsage, "cacheCreationTokens"),
		cacheableInputTokens:    llmUsagePayloadInt(item.CacheUsage, "cacheableInputTokens"),
		nonCacheableInputTokens: llmUsagePayloadInt(item.CacheUsage, "nonCacheableInputTokens"),
	}
	if len(item.CacheUsage.Fields) == 0 {
		return ret
	}
	ret.cacheEstimatedEntry = llmUsageCacheEstimatedEntry(item)
	ret.cacheProviderNativeEntry = llmUsageCacheProviderNativeEntry(item)
	return ret
}

func addLLMUsageCacheMetrics(summary *projectioncontract.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	addLLMUsageCacheTokenMetrics(summary, metrics)
	addLLMUsageCacheEntryMetrics(summary, metrics)
}

// source: repair_escalation_apply.go

func applyRepairEscalationProjection(ret *projectioncontract.RepairLoop, ticketID string, reason string, escalationContext workorcheventpayload.Payload) {
	if ret == nil {
		return
	}
	ret.Escalated = true
	ret.EscalationTicketID = ticketID
	ret.EscalationReason = reason
	ret.EvidenceRefs = append(ret.EvidenceRefs, ticketID)
	ret.State = "escalated_decision"
	if items := payloadObjectMapSlice(escalationContext, "missingDoDItems"); len(items) > 0 {
		ret.MissingDoDItems = items
	}
	if attempts := payloadObjectMapSlice(escalationContext, "recentRepairAttempts"); len(attempts) > 0 {
		ret.RecentAttempts = attempts
	}
}

func payloadObjectMapSlice(payload workorcheventpayload.Payload, name string) []map[string]any {
	value, ok := payload.Field(name)
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]map[string]any, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != workorcheventpayload.ValueObject {
			continue
		}
		mapped := workorcheventpayload.New(item.Object...).Map()
		if len(mapped) > 0 {
			ret = append(ret, mapped)
		}
	}
	return ret
}

// Historical positive measurements remain observable; a historical zero without
// presence is ambiguous and must not become known merely by projection.
func llmUsageCacheMeasurementKnown(payload workorcheventpayload.Payload, key string) bool {
	if known, exists := payload.Bool(key + "Known"); exists {
		return known
	}
	value, exists := payload.Int64(key)
	return exists && value > 0
}
