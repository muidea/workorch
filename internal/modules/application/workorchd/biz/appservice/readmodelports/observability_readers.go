package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type EventRecordReader struct {
	Hub events.Hub
}

func (r EventRecordReader) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return ownerreadports.ListEventRecordsByOwnerCommand(ctx, r.Hub, query)
}

type MemoryRecallTraceReader struct {
	Hub events.Hub
}

func (r MemoryRecallTraceReader) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.Hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := ownerreadports.PublishCommandWithResponse(ctx, r.Hub, events.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyMemoryRecallTraceQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryRecallTraces].([]*memorymodel.MemoryRecallTrace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
