package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceStreaming(payload workorcheventpayload.Payload) bool {
	return eventPayloadBool(payload, recordpayload.PayloadKeyStreaming)
}

func llmUsageTraceContextBudget(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := payload.Object(recordpayload.PayloadKeyContextBudget)
	return sanitizeLLMContextBudgetAudit(value)
}

func llmUsageTraceCacheUsage(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := payload.Object(recordpayload.PayloadKeyCacheUsage)
	return value
}

func llmUsageTraceCacheMarkers(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := payload.Object(recordpayload.PayloadKeyCacheMarkersSent)
	return value
}
