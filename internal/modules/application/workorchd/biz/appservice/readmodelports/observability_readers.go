package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type EventRecordReader struct {
	Hub magicevent.Hub
}

func (r EventRecordReader) List(ctx context.Context, query eventstreamevents.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return ownerreadports.ListEventRecordsByOwnerCommand(ctx, r.Hub, query)
}

type MemoryRecallTraceReader struct {
	Hub magicevent.Hub
}

func (r MemoryRecallTraceReader) List(ctx context.Context, query memoryevents.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.Hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandList, "workorchd/readmodelports", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryRecallTracesCommand{Query: memoryevents.MemoryRecallTraceQuery{WorkspaceID: query.WorkspaceID, RunID: query.RunID, StepID: query.StepID, Stage: query.Stage, Source: query.Source, Status: query.Status, Limit: query.Limit}})
	ownerevent.BindLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, "")
	response, err := ownerevent.Memory(r.Hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandList)
	if err != nil {
		return nil, err
	}
	return response.RecallTraces, nil
}
