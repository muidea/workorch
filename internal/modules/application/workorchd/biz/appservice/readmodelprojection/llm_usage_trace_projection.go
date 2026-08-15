package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: llm_usage_trace_labels.go

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

// source: llm_usage_trace_lifecycle.go

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

// source: llm_usage_trace_metrics.go

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

// source: llm_usage_trace_runtime.go

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

// source: llm_usage_trace_source_values.go

func llmUsageTraceEstimated(payload workorcheventpayload.Payload) bool {
	return eventPayloadBool(payload, recordpayload.PayloadKeyEstimated)
}

func llmUsageTraceUsageSource(payload workorcheventpayload.Payload, estimated bool) string {
	return llmUsageTraceSource(eventPayloadString(payload, recordpayload.PayloadKeyUsageSource), estimated)
}
