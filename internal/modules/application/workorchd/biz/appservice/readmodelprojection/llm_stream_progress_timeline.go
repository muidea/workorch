package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/pkg/events"
)

func streamProgressTimelineItemFromPersistedEvent(item readmodelcore.EventRecord, runState string) (readmodelcore.RuntimeTimelineItem, bool) {
	if item.Topic != events.TopicRunLLMStreamProgress {
		return readmodelcore.RuntimeTimelineItem{}, false
	}
	payload := cloneReadModelMap(item.Payload)
	return streamProgressTimelineItemFromPayload(item.ID, item.Topic, item.OccurredAt, item.StepID, payload, runState), true
}

func streamProgressTimelineItemFromObserved(item presenter.ObservedView, runState string) (readmodelcore.RuntimeTimelineItem, bool) {
	if item.Topic != events.TopicRunLLMStreamProgress {
		return readmodelcore.RuntimeTimelineItem{}, false
	}
	payload := cloneReadModelMap(item.Payload)
	return streamProgressTimelineItemFromPayload(item.SubjectID, item.Topic, item.OccurredAt, item.StepID, payload, runState), true
}
