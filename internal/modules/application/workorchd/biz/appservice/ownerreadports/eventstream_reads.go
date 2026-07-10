package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ListEventRecordsByOwnerCommand(ctx context.Context, hub magicevent.Hub, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[eventstreamevents.ListEventsCommand, eventstreamevents.EventStreamCommandResult](ctx, hub, eventstreamevents.TopicEventStreamCommandList, eventstreamevents.CommandDestination, "event_record", "list", eventstreamevents.ListEventsCommand{Query: query})
	if err != nil {
		return nil, err
	}
	return response.Events, nil
}
