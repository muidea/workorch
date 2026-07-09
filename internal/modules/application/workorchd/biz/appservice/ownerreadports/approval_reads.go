package ownerreadports

import (
	"context"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListApprovalsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*approvalmodel.Approval, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[approvalevents.ListApprovalsCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandList, approvalevents.CommandDestination, "approval", "all", approvalevents.ListApprovalsCommand{})
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func ListApprovalsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendMagicCommand[approvalevents.ListApprovalsByRunCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandListByRun, approvalevents.CommandDestination, "run", string(runID), approvalevents.ListApprovalsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func ListApprovalsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[approvalevents.ListApprovalsByWorkspaceCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandListByWorkspace, approvalevents.CommandDestination, "workspace", string(workspaceID), approvalevents.ListApprovalsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}
