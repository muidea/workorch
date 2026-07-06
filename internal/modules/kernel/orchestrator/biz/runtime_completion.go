package orchestrator

import (
	"context"
	"fmt"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateActive {
		return common.ErrConflict
	}
	if requiresDoDAssessmentCompletionGate(r) && !o.resultManager().hasAcceptedCompletionResult(ctx, r) {
		return common.ErrConflict
	}
	if err := o.validateRunReadyForCompletion(ctx, r); err != nil {
		return err
	}
	if err := o.finalizeManagedValidationResourcesForRun(ctx, r, "released", "run_completed"); err != nil {
		return err
	}
	r.State = common.RunStateCompleted
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	workspace, changed, err := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, r, common.WorkspaceStateActive)
	if err != nil || !changed {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) ValidateRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateActive {
		return common.ErrConflict
	}
	workspace, err := o.runStateManager().syncRunAndWorkspaceForValidation(ctx, r, nil)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) validateAndCompleteRunWithWorkspace(ctx context.Context, run *runModel.Run, currentStepID common.StepID, workspace *workspaceModel.Workspace) error {
	if run == nil || run.ID == "" {
		return common.ErrInvalidArgument
	}
	if run.State != common.RunStateActive {
		return common.ErrConflict
	}
	if err := o.validateRunReadyForCompletion(ctx, run); err != nil {
		return err
	}
	var err error
	workspace, err = o.runStateManager().syncRunAndWorkspaceForValidation(ctx, run, workspace)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChangedAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if err := o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, run.ID); err != nil {
		return err
	}

	if err := o.finalizeManagedValidationResourcesForRun(ctx, run, "released", "run_completed"); err != nil {
		return err
	}
	workspace, err = o.runStateManager().syncRunAndWorkspaceForCompletion(ctx, run, workspace)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChangedAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, run.ID)
}

func (o *RuntimeOrchestrator) validateRunReadyForCompletion(ctx context.Context, run *runModel.Run) error {
	if o == nil || run == nil {
		return common.ErrInvalidArgument
	}
	if err := o.coverageLedger().validateReadyForAcceptance(run, ctx); err != nil {
		return fmt.Errorf("%w: completion gate not ready: %v", common.ErrConflict, err)
	}
	return nil
}
