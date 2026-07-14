package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceStreaming(payload workorcheventpayload.Payload) bool {
	return eventPayloadBool(payload, events.PayloadKeyStreaming)
}

func llmUsageTraceContextBudget(payload workorcheventpayload.Payload) map[string]any {
	return eventPayloadMap(payload, events.PayloadKeyContextBudget)
}

func llmUsageTraceCacheUsage(payload workorcheventpayload.Payload) map[string]any {
	return eventPayloadMap(payload, events.PayloadKeyCacheUsage)
}

func llmUsageTraceCacheMarkers(payload workorcheventpayload.Payload) map[string]any {
	return eventPayloadMap(payload, events.PayloadKeyCacheMarkersSent)
}
