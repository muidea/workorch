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
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandPurgeWorkspace, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.PurgeWorkspaceDecisionsCommand{
		WorkspaceID: workspaceID,
	})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandPurgeWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	_, err := ownerevent.Decision[decisionevents.PurgeWorkspaceDecisionsResult](hub.Send(event), decisionevents.TopicDecisionCommandPurgeWorkspace)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	return nil
}

func purgeWorkspaceApprovals(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandPurgeWorkspace, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.PurgeWorkspaceApprovalsCommand{
		WorkspaceID: workspaceID,
	})
	ownerevent.BindLane(event, approvalevents.Contracts(), approvalevents.TopicApprovalCommandPurgeWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	_, err := ownerevent.Approval[approvalevents.PurgeWorkspaceApprovalsResult](hub.Send(event), approvalevents.TopicApprovalCommandPurgeWorkspace)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	return nil
}
