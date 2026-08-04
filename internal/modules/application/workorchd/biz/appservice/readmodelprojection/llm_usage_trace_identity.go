package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceWorkspaceID(item readmodelcore.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.WorkspaceID, eventPayloadString(payload, recordpayload.PayloadKeyWorkspaceID))
}

func llmUsageTraceRunID(item readmodelcore.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.RunID, eventPayloadString(payload, recordpayload.PayloadKeyRunID))
}

func llmUsageTraceStepID(item readmodelcore.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.StepID, eventPayloadString(payload, recordpayload.PayloadKeyStepID))
}

func llmUsageTraceEvidenceRef(item readmodelcore.EventRecord) string {
	return firstNonEmptyTimelineString(item.StepID, item.SubjectID)
}
