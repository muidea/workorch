package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceInputTokens(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, recordpayload.PayloadKeyInputTokens)
}

func llmUsageTraceOutputTokens(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, recordpayload.PayloadKeyOutputTokens)
}

func llmUsageTraceTotalTokens(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, recordpayload.PayloadKeyTotalTokens)
}

func llmUsageTraceRequestBytes(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, recordpayload.PayloadKeyRequestBytes)
}

func llmUsageTraceResponseBytes(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, recordpayload.PayloadKeyResponseBytes)
}

func llmUsageTraceDurationMs(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, recordpayload.PayloadKeyDurationMs)
}
