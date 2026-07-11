package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func RecordMemoryRecallTrace(ctx context.Context, commandHub magicevent.Hub, trace *memorymodel.MemoryRecallTrace) error {
	if commandHub == nil || trace == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[memoryevents.UpsertMemoryRecallTraceCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, trace.WorkspaceID), commandHub, memoryevents.TopicMemoryRecallTraceCommandUpsert, memoryevents.CommandDestination, "memory_recall", string(trace.ID), trace.WorkspaceID, trace.RunID, memoryevents.UpsertMemoryRecallTraceCommandFromModel(trace))
	return err
}
