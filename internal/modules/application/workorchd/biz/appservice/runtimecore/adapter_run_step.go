package runtimecore

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Adapter) GetRun(ctx context.Context, runID common.RunID) (*runmodel.Run, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetRun(ctx, runID)
}

func (r *Adapter) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRun(ctx, runID, workspaceID)
}

func (r *Adapter) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunStarted(ctx, runID)
}

func (r *Adapter) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunBlockedOnDecision(ctx, runID, ticketID)
}

func (r *Adapter) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunBlockedOnApproval(ctx, runID, approvalID)
}

func (r *Adapter) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunResumed(ctx, runID)
}

func (r *Adapter) GetStep(ctx context.Context, stepID common.StepID) (*runmodel.Step, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetStep(ctx, stepID)
}
