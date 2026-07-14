package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceStartedAt(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyStartedAt)
}

func llmUsageTraceCompletedAt(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyCompletedAt)
}

func llmUsageTraceStatus(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyStatus)
}

func llmUsageTraceFailureClass(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyFailureClass)
}
