package orchestrator

import (
	"context"
	"errors"

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
	return o.releaseWorkspaceForTerminalRun(ctx, r, common.WorkspaceStateCompleted)
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
	r.State = common.RunStateValidating
	r.FailureReason = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	return o.syncWorkspaceForActiveRun(ctx, r, common.WorkspaceStateValidating)
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
	workspace, err = o.syncRunAndWorkspaceForValidation(ctx, run, workspace)
	if err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChangedAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if err := o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, run.ID); err != nil {
		return err
	}

	run.State = common.RunStateCompleted
	run.BlockingTicketID = ""
	run.BlockingApprovalID = ""
	run.FailureReason = ""
	workspace.ActiveRunID = ""
	workspace.State = common.WorkspaceStateCompleted
	if err := runParallel(
		func() error {
			return o.runs.Update(ctx, run)
		},
		func() error {
			return o.workspaces.Update(ctx, workspace)
		},
	); err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChangedAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, run.ID)
}

func (o *RuntimeOrchestrator) syncRunAndWorkspaceForValidation(ctx context.Context, run *runModel.Run, workspace *workspaceModel.Workspace) (*workspaceModel.Workspace, error) {
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	ws := workspace
	if ws == nil {
		var err error
		ws, err = o.workspaces.Get(ctx, run.WorkspaceID)
		if err != nil {
			return nil, err
		}
	} else if ws.ID != run.WorkspaceID {
		return nil, common.ErrConflict
	}
	if ws.ActiveRunID != "" && ws.ActiveRunID != run.ID {
		activeRun, err := o.runs.Get(ctx, ws.ActiveRunID)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
		if activeRun != nil && activeRun.WorkspaceID == ws.ID && !isTerminalRunState(activeRun.State) {
			return nil, common.ErrConflict
		}
	}
	run.State = common.RunStateValidating
	run.FailureReason = ""
	ws.ActiveRunID = run.ID
	ws.State = common.WorkspaceStateValidating
	if err := runParallel(
		func() error {
			return o.runs.Update(ctx, run)
		},
		func() error {
			return o.workspaces.Update(ctx, ws)
		},
	); err != nil {
		return nil, err
	}
	return ws, nil
}
