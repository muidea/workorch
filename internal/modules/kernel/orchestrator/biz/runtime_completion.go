package orchestrator

import (
	"context"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateRunning && r.State != common.RunStateValidating {
		return common.ErrConflict
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
	workspace, changed, err := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, r, common.WorkspaceStateCompleted)
	if err != nil || !changed {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) ValidateRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateRunning {
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

func (o *RuntimeOrchestrator) validateAndCompleteRun(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	return o.validateAndCompleteRunWithWorkspace(ctx, run, currentStepID, nil)
}

func (o *RuntimeOrchestrator) validateAndCompleteRunWithWorkspace(ctx context.Context, run *runModel.Run, currentStepID common.StepID, workspace *workspaceModel.Workspace) error {
	if run == nil || run.ID == "" {
		return common.ErrInvalidArgument
	}
	if run.State != common.RunStateRunning {
		return common.ErrConflict
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

	workspace, err = o.runStateManager().syncRunAndWorkspaceForCompletion(ctx, run, workspace)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChangedAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, run.ID)
}
