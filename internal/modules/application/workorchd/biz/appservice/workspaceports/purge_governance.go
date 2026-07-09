package workspaceports

import (
	"context"
	"errors"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceDecisionTickets(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := sendWorkspaceMagicCommand[decisionevents.ListTicketsByWorkspaceCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandListTicketsByWS, decisionevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", decisionevents.ListTicketsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
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
		_, _ = sendWorkspaceMagicCommand[decisionevents.DeleteResolutionCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandDeleteResolution, decisionevents.CommandDestination, "decision_ticket", string(item.ID), workspaceID, item.RunID, decisionevents.DeleteResolutionCommand{
			TicketID:    item.ID,
			WorkspaceID: workspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
		})
		_, err := sendWorkspaceMagicCommand[decisionevents.DeleteTicketCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandDeleteTicket, decisionevents.CommandDestination, "decision_ticket", string(item.ID), workspaceID, item.RunID, decisionevents.DeleteTicketCommand{
			TicketID:    item.ID,
			WorkspaceID: workspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
		})
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}

func purgeWorkspaceApprovals(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := sendWorkspaceMagicCommand[approvalevents.ListApprovalsByWorkspaceCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandListByWorkspace, approvalevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", approvalevents.ListApprovalsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
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
		_, err := sendWorkspaceMagicCommand[approvalevents.DeleteApprovalCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandDelete, approvalevents.CommandDestination, "approval", string(item.ID), workspaceID, item.RunID, approvalevents.DeleteApprovalCommand{
			ApprovalID:  item.ID,
			WorkspaceID: workspaceID,
			RunID:       item.RunID,
			StepID:      item.StepID,
		})
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
