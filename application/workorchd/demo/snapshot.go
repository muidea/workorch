package demo

import (
	"context"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/application/workorchd/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func snapshot(ctx context.Context, rt *bootstrap.Runtime, slice string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, refs presenter.BlockingRefs) (*presenter.Snapshot, error) {
	ws, err := rt.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	r, err := rt.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	var step *runmodel.Step
	if stepID != "" {
		step, err = rt.GetStep(ctx, stepID)
		if err != nil && err != common.ErrNotFound {
			return nil, err
		}
	}

	var ticket *decisionmodel.Ticket
	if refs.TicketID != "" {
		ticket, err = rt.GetTicket(ctx, refs.TicketID)
		if err != nil {
			return nil, err
		}
	}

	var approval *approvalmodel.Approval
	if refs.ApprovalID != "" {
		approval, err = rt.GetApproval(ctx, refs.ApprovalID)
		if err != nil {
			return nil, err
		}
	}

	return presenter.BuildSnapshot(slice, ws, r, step, ticket, approval), nil
}
