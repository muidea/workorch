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
	r, ws, err := o.acceptRunStart(ctx, runID)
	if err != nil {
		return err
	}
	return o.dispatchAcceptedRun(ctx, r, ws)
}

func (o *RuntimeOrchestrator) AcceptRunStart(ctx context.Context, runID common.RunID) error {
	_, _, err := o.acceptRunStart(ctx, runID)
	return err
}

func (o *RuntimeOrchestrator) acceptRunStart(ctx context.Context, runID common.RunID) (*runModel.Run, *workspaceModel.Workspace, error) {
	if runID == "" {
		return nil, nil, common.ErrInvalidArgument
	}

	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	if r.State != common.RunStateCreated {
		return nil, nil, common.ErrConflict
	}
	ws, err := o.runStateManager().workspaceForCreatedRunStart(ctx, r)
	if err != nil {
		return nil, nil, err
	}

	r.State = common.RunStateRunning
	r.FailureReason = ""
	ws, err = o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, ws)
	if err != nil {
		return nil, nil, err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
		return nil, nil, err
	}
	if err := o.eventPublisher().publishWorkspaceStateChanged(ctx, ws, runID); err != nil {
		return nil, nil, err
	}
	return r, ws, nil
}

func (o *RuntimeOrchestrator) dispatchAcceptedRun(ctx context.Context, r *runModel.Run, ws *workspaceModel.Workspace) error {
	if r == nil || r.ID == "" {
		return common.ErrInvalidArgument
	}
	specs, err := o.nextDispatchSpecs(ctx, r)
	if err != nil {
		return errors.Join(o.failRunningResume(ctx, r.ID, err), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
	}
	if err := dispatchStepSpecsWithWorkspace(ctx, o.dispatchStepWithWorkspace, o.dispatchGraphStepBatchWithWorkspace, r, specs, ws); err != nil {
		return errors.Join(o.failRunningResume(ctx, r.ID, err), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
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
	return dispatchStepSpecsWithWorkspace(ctx, o.dispatchStepWithWorkspace, o.dispatchGraphStepBatchWithWorkspace, r, specs, nil)
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

func (o *RuntimeOrchestrator) CanResumeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	switch r.State {
	case common.RunStateRunning, common.RunStateValidating:
		return nil
	case common.RunStateBlockedOnDecision:
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
		approval, err := o.approvals.Get(ctx, r.BlockingApprovalID)
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
	originalResumeState := runResumeStateSnapshotFromRun(r)
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
			policy := o.RuntimePolicy()
			if duplicateRuntimeFailureBlockerCanAutoResume(ctx, o.steps, o.repairManager(), policy, r, ticket) {
				resumeAfterStepID = ticket.StepID
				r.BlockingTicketID = ""
				break
			}
			if runtimeRepairExhaustionBlockerCanAutoResume(ctx, o.steps, o.repairManager(), policy, r, ticket) {
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
			specs, specErr := runningTerminalStepRecoverySpecs(
				ctx,
				o.steps,
				func(ctx context.Context, run *runModel.Run) ([]stepDispatchSpec, bool, error) {
					return artifactContinuationRecoverySpecs(ctx, o.steps, o.evidence.ArtifactContinuations, run)
				},
				o.nextDispatchSpecsFromStep,
				r,
			)
			if specErr != nil {
				return specErr
			}
			if len(specs) == 0 {
				specs, specErr = runningInitialDispatchRecoverySpecs(ctx, o.steps, o.nextDispatchSpecs, r)
				if specErr != nil {
					return errors.Join(o.failRunningResume(ctx, r.ID, specErr), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
				}
				if len(specs) == 0 {
					return err
				}
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
			if dispatchErr := dispatchStepSpecsWithWorkspace(ctx, o.dispatchStepWithWorkspace, o.dispatchGraphStepBatchWithWorkspace, r, specs, workspace); dispatchErr != nil {
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

	resumeRun := cloneRunForResume(r)
	resumeRun.State = common.RunStateRunning
	resumeRun.FailureReason = ""
	mergeRunExecutionContextPatch(resumeRun, executionContextPatch)
	specs, err := nextDispatchSpecsAfterResolvedDecision(
		ctx,
		func(ctx context.Context, run *runModel.Run, stepID common.StepID) ([]stepDispatchSpec, bool, error) {
			return knownReadableDecisionRepairSpecs(ctx, o.steps, o.workspaces, o.repairManager(), o, run, stepID)
		},
		func(ctx context.Context, run *runModel.Run, stepID common.StepID) ([]stepDispatchSpec, bool, error) {
			return providerRetrySpecsAfterResolvedDecision(ctx, o.steps, o.repairManager(), o.RuntimePolicy(), run, stepID)
		},
		func(ctx context.Context, run *runModel.Run, stepID common.StepID) ([]stepDispatchSpec, bool, error) {
			return runtimeRepairSpecsAfterResolvedDecision(ctx, o.steps, o.repairManager(), o, run, stepID)
		},
		func(ctx context.Context, run *runModel.Run, stepID common.StepID, input map[string]any) ([]stepDispatchSpec, bool, error) {
			return contextIntegritySpecsAfterResolvedDecision(ctx, o.steps, run, stepID, input)
		},
		o.nextDispatchSpecsAfterStep,
		resumeRun,
		resumeAfterStepID,
		resumeInput,
	)
	if err != nil {
		return o.failResolvedDecisionResume(ctx, r, originalResumeState, executionContextPatch, err)
	}
	mergeResumeInput(specs, resumeInput)
	r.State = resumeRun.State
	r.FailureReason = resumeRun.FailureReason
	r.BlockingTicketID = resumeRun.BlockingTicketID
	r.BlockingApprovalID = resumeRun.BlockingApprovalID
	r.ExecutionContext = cloneStepPayload(resumeRun.ExecutionContext)
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
	if err := dispatchStepSpecsWithWorkspace(ctx, o.dispatchStepWithWorkspace, o.dispatchGraphStepBatchWithWorkspace, r, specs, workspace); err != nil {
		return errors.Join(o.failRunningResume(ctx, r.ID, err), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
	}
	return o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID)
}

type runResumeStateSnapshot struct {
	state              common.RunState
	blockingTicketID   common.TicketID
	blockingApprovalID common.ApprovalID
}

func runResumeStateSnapshotFromRun(run *runModel.Run) runResumeStateSnapshot {
	if run == nil {
		return runResumeStateSnapshot{}
	}
	return runResumeStateSnapshot{
		state:              run.State,
		blockingTicketID:   run.BlockingTicketID,
		blockingApprovalID: run.BlockingApprovalID,
	}
}

func (s runResumeStateSnapshot) matches(run *runModel.Run) bool {
	return run != nil &&
		run.State == s.state &&
		run.BlockingTicketID == s.blockingTicketID &&
		run.BlockingApprovalID == s.blockingApprovalID
}

func cloneRunForResume(run *runModel.Run) *runModel.Run {
	if run == nil {
		return nil
	}
	cloned := *run
	cloned.ExecutionContext = cloneStepPayload(run.ExecutionContext)
	return &cloned
}

func (o *RuntimeOrchestrator) failResolvedDecisionResume(ctx context.Context, run *runModel.Run, original runResumeStateSnapshot, executionContextPatch map[string]any, cause error) error {
	if cause == nil {
		return nil
	}
	if o == nil || run == nil || run.ID == "" || o.runs == nil {
		return cause
	}
	if errors.Is(cause, common.ErrBlocked) {
		return cause
	}
	current, err := o.runs.Get(ctx, run.ID)
	if err != nil {
		return errors.Join(cause, err)
	}
	if !original.matches(current) {
		return cause
	}
	mergeRunExecutionContextPatch(current, executionContextPatch)
	if err := o.closeLoadedRun(ctx, current, common.RunStateFailed, cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return nil
}
