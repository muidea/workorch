package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

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
