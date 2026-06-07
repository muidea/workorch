package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func llmUsageTraceWorkspaceID(item readmodelcore.EventRecord, payload map[string]any) string {
	return firstNonEmptyTimelineString(item.WorkspaceID, readModelStringValue(payload[events.PayloadKeyWorkspaceID]))
}

func llmUsageTraceRunID(item readmodelcore.EventRecord, payload map[string]any) string {
	return firstNonEmptyTimelineString(item.RunID, readModelStringValue(payload[events.PayloadKeyRunID]))
}

func llmUsageTraceStepID(item readmodelcore.EventRecord, payload map[string]any) string {
	return firstNonEmptyTimelineString(item.StepID, readModelStringValue(payload[events.PayloadKeyStepID]))
}

func llmUsageTraceEvidenceRef(item readmodelcore.EventRecord) string {
	return firstNonEmptyTimelineString(item.StepID, item.SubjectID)
}
