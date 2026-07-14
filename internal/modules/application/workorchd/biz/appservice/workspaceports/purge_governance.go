package workspaceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/pkg/common"
)

func purgeWorkspaceDecisionTickets(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTicketsByWS, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
	ownerevent.BindLane(event, decisionevents.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Decision(hub.Send(event), decisionevents.TopicDecisionCommandListTicketsByWS)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	items := response.Tickets
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		resolutionEvent := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandDeleteResolution, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.DeleteResolutionCommand{
			TicketID:    item.ID,
			WorkspaceID: workspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
		})
		ownerevent.BindLane(resolutionEvent, decisionevents.TopicDecisionCommandDeleteResolution, "decision_ticket", string(item.ID), workspaceID, item.RunID, item.StepID)
		_, _ = ownerevent.Decision(hub.Send(resolutionEvent), decisionevents.TopicDecisionCommandDeleteResolution)
		ticketEvent := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandDeleteTicket, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.DeleteTicketCommand{
			TicketID:    item.ID,
			WorkspaceID: workspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
		})
		ownerevent.BindLane(ticketEvent, decisionevents.TopicDecisionCommandDeleteTicket, "decision_ticket", string(item.ID), workspaceID, item.RunID, item.StepID)
		_, err := ownerevent.Decision(hub.Send(ticketEvent), decisionevents.TopicDecisionCommandDeleteTicket)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}

func purgeWorkspaceApprovals(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandListByWorkspace, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
	ownerevent.BindLane(event, approvalevents.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Approval(hub.Send(event), approvalevents.TopicApprovalCommandListByWorkspace)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	items := response.Approvals
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandDelete, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.DeleteApprovalCommand{
			ApprovalID:  item.ID,
			WorkspaceID: workspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
		})
		ownerevent.BindLane(event, approvalevents.TopicApprovalCommandDelete, "approval", string(item.ID), workspaceID, item.RunID, item.StepID)
		_, err := ownerevent.Approval(hub.Send(event), approvalevents.TopicApprovalCommandDelete)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
