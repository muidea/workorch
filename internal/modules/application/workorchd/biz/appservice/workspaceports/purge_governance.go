package workspaceports

import (
	"context"
	"errors"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceDecisionTickets(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := ownerreadports.PublishCommandWithResponse(ctx, hub, decisionevents.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		_ = ownercommandports.Publish(ctx, hub, decisionevents.TopicDecisionCommandDeleteResolution, "decision_ticket", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: decisioncontracts.DeleteResolutionCommand{TicketID: item.ID},
		})
		if err := ownercommandports.Publish(ctx, hub, decisionevents.TopicDecisionCommandDeleteTicket, "decision_ticket", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: decisioncontracts.DeleteTicketCommand{TicketID: item.ID},
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}

func purgeWorkspaceApprovals(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := ownerreadports.PublishCommandWithResponse(ctx, hub, approvalevents.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := ownercommandports.Publish(ctx, hub, approvalevents.TopicApprovalCommandDelete, "approval", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: approvalcontracts.DeleteApprovalCommand{ApprovalID: item.ID},
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
