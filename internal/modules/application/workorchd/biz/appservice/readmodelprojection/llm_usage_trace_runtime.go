package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

func llmUsageTraceStreaming(payload map[string]any) bool {
	return readModelBoolValue(payload[events.PayloadKeyStreaming])
}

func llmUsageTraceContextBudget(payload map[string]any) map[string]any {
	return readModelMapValue(payload[events.PayloadKeyContextBudget])
}

func llmUsageTraceCacheUsage(payload map[string]any) map[string]any {
	return readModelMapValue(payload[events.PayloadKeyCacheUsage])
}
