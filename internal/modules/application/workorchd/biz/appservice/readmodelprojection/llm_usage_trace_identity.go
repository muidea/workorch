package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func llmUsageTraceRequestIdentity(payload workorcheventpayload.Payload) (string, string, bool) {
	logicalRequestID, logicalPresent := payload.String(recordpayload.PayloadKeyLogicalRequestID)
	physicalCallID, physicalPresent := payload.String(recordpayload.PayloadKeyPhysicalCallID)
	if !logicalPresent && !physicalPresent {
		// Persistence-only compatibility for generic usage and event records
		// written before the two-level request identity was introduced.
		return "", "", true
	}
	if !logicalPresent || !physicalPresent || logicalRequestID == "" || physicalCallID == "" ||
		logicalRequestID != strings.TrimSpace(logicalRequestID) || physicalCallID != strings.TrimSpace(physicalCallID) {
		return "", "", false
	}
	requestID, requestPresent := payload.String(recordpayload.PayloadKeyRequestID)
	if !requestPresent || requestID != physicalCallID {
		return "", "", false
	}
	return logicalRequestID, physicalCallID, true
}

func llmUsageTraceWorkspaceID(item projectioncontract.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.WorkspaceID, eventPayloadString(payload, recordpayload.PayloadKeyWorkspaceID))
}

func llmUsageTraceRunID(item projectioncontract.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.RunID, eventPayloadString(payload, recordpayload.PayloadKeyRunID))
}

func llmUsageTraceStepID(item projectioncontract.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.StepID, eventPayloadString(payload, recordpayload.PayloadKeyStepID))
}

func llmUsageTraceEvidenceRef(item projectioncontract.EventRecord) string {
	return firstNonEmptyTimelineString(item.StepID, item.SubjectID)
}
