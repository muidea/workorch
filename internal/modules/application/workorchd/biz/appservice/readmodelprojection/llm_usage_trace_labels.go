package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceProviderID(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyProviderID)
}

func llmUsageTraceProvider(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyProvider)
}

func llmUsageTraceAPISurface(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyAPISurface)
}

func llmUsageTraceModel(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyModel)
}

func llmUsageTraceOperation(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyOperation)
}
