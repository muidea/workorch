package runtimeports

import (
	"context"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"

	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
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
	return p.publish(ctx, approvalevents.TopicApprovalCommandEnsure, "approval", string(approvalID), workspaceID, runID, stepID, map[string]any{
		events.PayloadKeyCommand: approvalcontracts.CreateApprovalCommand{
			ApprovalID:  approvalID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      stepID,
			Reason:      reason,
		},
	})
}

func (p *EventHubActionPort) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || approvalID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyCommand: approvalcontracts.ResolveApprovalCommand{ApprovalID: approvalID, WorkspaceID: workspaceID},
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	return p.publish(ctx, approvalevents.TopicApprovalCommandEnsureApproved, "approval", string(approvalID), workspaceID, "", "", payload)
}

func (p *EventHubActionPort) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || approvalID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyCommand: approvalcontracts.ResolveApprovalCommand{ApprovalID: approvalID, WorkspaceID: workspaceID},
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	return p.publish(ctx, approvalevents.TopicApprovalCommandEnsureRejected, "approval", string(approvalID), workspaceID, "", "", payload)
}
