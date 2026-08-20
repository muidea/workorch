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
	query = query.Normalized()
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamCommandList, sourceModule, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, eventstreamevents.ListEventsCommand{Query: eventstreamevents.EventQuery{
		WorkspaceID: query.WorkspaceID, RunID: query.RunID, Topic: query.Topic, ExcludeTopics: append([]string(nil), query.ExcludeTopics...),
		SubjectType: query.SubjectType, SubjectID: query.SubjectID, CursorAfterID: query.CursorAfterID,
		OccurredAfter: query.OccurredAfter, OccurredBefore: query.OccurredBefore, Limit: query.Limit, IncludePayload: query.IncludePayload,
	}})
	ownerevent.BindLane(event, eventstreamevents.Contracts(), eventstreamevents.TopicEventStreamCommandList, "event_record", "list", query.WorkspaceID, query.RunID, "")
	response, err := ownerevent.EventStream[eventstreamevents.ListEventsResult](hub.Send(event), eventstreamevents.TopicEventStreamCommandList)
	if err != nil {
		return nil, err
	}
	items := make([]*eventstreammodel.EventRecord, 0, len(response.Events))
	for _, item := range response.Events {
		items = append(items, item.Model())
	}
	return items, nil
}
