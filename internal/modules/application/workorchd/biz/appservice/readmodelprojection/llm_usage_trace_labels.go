package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

func llmUsageTraceProviderID(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyProviderID])
}

func llmUsageTraceProvider(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyProvider])
}

func llmUsageTraceModel(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyModel])
}

func llmUsageTraceOperation(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyOperation])
}
