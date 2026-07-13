package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func streamProgressTimelineItemFromPersistedEvent(item readmodelcore.EventRecord, runState string) (readmodelcore.RuntimeTimelineItem, bool) {
	if item.Topic != "/run/llm_stream/progress" {
		return readmodelcore.RuntimeTimelineItem{}, false
	}
	payload := cloneReadModelMap(item.Payload)
	return streamProgressTimelineItemFromPayload(item.ID, item.Topic, item.OccurredAt, item.StepID, payload, runState), true
}

func streamProgressTimelineItemFromObserved(item ObservedView, runState string) (readmodelcore.RuntimeTimelineItem, bool) {
	if item.Topic != "/run/llm_stream/progress" {
		return readmodelcore.RuntimeTimelineItem{}, false
	}
	payload := cloneReadModelMap(item.Payload)
	return streamProgressTimelineItemFromPayload(item.SubjectID, item.Topic, item.OccurredAt, item.StepID, payload, runState), true
}
