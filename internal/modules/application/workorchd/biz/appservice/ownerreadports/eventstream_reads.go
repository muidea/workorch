package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func ListEventRecordsByOwnerCommand(ctx context.Context, hub magicevent.Hub, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamCommandList, sourceModule, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, eventstreamevents.ListEventsCommand{Query: query.Clone()})
	ownerevent.BindLane(event, eventstreamevents.Contracts(), eventstreamevents.TopicEventStreamCommandList, "event_record", "list", query.WorkspaceID, query.RunID, "")
	response, err := ownerevent.EventStream(hub.Send(event), eventstreamevents.TopicEventStreamCommandList)
	if err != nil {
		return nil, err
	}
	items := make([]*eventstreammodel.EventRecord, 0, len(response.Events))
	for _, item := range response.Events {
		items = append(items, item.Model())
	}
	return items, nil
}
