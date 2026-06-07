package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func llmUsageTraceFromEventRecord(item readmodelcore.EventRecord) (readmodelcore.LLMUsageTrace, bool) {
	payload := cloneReadModelMap(item.Payload)
	if len(payload) == 0 {
		return readmodelcore.LLMUsageTrace{}, false
	}
	estimated := llmUsageTraceEstimated(payload)
	usageSource := llmUsageTraceUsageSource(payload, estimated)
	return readmodelcore.LLMUsageTrace{
		ID:            item.ID,
		WorkspaceID:   llmUsageTraceWorkspaceID(item, payload),
		RunID:         llmUsageTraceRunID(item, payload),
		StepID:        llmUsageTraceStepID(item, payload),
		ProviderID:    llmUsageTraceProviderID(payload),
		Provider:      llmUsageTraceProvider(payload),
		Model:         llmUsageTraceModel(payload),
		Operation:     llmUsageTraceOperation(payload),
		InputTokens:   llmUsageTraceInputTokens(payload),
		OutputTokens:  llmUsageTraceOutputTokens(payload),
		TotalTokens:   llmUsageTraceTotalTokens(payload),
		Estimated:     estimated,
		UsageSource:   usageSource,
		RequestBytes:  llmUsageTraceRequestBytes(payload),
		ResponseBytes: llmUsageTraceResponseBytes(payload),
		Streaming:     llmUsageTraceStreaming(payload),
		ContextBudget: llmUsageTraceContextBudget(payload),
		CacheUsage:    llmUsageTraceCacheUsage(payload),
		StartedAt:     llmUsageTraceStartedAt(payload),
		CompletedAt:   llmUsageTraceCompletedAt(payload),
		DurationMs:    llmUsageTraceDurationMs(payload),
		Status:        llmUsageTraceStatus(payload),
		FailureClass:  llmUsageTraceFailureClass(payload),
		OccurredAt:    item.OccurredAt,
		EvidenceRef:   llmUsageTraceEvidenceRef(item),
	}, true
}
