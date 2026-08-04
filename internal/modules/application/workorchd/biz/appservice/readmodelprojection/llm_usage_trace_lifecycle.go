package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceStartedAt(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyStartedAt)
}

func llmUsageTraceCompletedAt(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyCompletedAt)
}

func llmUsageTraceStatus(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyStatus)
}

func llmUsageTraceFailureClass(payload workorcheventpayload.Payload) string {
	return eventPayloadString(payload, recordpayload.PayloadKeyFailureClass)
}
