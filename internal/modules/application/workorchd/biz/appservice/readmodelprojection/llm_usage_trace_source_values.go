package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceEstimated(payload workorcheventpayload.Payload) bool {
	return eventPayloadBool(payload, recordpayload.PayloadKeyEstimated)
}

func llmUsageTraceUsageSource(payload workorcheventpayload.Payload, estimated bool) string {
	return llmUsageTraceSource(eventPayloadString(payload, recordpayload.PayloadKeyUsageSource), estimated)
}
