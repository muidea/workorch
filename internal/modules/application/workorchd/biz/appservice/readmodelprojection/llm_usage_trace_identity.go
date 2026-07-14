package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func llmUsageTraceWorkspaceID(item readmodelcore.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.WorkspaceID, eventPayloadString(payload, events.PayloadKeyWorkspaceID))
}

func llmUsageTraceRunID(item readmodelcore.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.RunID, eventPayloadString(payload, events.PayloadKeyRunID))
}

func llmUsageTraceStepID(item readmodelcore.EventRecord, payload workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(item.StepID, eventPayloadString(payload, events.PayloadKeyStepID))
}

func llmUsageTraceEvidenceRef(item readmodelcore.EventRecord) string {
	return firstNonEmptyTimelineString(item.StepID, item.SubjectID)
}
