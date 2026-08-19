package ownerreadports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	maxDecisionReadPages = 1000
	maxDecisionReadItems = 100000
)

func ListDecisionTicketsByWorkspaceOwnerPage(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, cursor string, limit int) ([]*decisionmodel.Ticket, string, error) {
	if hub == nil || workspaceID == "" {
		return nil, "", common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTicketsByWS, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsByWorkspaceCommand{
		WorkspaceID: workspaceID,
		Cursor:      cursor,
		Limit:       limit,
	})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Decision[decisionevents.ListTicketsByWorkspaceResult](hub.Send(event), decisionevents.TopicDecisionCommandListTicketsByWS)
	return response.Tickets, response.NextCursor, err
}

func ListDecisionTicketsOwnerPage(ctx context.Context, hub magicevent.Hub, cursor string, limit int) ([]*decisionmodel.Ticket, string, error) {
	if hub == nil {
		return nil, "", common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTickets, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsCommand{
		Scope:  decisionevents.TicketListScopeAll,
		Cursor: cursor,
		Limit:  limit,
	})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandListTickets, "decision_ticket", "all", "", "", "")
	response, err := ownerevent.Decision[decisionevents.ListTicketsResult](hub.Send(event), decisionevents.TopicDecisionCommandListTickets)
	return response.Tickets, response.NextCursor, err
}

func ListDecisionTicketsByRunOwnerPage(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID, cursor string, limit int) ([]*decisionmodel.Ticket, string, error) {
	if hub == nil || workspaceID == "" || runID == "" {
		return nil, "", common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTicketsByRun, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		Cursor:      cursor,
		Limit:       limit,
	})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandListTicketsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Decision[decisionevents.ListTicketsByRunResult](hub.Send(event), decisionevents.TopicDecisionCommandListTicketsByRun)
	return response.Tickets, response.NextCursor, err
}

func ListDecisionTicketsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return collectDecisionTicketPages(func(cursor string) ([]*decisionmodel.Ticket, string, error) {
		return ListDecisionTicketsByWorkspaceOwnerPage(ctx, hub, workspaceID, cursor, decisionevents.MaxListLimit)
	})
}

func ListDecisionTicketsByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*decisionmodel.Ticket, error) {
	return collectDecisionTicketPages(func(cursor string) ([]*decisionmodel.Ticket, string, error) {
		return ListDecisionTicketsOwnerPage(ctx, hub, cursor, decisionevents.MaxListLimit)
	})
}

func ListDecisionTicketsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	return collectDecisionTicketPages(func(cursor string) ([]*decisionmodel.Ticket, string, error) {
		return ListDecisionTicketsByRunOwnerPage(ctx, hub, workspaceID, runID, cursor, decisionevents.MaxListLimit)
	})
}

func collectDecisionTicketPages(read func(string) ([]*decisionmodel.Ticket, string, error)) ([]*decisionmodel.Ticket, error) {
	if read == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]*decisionmodel.Ticket, 0)
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < maxDecisionReadPages; page++ {
		items, next, err := read(cursor)
		if err != nil {
			return nil, err
		}
		if len(ret)+len(items) > maxDecisionReadItems {
			return nil, fmt.Errorf("%w: decision read exceeds item limit", common.ErrConflict)
		}
		ret = append(ret, decisionmodel.CloneTickets(items)...)
		if next == "" {
			return ret, nil
		}
		if next == cursor {
			return nil, fmt.Errorf("%w: decision read cursor did not advance", common.ErrConflict)
		}
		if _, ok := seen[next]; ok {
			return nil, fmt.Errorf("%w: decision read cursor cycle", common.ErrConflict)
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return nil, fmt.Errorf("%w: decision read exceeds page limit", common.ErrConflict)
}
