package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

func llmUsageTraceStartedAt(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyStartedAt])
}

func llmUsageTraceCompletedAt(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyCompletedAt])
}

func llmUsageTraceStatus(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyStatus])
}

func llmUsageTraceFailureClass(payload map[string]any) string {
	return readModelStringValue(payload[events.PayloadKeyFailureClass])
}
