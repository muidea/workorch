package orchestrator

import (
	"context"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestMemoryStateAdaptersPropagateWorkspaceScopeAndPageRecallQueries(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-memory-adapter")
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	const observerID = "/orchestrator/consumers/memory-state-adapter-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, memoryevents.CommandDestination, hub)
	t.Cleanup(func() {
		observer.Unsubscribe(memoryevents.TopicMemoryCommandGet)
		observer.Unsubscribe(memoryevents.TopicMemoryRecallTraceCommandGet)
		observer.Unsubscribe(memoryevents.TopicMemoryRecallTraceCommandList)
	})

	observer.Subscribe(memoryevents.TopicMemoryCommandGet, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(memoryevents.GetMemoryCommand)
		if !ok || command.WorkspaceID != "ws-memory-adapter" || command.MemoryID != "memory-1" {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
			return
		}
		result.Set(memoryevents.GetMemoryResult{Item: &memorymodel.MemoryItem{ID: command.MemoryID, WorkspaceID: command.WorkspaceID}}, nil)
	})
	observer.Subscribe(memoryevents.TopicMemoryRecallTraceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(memoryevents.GetMemoryRecallTraceCommand)
		if !ok || command.WorkspaceID != "ws-memory-adapter" || command.TraceID != "trace-1" {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
			return
		}
		result.Set(memoryevents.GetMemoryRecallTraceResult{Trace: memoryevents.MemoryRecallTraceFromModel(&memorymodel.MemoryRecallTrace{
			ID: command.TraceID, WorkspaceID: command.WorkspaceID, RunID: "run-1", Source: "recovery",
		})}, nil)
	})
	listCalls := 0
	observer.Subscribe(memoryevents.TopicMemoryRecallTraceCommandList, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(memoryevents.ListMemoryRecallTracesCommand)
		if !ok || command.Query.WorkspaceID != "ws-memory-adapter" || command.Query.RunID != "run-1" {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
			return
		}
		listCalls++
		switch command.Query.CursorAfterID {
		case "":
			result.Set(memoryevents.ListMemoryRecallTracesResult{
				Traces:     []*memoryevents.MemoryRecallTrace{memoryevents.MemoryRecallTraceFromModel(&memorymodel.MemoryRecallTrace{ID: "trace-1", WorkspaceID: "ws-memory-adapter", RunID: "run-1", Source: "recovery"})},
				NextCursor: "trace-1",
			}, nil)
		case "trace-1":
			result.Set(memoryevents.ListMemoryRecallTracesResult{
				Traces: []*memoryevents.MemoryRecallTrace{memoryevents.MemoryRecallTraceFromModel(&memorymodel.MemoryRecallTrace{ID: "trace-2", WorkspaceID: "ws-memory-adapter", RunID: "run-1", Source: "recovery"})},
			}, nil)
		default:
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrConflict.Error(), Cause: common.ErrConflict})
		}
	})

	item, err := (memoryEventRepository{hub: hub}).Get(ctx, "memory-1")
	if err != nil || item.WorkspaceID != "ws-memory-adapter" {
		t.Fatalf("memory adapter get = %+v, err=%v", item, err)
	}
	traceRepo := memoryRecallTraceEventRepository{hub: hub}
	trace, err := traceRepo.Get(ctx, "trace-1")
	if err != nil || trace.WorkspaceID != "ws-memory-adapter" {
		t.Fatalf("recall adapter get = %+v, err=%v", trace, err)
	}
	traces, err := traceRepo.List(ctx, workorchquerycontract.MemoryRecallTraceQuery{RunID: "run-1"})
	if err != nil {
		t.Fatalf("recall adapter list: %v", err)
	}
	if len(traces) != 2 || traces[0].ID != "trace-1" || traces[1].ID != "trace-2" || listCalls != 2 {
		t.Fatalf("paged recall traces = %+v, calls=%d", traces, listCalls)
	}
}
