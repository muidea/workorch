package runtimecore

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Adapter) EnsureApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Approval); err != nil {
		return err
	}
	return r.actions.Approval.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, reason)
}

func (r *Adapter) GetApproval(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetApproval(ctx, approvalID)
}

func (r *Adapter) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Approval); err != nil {
		return err
	}
	return r.actions.Approval.EnsureApprovalApproved(ctx, approvalID)
}

func (r *Adapter) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Approval); err != nil {
		return err
	}
	return r.actions.Approval.EnsureApprovalRejected(ctx, approvalID)
}
