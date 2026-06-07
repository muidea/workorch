package ownerreadports

import (
	"context"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ListEventRecordsByOwnerCommand(ctx context.Context, hub events.Hub, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicEventStreamCommandList, "event_record", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*eventstreammodel.EventRecord)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
