package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type EventRecordReader struct {
	Hub magicevent.Hub
}

func (r EventRecordReader) List(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return ownerreadports.ListEventRecordsByOwnerCommand(ctx, r.Hub, query)
}

type MemoryRecallTraceReader struct {
	Hub magicevent.Hub
}

func (r MemoryRecallTraceReader) List(ctx context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.Hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandList, "workorchd/readmodelports", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryRecallTracesCommand{Query: query})
	ownerevent.BindLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, "")
	response, err := ownerevent.Memory[memoryevents.ListMemoryRecallTracesResult](r.Hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandList)
	if err != nil {
		return nil, err
	}
	ret := make([]*memorymodel.MemoryRecallTrace, 0, len(response.Traces))
	for _, item := range response.Traces {
		trace, err := item.Model()
		if err != nil {
			return nil, err
		}
		ret = append(ret, trace)
	}
	return ret, nil
}
