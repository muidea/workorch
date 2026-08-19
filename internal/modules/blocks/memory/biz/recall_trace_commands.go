package memory

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeMemoryRecallTraceCommand(ctx context.Context, ev magicevent.Event, result magicevent.Result) error {
	switch ev.ID() {
	case memoryevents.TopicMemoryRecallTraceCommandUpsert:
		cmd, ok := ev.Data().(memoryevents.UpsertMemoryRecallTraceCommand)
		if !ok {
			setMemoryMagicResult(result, memoryevents.UpsertMemoryRecallTraceResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		traceValue := memoryevents.MemoryRecallTrace(cmd)
		trace, err := (&traceValue).Model()
		if err != nil {
			setMemoryMagicResult(result, memoryevents.UpsertMemoryRecallTraceResult{}, err)
			return err
		}
		disposition, err := m.service.RecordMemoryRecallTrace(ctx, trace)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.UpsertMemoryRecallTraceResult{}, err)
			return err
		}
		if disposition == memoryevents.MemoryRecallTraceRecorded {
			m.publishMemoryRecallRecorded(ctx, trace)
		}
		setMemoryMagicResult(result, memoryevents.UpsertMemoryRecallTraceResult{
			WorkspaceID: trace.WorkspaceID, RunID: trace.RunID, TraceID: trace.ID, Disposition: disposition,
		}, nil)
		return nil
	case memoryevents.TopicMemoryRecallTraceCommandGet:
		cmd, ok := ev.Data().(memoryevents.GetMemoryRecallTraceCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.TraceID == "" {
			setMemoryMagicResult(result, memoryevents.GetMemoryRecallTraceResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		item, err := m.service.GetMemoryRecallTrace(ctx, cmd.WorkspaceID, cmd.TraceID)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.GetMemoryRecallTraceResult{}, err)
			return err
		}
		setMemoryMagicResult(result, (memoryevents.GetMemoryRecallTraceResult{Trace: memoryevents.MemoryRecallTraceFromModel(item)}).Clone(), nil)
		return nil
	case memoryevents.TopicMemoryRecallTraceCommandList:
		cmd, ok := ev.Data().(memoryevents.ListMemoryRecallTracesCommand)
		if !ok || cmd.Query.WorkspaceID == "" {
			setMemoryMagicResult(result, memoryevents.ListMemoryRecallTracesResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		items, nextCursor, err := m.service.ListMemoryRecallTraces(ctx, cmd.Query)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.ListMemoryRecallTracesResult{}, err)
			return err
		}
		setMemoryMagicResult(result, (memoryevents.ListMemoryRecallTracesResult{
			Traces: memoryevents.MemoryRecallTracesFromModels(items), NextCursor: nextCursor,
		}).Clone(), nil)
		return nil
	case memoryevents.TopicMemoryRecallTraceCommandDelete:
		cmd, ok := ev.Data().(memoryevents.DeleteMemoryRecallTraceCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.TraceID == "" {
			setMemoryMagicResult(result, memoryevents.DeleteMemoryRecallTraceResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		item, err := m.service.GetMemoryRecallTrace(ctx, cmd.WorkspaceID, cmd.TraceID)
		if err != nil {
			setMemoryMagicResult(result, memoryevents.DeleteMemoryRecallTraceResult{}, err)
			return err
		}
		if err := m.service.DeleteMemoryRecallTrace(ctx, cmd.TraceID); err != nil {
			setMemoryMagicResult(result, memoryevents.DeleteMemoryRecallTraceResult{}, err)
			return err
		}
		setMemoryMagicResult(result, memoryevents.DeleteMemoryRecallTraceResult{
			WorkspaceID: item.WorkspaceID, RunID: item.RunID, TraceID: item.ID,
		}, nil)
		return nil
	default:
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
}
