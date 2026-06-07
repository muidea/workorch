package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

func llmUsageTraceInputTokens(payload map[string]any) int {
	return readModelIntValue(payload[events.PayloadKeyInputTokens])
}

func llmUsageTraceOutputTokens(payload map[string]any) int {
	return readModelIntValue(payload[events.PayloadKeyOutputTokens])
}

func llmUsageTraceTotalTokens(payload map[string]any) int {
	return readModelIntValue(payload[events.PayloadKeyTotalTokens])
}

func llmUsageTraceRequestBytes(payload map[string]any) int {
	return readModelIntValue(payload[events.PayloadKeyRequestBytes])
}

func llmUsageTraceResponseBytes(payload map[string]any) int {
	return readModelIntValue(payload[events.PayloadKeyResponseBytes])
}

func llmUsageTraceDurationMs(payload map[string]any) int {
	return readModelIntValue(payload[events.PayloadKeyDurationMs])
}
