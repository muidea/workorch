package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func persistedEventTimelineItem(item projectioncontract.EventRecord) readmodelcore.RuntimeTimelineItem {
	runState, _ := item.Payload.String(recordpayload.PayloadKeyState)
	return readmodelcore.RuntimeTimelineItem{
		Source:     "persisted_event",
		Type:       item.SubjectType,
		ID:         item.ID,
		Topic:      item.Topic,
		RunState:   runState,
		StepID:     item.StepID,
		OccurredAt: item.OccurredAt,
		Data:       item.Payload.Map(),
	}
}

func observedEventTimelineItem(item readmodelcore.ObservedView) readmodelcore.RuntimeTimelineItem {
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
