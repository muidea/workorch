package memory

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

func (m *Biz) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(topic, ModuleID, memoryevents.NotificationDestination, magicevent.NewHeader(), ctx, memoryevents.MemoryChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), topic, eventcontract.LaneScope{
		SubjectType: "memory", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID),
	}))
	m.PostEvent(event)
}

func (m *Biz) publishMemoryRecallRecorded(ctx context.Context, trace *memorymodel.MemoryRecallTrace) {
	if m == nil || m.EventHub() == nil || trace == nil {
		return
	}
	event := magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallRecorded,
		ModuleID,
		memoryevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		memoryevents.MemoryRecallRecordedData{
			TraceID:            trace.ID,
			WorkspaceID:        trace.WorkspaceID,
			RunID:              trace.RunID,
			StepID:             trace.StepID,
			Stage:              trace.Stage,
			Source:             trace.Source,
			Policy:             trace.Policy,
			Status:             trace.Status,
			ProviderID:         trace.ProviderID,
			Operation:          trace.Operation,
			RetrievedMemoryIDs: append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
			Reason:             trace.Reason,
			OccurredAt:         trace.OccurredAt,
		},
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "memory_recall",
		SubjectID:   string(trace.ID),
		WorkspaceID: string(trace.WorkspaceID),
		RunID:       string(trace.RunID),
		StepID:      string(trace.StepID),
	}))
	m.PostEvent(event)
}
