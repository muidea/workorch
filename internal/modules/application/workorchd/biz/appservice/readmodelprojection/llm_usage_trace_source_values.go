package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

func llmUsageTraceEstimated(payload map[string]any) bool {
	return readModelBoolValue(payload[events.PayloadKeyEstimated])
}

func llmUsageTraceUsageSource(payload map[string]any, estimated bool) string {
	return llmUsageTraceSource(readModelStringValue(payload[events.PayloadKeyUsageSource]), estimated)
}
