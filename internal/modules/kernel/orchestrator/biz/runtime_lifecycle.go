package orchestrator

import (
	"context"

	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) AcceptRunStart(ctx context.Context, runID common.RunID) error {
	_, _, err := o.acceptRunStart(ctx, runID)
	return err
}

func (o *RuntimeOrchestrator) acceptRunStart(ctx context.Context, runID common.RunID) (*runModel.Run, *workspaceModel.Workspace, error) {
	if runID == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	r, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	if r.State != common.RunStatePending || !common.RunStateCanTransition(r.State, common.RunStateActive) {
		return nil, nil, common.ErrConflict
	}
	ws, err := o.runStateManager().workspaceForCreatedRunStart(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	r.State = common.RunStateActive
	r.FailureReason = ""
	ws, err = o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateActive, ws)
	if err != nil {
		return nil, nil, err
	}
	return r, ws, nil
}

func (o *RuntimeOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if runID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	return o.blockOnDecisionAtStep(ctx, r, nil, ticketID)
}

func (o *RuntimeOrchestrator) blockOnDecisionAtStep(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, ticketID common.TicketID) error {
	if r == nil || r.ID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	if (r.State != common.RunStatePending && r.State != common.RunStateActive) || !common.RunStateCanTransition(r.State, common.RunStateBlockedOnDecision) {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnDecision
	r.BlockingTicketID = ticketID
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	workspace, err := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateActive, workspace)
	if err != nil {
		return err
	}
	return nil
}

func (o *RuntimeOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	return o.blockOnApprovalAtStep(ctx, r, nil, approvalID)
}

func (o *RuntimeOrchestrator) blockOnApprovalAtStep(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, approvalID common.ApprovalID) error {
	if r == nil || r.ID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	if (r.State != common.RunStatePending && r.State != common.RunStateActive) || !common.RunStateCanTransition(r.State, common.RunStateBlockedOnApproval) {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnApproval
	r.BlockingApprovalID = approvalID
	r.BlockingTicketID = ""
	r.FailureReason = ""
	workspace, err := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateBlockedOnApproval, workspace)
	if err != nil {
		return err
	}
	return nil
}

func (o *RuntimeOrchestrator) CanResumeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	switch r.State {
	case common.RunStateActive:
		return nil
	case common.RunStateBlockedOnDecision:
		if r.RuntimeContractVersion == runtimecontract.RuntimeContractVersionV2 {
			committed, err := o.hasCommittedRuntimeV2DecisionResolution(ctx, r)
			if err != nil {
				return err
			}
			if !committed {
				return common.ErrBlocked
			}
			return nil
		}
		if r.BlockingTicketID == "" || o.tickets == nil {
			return common.ErrConflict
		}
		ticket, err := o.tickets.Get(ctx, r.BlockingTicketID)
		if err != nil {
			return err
		}
		if ticket.State == common.DecisionTicketStateOpen {
			return common.ErrBlocked
		}
		return nil
	case common.RunStateBlockedOnApproval:
		if r.BlockingApprovalID == "" || o.approvals == nil {
			return common.ErrConflict
		}
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, r.WorkspaceID)
		approval, err := o.approvals.Get(scopedCtx, r.BlockingApprovalID)
		if err != nil {
			return err
		}
		if approval.State == common.ApprovalStatePending {
			return common.ErrBlocked
		}
		return nil
	default:
		return common.ErrConflict
	}
}
