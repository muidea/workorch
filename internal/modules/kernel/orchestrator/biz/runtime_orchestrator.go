package orchestrator

import (
	"context"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type RuntimeAdapter interface {
	ExecuteStep(context.Context, common.RunID, common.StepID) (map[string]any, error)
}

type RuntimeOrchestrator struct {
	runtime    RuntimeAdapter
	runs       repository.RunRepository
	steps      repository.StepRepository
	workspaces repository.WorkspaceRepository
}

func NewRuntimeOrchestrator(runtime RuntimeAdapter, runs repository.RunRepository, steps repository.StepRepository, workspaces repository.WorkspaceRepository) *RuntimeOrchestrator {
	return &RuntimeOrchestrator{runtime: runtime, runs: runs, steps: steps, workspaces: workspaces}
}

func (o *RuntimeOrchestrator) StartRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}

	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID != "" && ws.ActiveRunID != runID {
		return common.ErrConflict
	}

	r.State = common.RunStateRunning
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	ws.ActiveRunID = runID
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return err
	}
	return o.DispatchNextStep(ctx, runID)
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

	step := &runModel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  "legacy/minimal",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"workspaceId": string(r.WorkspaceID),
		},
	}
	if err := o.steps.Create(ctx, step); err != nil {
		return err
	}

	output, err := o.runtime.ExecuteStep(ctx, runID, step.ID)
	if err != nil {
		step.State = common.StepStateFailed
		step.Output = map[string]any{"reason": err.Error()}
		_ = o.steps.Update(ctx, step)
		return o.FailRun(ctx, runID, err.Error())
	}

	step.State = common.StepStateCompleted
	step.Output = output
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	return o.CompleteRun(ctx, runID)
}

func (o *RuntimeOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if runID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	r.State = common.RunStateBlockedOnDecision
	return o.runs.Update(ctx, r)
}

func (o *RuntimeOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	r.State = common.RunStateBlockedOnApproval
	return o.runs.Update(ctx, r)
}

func (o *RuntimeOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	return o.DispatchNextStep(ctx, runID)
}

func (o *RuntimeOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	r.State = common.RunStateCompleted
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}

	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == runID {
		ws.ActiveRunID = ""
		return o.workspaces.Update(ctx, ws)
	}
	return nil
}

func (o *RuntimeOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	if runID == "" || reason == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	r.State = common.RunStateFailed
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}

	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == runID {
		ws.ActiveRunID = ""
		return o.workspaces.Update(ctx, ws)
	}
	return nil
}
