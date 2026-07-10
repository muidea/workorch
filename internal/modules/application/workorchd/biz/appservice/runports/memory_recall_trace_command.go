package runports

import (
	"context"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func RecordMemoryRecallTrace(ctx context.Context, hub events.Hub, trace *memorymodel.MemoryRecallTrace) error {
	if hub == nil || trace == nil {
		return common.ErrInvalidArgument
	}
	if _, err := sendRunMagicCommand[memoryevents.UpsertMemoryRecallTraceCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, trace.WorkspaceID), hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, memoryevents.CommandDestination, "memory_recall", string(trace.ID), trace.WorkspaceID, trace.RunID, memoryevents.UpsertMemoryRecallTraceCommandFromModel(trace)); err != nil {
		return err
	}
	return events.PublishByContract(ctx, hub, events.Envelope{
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
