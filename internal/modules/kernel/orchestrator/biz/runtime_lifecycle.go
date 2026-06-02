package orchestrator

import (
	"context"
	"errors"
	"fmt"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) StartRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}

	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateCreated {
		return common.ErrConflict
	}
	ws, err := o.runStateManager().workspaceForCreatedRunStart(ctx, r)
	if err != nil {
		return err
	}

	r.State = common.RunStateRunning
	r.FailureReason = ""
	ws, err = o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, ws)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	if err := o.eventPublisher().publishWorkspaceStateChanged(ctx, ws, runID); err != nil {
		return err
	}
	specs, err := o.nextDispatchSpecs(ctx, r)
	if err != nil {
		return err
	}
	if err := o.dispatchStepSpecsWithWorkspace(ctx, r, specs, ws); err != nil {
		return errors.Join(err, o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
	}
	return o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID)
}

func (o *RuntimeOrchestrator) DispatchNextStep(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	if o.runtime == nil {
		return common.ErrInvalidArgument
	}

	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	specs, err := o.nextDispatchSpecs(ctx, r)
	if err != nil {
		return err
	}
	return o.dispatchStepSpecsWithWorkspace(ctx, r, specs, nil)
}

func (o *RuntimeOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if runID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.blockOnDecisionAtStep(ctx, r, nil, ticketID, "")
}

func (o *RuntimeOrchestrator) blockOnDecisionAtStep(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, ticketID common.TicketID, currentStepID common.StepID) error {
	if r == nil || r.ID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	if r.State != common.RunStateCreated && r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnDecision
	r.BlockingTicketID = ticketID
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	workspace, err := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateWaitingDecision, workspace)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChangedAtStep(ctx, r, currentStepID); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.blockOnApprovalAtStep(ctx, r, nil, approvalID, "")
}

func (o *RuntimeOrchestrator) blockOnApprovalAtStep(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, approvalID common.ApprovalID, currentStepID common.StepID) error {
	if r == nil || r.ID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	if r.State != common.RunStateCreated && r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnApproval
	r.BlockingApprovalID = approvalID
	r.BlockingTicketID = ""
	r.FailureReason = ""
	workspace, err := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateBlocked, workspace)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChangedAtStep(ctx, r, currentStepID); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	return o.ResumeRunWithExecutionContextPatch(ctx, runID, nil)
}

func (o *RuntimeOrchestrator) ResumeRunWithExecutionContextPatch(ctx context.Context, runID common.RunID, executionContextPatch map[string]any) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.ResumeLoadedRunWithExecutionContextPatch(ctx, r, executionContextPatch)
}

func (o *RuntimeOrchestrator) ResumeLoadedRunWithExecutionContextPatch(ctx context.Context, r *runModel.Run, executionContextPatch map[string]any) error {
	if r == nil || r.ID == "" {
		return common.ErrInvalidArgument
	}
	var resumeAfterStepID common.StepID
	var resumeInput map[string]any
	switch r.State {
	case common.RunStateBlockedOnDecision:
		if r.BlockingTicketID == "" || o.tickets == nil {
			return common.ErrConflict
		}
		ticket, err := o.tickets.Get(ctx, r.BlockingTicketID)
		if err != nil {
			return err
		}
		if ticket.State != common.DecisionTicketStateResolved {
			if runtimeRepairExhaustionBlockerHasSuccessEvidence(ctx, o.steps, ticket) {
				r.BlockingTicketID = ""
				break
			}
			if duplicateRuntimeFailureBlockerCanAutoResume(ctx, o.steps, o.repairManager(), o.policy, r, ticket) {
				resumeAfterStepID = ticket.StepID
				r.BlockingTicketID = ""
				break
			}
			if runtimeRepairExhaustionBlockerCanAutoResume(ctx, o.steps, o.repairManager(), o.policy, r, ticket) {
				ticket.State = common.DecisionTicketStateDismissed
				if err := o.tickets.Update(ctx, ticket); err != nil {
					return err
				}
				resumeAfterStepID = ticket.StepID
				r.BlockingTicketID = ""
				break
			}
			if knownReadableDecisionBlockerCanAutoRepair(ctx, o.steps, o.workspaces, r, ticket) {
				ticket.State = common.DecisionTicketStateDismissed
				if err := o.tickets.Update(ctx, ticket); err != nil {
					return err
				}
				resumeAfterStepID = ticket.StepID
				resumeInput = map[string]any{"knownReadableDecisionRepair": true}
				r.BlockingTicketID = ""
				break
			}
			return common.ErrBlocked
		}
		resumeAfterStepID = ticket.StepID
		if o.resolutions != nil {
			resolution, err := o.resolutions.GetByTicketID(ctx, r.BlockingTicketID)
			if err != nil {
				return err
			}
			switch effectiveDecisionResolutionType(ticket, resolution) {
			case decisioncommon.ResolutionTypeProvideMissingInfo:
				resumeInput = decisionResolutionInput(ticket, resolution)
				r.BlockingTicketID = ""
			case decisioncommon.ResolutionTypeRejectCurrentPath:
				mergeRunExecutionContextPatch(r, executionContextPatch)
				return o.closeLoadedRun(ctx, r, common.RunStateFailed, fmt.Sprintf("decision ticket %s rejected", ticket.ID))
			case decisioncommon.ResolutionTypeReplan:
				mergeRunExecutionContextPatch(r, executionContextPatch)
				return o.replanLoadedRun(ctx, r, ticket, resolution)
			default:
				return common.ErrInvalidArgument
			}
		} else {
			r.BlockingTicketID = ""
		}
	case common.RunStateBlockedOnApproval:
		if r.BlockingApprovalID == "" || o.approvals == nil {
			return common.ErrConflict
		}
		approval, err := o.approvals.Get(ctx, r.BlockingApprovalID)
		if err != nil {
			return err
		}
		switch approval.State {
		case common.ApprovalStateApproved:
			resumeAfterStepID = approval.StepID
			resumeInput = approvalResolutionInput(approval)
			r.BlockingApprovalID = ""
		case common.ApprovalStateRejected:
			mergeRunExecutionContextPatch(r, executionContextPatch)
			return o.closeLoadedRun(ctx, r, common.RunStateFailed, fmt.Sprintf("approval %s rejected", approval.ID))
		default:
			return common.ErrBlocked
		}
	case common.RunStateRunning:
		recoverStepID, recoverInput, err := o.runningDecisionRecoveryContext(ctx, r)
		if err != nil {
			if !errors.Is(err, common.ErrConflict) {
				return err
			}
			specs, specErr := o.runningTerminalStepRecoverySpecs(ctx, r)
			if specErr != nil {
				return specErr
			}
			if len(specs) == 0 {
				return err
			}
			r.State = common.RunStateRunning
			r.FailureReason = ""
			mergeRunExecutionContextPatch(r, executionContextPatch)
			workspace, syncErr := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, nil)
			if syncErr != nil {
				return syncErr
			}
			if publishErr := o.eventPublisher().publishRunStateChanged(ctx, r); publishErr != nil {
				return publishErr
			}
			if publishErr := o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID); publishErr != nil {
				return publishErr
			}
			if dispatchErr := o.dispatchStepSpecsWithWorkspace(ctx, r, specs, workspace); dispatchErr != nil {
				return errors.Join(o.failRunningResume(ctx, r.ID, dispatchErr), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
			}
			return o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID)
		}
		if recoverStepID == "" {
			if len(executionContextPatch) > 0 {
				mergeRunExecutionContextPatch(r, executionContextPatch)
				workspace, err := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, nil)
				if err != nil {
					return err
				}
				if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
					return err
				}
				return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID)
			}
			return nil
		}
		resumeAfterStepID = recoverStepID
		resumeInput = recoverInput
	default:
		return common.ErrConflict
	}

	r.State = common.RunStateRunning
	r.FailureReason = ""
	mergeRunExecutionContextPatch(r, executionContextPatch)
	workspace, err := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, nil)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	if err := o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID); err != nil {
		return err
	}
	specs, err := o.nextDispatchSpecsAfterResolvedDecision(ctx, r, resumeAfterStepID, resumeInput)
	if err != nil {
		return o.failRunningResume(ctx, r.ID, err)
	}
	mergeResumeInput(specs, resumeInput)
	if err := o.dispatchStepSpecsWithWorkspace(ctx, r, specs, workspace); err != nil {
		return errors.Join(o.failRunningResume(ctx, r.ID, err), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
	}
	return o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID)
}
