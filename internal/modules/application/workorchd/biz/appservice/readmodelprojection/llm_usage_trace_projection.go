package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func llmUsageTraceFromEventRecord(item readmodelcore.EventRecord) (readmodelcore.LLMUsageTrace, bool) {
	if len(item.Payload.Fields) == 0 {
		return readmodelcore.LLMUsageTrace{}, false
	}
	estimated := llmUsageTraceEstimated(item.Payload)
	usageSource := llmUsageTraceUsageSource(item.Payload, estimated)
	return readmodelcore.LLMUsageTrace{
		ID:            item.ID,
		WorkspaceID:   llmUsageTraceWorkspaceID(item, item.Payload),
		RunID:         llmUsageTraceRunID(item, item.Payload),
		StepID:        llmUsageTraceStepID(item, item.Payload),
		ProviderID:    llmUsageTraceProviderID(item.Payload),
		Provider:      llmUsageTraceProvider(item.Payload),
		APISurface:    llmUsageTraceAPISurface(item.Payload),
		Model:         llmUsageTraceModel(item.Payload),
		Operation:     llmUsageTraceOperation(item.Payload),
		InputTokens:   llmUsageTraceInputTokens(item.Payload),
		OutputTokens:  llmUsageTraceOutputTokens(item.Payload),
		TotalTokens:   llmUsageTraceTotalTokens(item.Payload),
		Estimated:     estimated,
		UsageSource:   usageSource,
		RequestBytes:  llmUsageTraceRequestBytes(item.Payload),
		ResponseBytes: llmUsageTraceResponseBytes(item.Payload),
		Streaming:     llmUsageTraceStreaming(item.Payload),
		ContextBudget: llmUsageTraceContextBudget(item.Payload),
		CacheUsage:    llmUsageTraceCacheUsage(item.Payload),
		CacheMarkers:  llmUsageTraceCacheMarkers(item.Payload),
		StartedAt:     llmUsageTraceStartedAt(item.Payload),
		CompletedAt:   llmUsageTraceCompletedAt(item.Payload),
		DurationMs:    llmUsageTraceDurationMs(item.Payload),
		Status:        llmUsageTraceStatus(item.Payload),
		FailureClass:  llmUsageTraceFailureClass(item.Payload),
		OccurredAt:    item.OccurredAt,
		EvidenceRef:   llmUsageTraceEvidenceRef(item),
	}, true
}
