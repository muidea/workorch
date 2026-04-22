package orchestrator

import (
	"context"
	"fmt"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
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
	tickets    repository.DecisionTicketRepository
	approvals  repository.ApprovalRepository
	eventHub   events.Hub
}

func NewRuntimeOrchestrator(
	runtime RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	approvalRepo repository.ApprovalRepository,
	eventHub events.Hub,
) *RuntimeOrchestrator {
	if eventHub == nil {
		eventHub = events.NewMemoryHub()
	}
	return &RuntimeOrchestrator{
		runtime:    runtime,
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		tickets:    ticketRepo,
		approvals:  approvalRepo,
		eventHub:   eventHub,
	}
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
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	ws.ActiveRunID = runID
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return err
	}
	if err := o.publishWorkspaceStateChanged(ctx, ws, runID); err != nil {
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

	stepID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		if err != common.ErrNotFound {
			return err
		}
		step = &runModel.Step{
			ID:    stepID,
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
	} else {
		step.State = common.StepStateDispatched
		if step.Input == nil {
			step.Input = map[string]any{
				"workspaceId": string(r.WorkspaceID),
			}
		}
		if err := o.steps.Update(ctx, step); err != nil {
			return err
		}
	}
	if err := o.publishStepState(ctx, r, step, events.TopicStepDispatched); err != nil {
		return err
	}

	output, err := o.runtime.ExecuteStep(ctx, runID, step.ID)
	if err != nil {
		step.State = common.StepStateFailed
		step.Output = map[string]any{"reason": err.Error()}
		_ = o.steps.Update(ctx, step)
		_ = o.publishStepState(ctx, r, step, events.TopicStepStateChanged)
		return o.FailRun(ctx, runID, err.Error())
	}

	step.State = common.StepStateCompleted
	step.Output = output
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	if err := o.publishStepState(ctx, r, step, events.TopicStepStateChanged); err != nil {
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
	r.BlockingTicketID = ticketID
	r.BlockingApprovalID = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	return o.publishRunStateChanged(ctx, r)
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
	r.BlockingApprovalID = approvalID
	r.BlockingTicketID = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	return o.publishRunStateChanged(ctx, r)
}

func (o *RuntimeOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}

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
			return common.ErrBlocked
		}
		r.BlockingTicketID = ""
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
			r.BlockingApprovalID = ""
		case common.ApprovalStateRejected:
			return o.FailRun(ctx, runID, fmt.Sprintf("approval %s rejected", approval.ID))
		default:
			return common.ErrBlocked
		}
	default:
		return common.ErrConflict
	}

	r.State = common.RunStateRunning
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
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
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}

	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == runID {
		ws.ActiveRunID = ""
		if err := o.workspaces.Update(ctx, ws); err != nil {
			return err
		}
		return o.publishWorkspaceStateChanged(ctx, ws, runID)
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
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}

	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == runID {
		ws.ActiveRunID = ""
		if err := o.workspaces.Update(ctx, ws); err != nil {
			return err
		}
		return o.publishWorkspaceStateChanged(ctx, ws, runID)
	}
	return nil
}

func (o *RuntimeOrchestrator) publishRunStateChanged(ctx context.Context, run *runModel.Run) error {
	if o.eventHub == nil || run == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.Phase1EventVersion,
		Topic:        events.TopicRunStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "run",
		SubjectID:    string(run.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		Payload:      events.RunPayload(run.State, run.BlockingTicketID, run.BlockingApprovalID),
	})
}

func (o *RuntimeOrchestrator) publishWorkspaceStateChanged(ctx context.Context, workspace *workspaceModel.Workspace, runID common.RunID) error {
	if o.eventHub == nil || workspace == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.Phase1EventVersion,
		Topic:        events.TopicWorkspaceStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "workspace",
		SubjectID:    string(workspace.ID),
		WorkspaceID:  string(workspace.ID),
		RunID:        string(runID),
		Payload:      events.WorkspacePayload("", workspace.State, workspace.ActiveRunID),
	})
}

func (o *RuntimeOrchestrator) publishStepState(ctx context.Context, run *runModel.Run, step *runModel.Step, topic string) error {
	if o.eventHub == nil || run == nil || step == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.Phase1EventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "step",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload:      events.StepPayload(step.Kind, step.State, step.Input, step.Output),
	})
}
