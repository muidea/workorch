package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func streamProgressTimelineItemFromPersistedEvent(item projectioncontract.EventRecord, runState string) (readmodelcore.RuntimeTimelineItem, bool) {
	if item.Topic != "/run/llm_stream/progress" {
		return readmodelcore.RuntimeTimelineItem{}, false
	}
	projectedStepID, _ := item.Payload.String(recordpayload.PayloadKeyStepID)
	projectedStepID = firstNonEmptyTimelineString(item.StepID, projectedStepID)
	itemType, _ := item.Payload.String(recordpayload.PayloadKeyStatus)
	itemType = firstNonEmptyTimelineString(itemType, "progress")
	summary, _ := item.Payload.String(recordpayload.PayloadKeySummary)
	if summary == "" {
		summary = streamProgressSummaryFromEventPayload(item.Payload)
	}
	displayState, _ := item.Payload.String(recordpayload.PayloadKeyStatus)
	displayState = firstNonEmptyTimelineString(displayState, runState)
	switch displayState {
	case "completed":
		displayState = string(common.StepStateSucceeded)
	case "dispatched":
		displayState = string(common.StepStateExecuting)
	default:
		displayState = string(common.CanonicalStepState(common.StepState(displayState)))
	}
	return readmodelcore.RuntimeTimelineItem{
		Source:     "llm_stream_progress_event",
		Type:       itemType,
		ID:         firstNonEmptyTimelineString(item.ID, projectedStepID+"/llm-stream-progress"),
		Topic:      item.Topic,
		RunState:   displayState,
		StepID:     projectedStepID,
		StepState:  itemType,
		Summary:    summary,
		OccurredAt: item.OccurredAt,
		Data:       item.Payload.Map(),
	}, true
}

func streamProgressTimelineItemFromObserved(item readmodelcore.ObservedView, runState string) (readmodelcore.RuntimeTimelineItem, bool) {
	if item.Topic != "/run/llm_stream/progress" {
		return readmodelcore.RuntimeTimelineItem{}, false
	}
	payload := cloneReadModelMap(item.Payload)
	return streamProgressTimelineItemFromPayload(item.SubjectID, item.Topic, item.OccurredAt, item.StepID, payload, runState), true
}
