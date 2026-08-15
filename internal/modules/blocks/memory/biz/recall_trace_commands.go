package memory

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeMemoryRecallTraceCommand(ctx context.Context, ev magicevent.Event) (memoryevents.MemoryCommandResult, error) {
	switch ev.ID() {
	case memoryevents.TopicMemoryRecallTraceCommandUpsert:
		cmd, ok := ev.Data().(memoryevents.UpsertMemoryRecallTraceCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		traceValue := memoryevents.MemoryRecallTrace(cmd)
		trace, err := (&traceValue).Model()
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		if err := m.service.UpsertMemoryRecallTrace(ctx, trace); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		m.publishMemoryRecallRecorded(ctx, trace)
		return memoryevents.MemoryCommandResult{}, nil
	case memoryevents.TopicMemoryRecallTraceCommandGet:
		cmd, ok := ev.Data().(memoryevents.GetMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		item, err := m.service.GetMemoryRecallTrace(ctx, cmd.TraceID)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{RecallTrace: memoryevents.MemoryRecallTraceFromModel(item)}, nil
	case memoryevents.TopicMemoryRecallTraceCommandList:
		cmd, ok := ev.Data().(memoryevents.ListMemoryRecallTracesCommand)
		if !ok {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.Query.WorkspaceID)
		items, err := m.service.ListMemoryRecallTraces(ctx, cmd.Query)
		if err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{RecallTraces: memoryevents.MemoryRecallTracesFromModels(items)}, nil
	case memoryevents.TopicMemoryRecallTraceCommandDelete:
		cmd, ok := ev.Data().(memoryevents.DeleteMemoryRecallTraceCommand)
		if !ok || cmd.TraceID == "" {
			return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
		}
		ctx = memoryCommandContext(ctx, cmd.WorkspaceID)
		if err := m.service.DeleteMemoryRecallTrace(ctx, cmd.TraceID); err != nil {
			return memoryevents.MemoryCommandResult{}, err
		}
		return memoryevents.MemoryCommandResult{}, nil
	default:
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
}
