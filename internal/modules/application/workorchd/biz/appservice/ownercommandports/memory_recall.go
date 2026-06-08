package ownercommandports

import (
	"context"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func PublishMemoryRecallTraceUpsert(ctx context.Context, hub events.Hub, trace *memorymodel.MemoryRecallTrace) error {
	if trace == nil {
		return common.ErrInvalidArgument
	}
	return Publish(ctx, hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, "memory_recall", string(trace.ID), trace.WorkspaceID, trace.RunID, map[string]any{
		events.PayloadKeyCommand: memoryRecallTraceCommandFromModel(trace),
	})
}

func RecordMemoryRecallTrace(ctx context.Context, hub events.Hub, trace *memorymodel.MemoryRecallTrace) error {
	if hub == nil || trace == nil {
		return common.ErrInvalidArgument
	}
	if err := PublishMemoryRecallTraceUpsert(ctx, hub, trace); err != nil {
		return err
	}
	return hub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        memoryevents.TopicMemoryRecallRecorded,
		OccurredAt:   trace.OccurredAt,
		SourceModule: modulecommon.EventSourceModuleID,
		SubjectType:  "memory_recall",
		SubjectID:    string(trace.ID),
		WorkspaceID:  string(trace.WorkspaceID),
		RunID:        string(trace.RunID),
		Payload: events.MemoryRecallPayload(
			trace.ID,
			trace.Stage,
			trace.Source,
			trace.Policy,
			trace.Status,
			trace.ProviderID,
			trace.Operation,
			trace.RetrievedMemoryIDs,
			trace.Reason,
		),
	})
}
