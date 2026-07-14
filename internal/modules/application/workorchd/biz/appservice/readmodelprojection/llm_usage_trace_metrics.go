package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceInputTokens(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, events.PayloadKeyInputTokens)
}

func llmUsageTraceOutputTokens(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, events.PayloadKeyOutputTokens)
}

func llmUsageTraceTotalTokens(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, events.PayloadKeyTotalTokens)
}

func llmUsageTraceRequestBytes(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, events.PayloadKeyRequestBytes)
}

func llmUsageTraceResponseBytes(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, events.PayloadKeyResponseBytes)
}

func llmUsageTraceDurationMs(payload workorcheventpayload.Payload) int {
	return eventPayloadInt(payload, events.PayloadKeyDurationMs)
}
