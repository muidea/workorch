package runtimeports

import (
	"context"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
)

func (p *EventHubActionPort) EnsureApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if p == nil || p.hub == nil || approvalID == "" || workspaceID == "" || runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, approvalevents.TopicApprovalCommandEnsure, approvalevents.CommandDestination, "approval", string(approvalID), workspaceID, runID, stepID, approvalevents.EnsureApprovalCommand{
		ApprovalID:  approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      reason,
	})
}

func (p *EventHubActionPort) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || approvalID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	return p.send(ctx, approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.CommandDestination, "approval", string(approvalID), workspaceID, "", "", approvalevents.EnsureApprovedCommand{ApprovalID: approvalID, WorkspaceID: workspaceID})
}

func (p *EventHubActionPort) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || approvalID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	return p.send(ctx, approvalevents.TopicApprovalCommandEnsureRejected, approvalevents.CommandDestination, "approval", string(approvalID), workspaceID, "", "", approvalevents.EnsureRejectedCommand{ApprovalID: approvalID, WorkspaceID: workspaceID})
}
