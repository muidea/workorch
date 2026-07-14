package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceProviderID(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyProviderID)
}

func llmUsageTraceProvider(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyProvider)
}

func llmUsageTraceAPISurface(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyAPISurface)
}

func llmUsageTraceModel(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyModel)
}

func llmUsageTraceOperation(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, events.PayloadKeyOperation)
}
