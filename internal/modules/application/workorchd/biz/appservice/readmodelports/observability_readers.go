package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type EventRecordReader struct {
	Hub magicevent.Hub
}

func (r EventRecordReader) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return ownerreadports.ListEventRecordsByOwnerCommand(ctx, r.Hub, query)
}

type MemoryRecallTraceReader struct {
	Hub magicevent.Hub
}

func (r MemoryRecallTraceReader) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.Hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandList, "workorchd/readmodelports", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.ListMemoryRecallTracesCommand{Query: query})
	ownerevent.BindLane(event, memoryevents.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, "")
	response, err := ownerevent.Memory(r.Hub.Send(event), memoryevents.TopicMemoryRecallTraceCommandList)
	if err != nil {
		return nil, err
	}
	return response.RecallTraces, nil
}
