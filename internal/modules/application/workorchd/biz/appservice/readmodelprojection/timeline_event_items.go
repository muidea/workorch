package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/pkg/events"
)

func persistedEventTimelineItem(item readmodelcore.EventRecord) readmodelcore.RuntimeTimelineItem {
	return readmodelcore.RuntimeTimelineItem{
		Source:     "persisted_event",
		Type:       item.SubjectType,
		ID:         item.ID,
		Topic:      item.Topic,
		RunState:   readModelStringValue(item.Payload[events.PayloadKeyState]),
		StepID:     item.StepID,
		OccurredAt: item.OccurredAt,
		Data:       cloneReadModelMap(item.Payload),
	}
}

func observedEventTimelineItem(item presenter.ObservedView) readmodelcore.RuntimeTimelineItem {
	return readmodelcore.RuntimeTimelineItem{
		Source:     "event",
		Type:       item.SubjectType,
		ID:         item.SubjectID,
		Topic:      item.Topic,
		StepID:     item.StepID,
		OccurredAt: item.OccurredAt,
		Data:       cloneReadModelMap(item.Payload),
	}
}
