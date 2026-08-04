package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func persistedEventTimelineItem(item projectioncontract.EventRecord) projectioncontract.RuntimeTimelineItem {
	runState, _ := item.Payload.String(recordpayload.PayloadKeyState)
	return projectioncontract.RuntimeTimelineItem{
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

func observedEventTimelineItem(item projectioncontract.ObservedView) projectioncontract.RuntimeTimelineItem {
	return projectioncontract.RuntimeTimelineItem{
		Source:     "event",
		Type:       item.SubjectType,
		ID:         item.SubjectID,
		Topic:      item.Topic,
		StepID:     item.StepID,
		OccurredAt: item.OccurredAt,
		Data:       cloneReadModelMap(item.Payload),
	}
}
