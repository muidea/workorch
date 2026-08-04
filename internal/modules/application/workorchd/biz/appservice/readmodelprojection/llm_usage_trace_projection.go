package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func llmUsageTraceFromEventRecord(item projectioncontract.EventRecord) (projectioncontract.LLMUsageTrace, bool) {
	if len(item.Payload.Fields) == 0 {
		return projectioncontract.LLMUsageTrace{}, false
	}
	estimated := llmUsageTraceEstimated(item.Payload)
	usageSource := llmUsageTraceUsageSource(item.Payload, estimated)
	return projectioncontract.LLMUsageTrace{
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
