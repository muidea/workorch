package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceStreaming(payload workorcheventpayload.Payload) bool {
	return eventPayloadBool(payload, events.PayloadKeyStreaming)
}

func llmUsageTraceContextBudget(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := payload.Object(events.PayloadKeyContextBudget)
	return value
}

func llmUsageTraceCacheUsage(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := payload.Object(events.PayloadKeyCacheUsage)
	return value
}

func llmUsageTraceCacheMarkers(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := payload.Object(events.PayloadKeyCacheMarkersSent)
	return value
}
