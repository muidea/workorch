package ownercommandports

import (
	"context"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func PublishMemoryItemDelete(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, runID common.RunID, memoryID common.MemoryID) error {
	_ = runID
	if memoryID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendMagicCommand[memoryevents.DeleteMemoryCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, memoryevents.TopicMemoryCommandDelete, memoryevents.CommandDestination, "memory", string(memoryID), memoryevents.DeleteMemoryCommand{MemoryID: memoryID})
	return err
}

func PublishMemoryRecallTraceUpsert(ctx context.Context, hub events.Hub, trace *memorymodel.MemoryRecallTrace) error {
	if trace == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendMagicCommand[memoryevents.UpsertMemoryRecallTraceCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, trace.WorkspaceID), hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, memoryevents.CommandDestination, "memory_recall", string(trace.ID), memoryevents.UpsertMemoryRecallTraceCommand(memoryRecallTraceCommandFromModel(trace)))
	return err
}

func RecordMemoryRecallTrace(ctx context.Context, hub events.Hub, trace *memorymodel.MemoryRecallTrace) error {
	if hub == nil || trace == nil {
		return common.ErrInvalidArgument
	}
	if err := PublishMemoryRecallTraceUpsert(ctx, hub, trace); err != nil {
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
