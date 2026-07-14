package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceEstimated(payload workorcheventpayload.Payload) bool {
	return eventPayloadBool(payload, events.PayloadKeyEstimated)
}

func llmUsageTraceUsageSource(payload workorcheventpayload.Payload, estimated bool) string {
	return llmUsageTraceSource(eventPayloadString(payload, events.PayloadKeyUsageSource), estimated)
}
