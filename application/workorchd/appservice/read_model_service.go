package appservice

import (
	"context"
	"errors"

	"github.com/muidea/workorch/application/workorchd/observed"
	"github.com/muidea/workorch/application/workorchd/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type RunSnapshot struct {
	Workspace Workspace                `json:"workspace"`
	Run       Run                      `json:"run"`
	Step      *presenter.StepView      `json:"step,omitempty"`
	Steps     []presenter.StepView     `json:"steps,omitempty"`
	Decisions []Decision               `json:"decisions,omitempty"`
	Approvals []Approval               `json:"approvals,omitempty"`
	Observed  []presenter.ObservedView `json:"observed,omitempty"`
}

type RunStatus struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspaceId"`
	State              string `json:"state"`
	CurrentStepID      string `json:"currentStepId,omitempty"`
	CurrentStepKind    string `json:"currentStepKind,omitempty"`
	CurrentStepState   string `json:"currentStepState,omitempty"`
	StepCount          int    `json:"stepCount"`
	DecisionCount      int    `json:"decisionCount"`
	ApprovalCount      int    `json:"approvalCount"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	FailureReason      string `json:"failureReason,omitempty"`
}

type WorkspaceSnapshot struct {
	Workspace Workspace  `json:"workspace"`
	Runs      []Run      `json:"runs,omitempty"`
	Decisions []Decision `json:"decisions,omitempty"`
	Approvals []Approval `json:"approvals,omitempty"`
}

type readModelDependencies struct {
	workspaces  repository.WorkspaceRepository
	runs        repository.RunRepository
	steps       repository.StepRepository
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	approvals   repository.ApprovalRepository
	published   []events.WorkorchEvent
}

func ReadRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunSnapshot{}, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	return readRunSnapshot(ctx, deps, common.RunID(runID))
}

func ReadRunSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunSnapshot, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunSnapshotFromEnv(ctx context.Context, runID string) (RunSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunStatusWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunStatus, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunStatus{}, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return RunStatus{}, err
	}
	return mapRunStatus(snapshot), nil
}

func ReadRunStatusWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunStatus, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadRunStatusFromEnv(ctx context.Context, runID string) (RunStatus, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadWorkspaceSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceSnapshotFromEnv(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func readModelDependenciesFromSession(session *RuntimeSession) (readModelDependencies, error) {
	if session == nil || session.Bootstrap == nil {
		return readModelDependencies{}, common.ErrInvalidArgument
	}
	exports := session.Bootstrap.Exports()
	if exports.Repositories.Workspaces == nil ||
		exports.Repositories.Runs == nil ||
		exports.Repositories.Steps == nil ||
		exports.Repositories.Tickets == nil ||
		exports.Repositories.Resolutions == nil ||
		exports.Repositories.Approvals == nil {
		return readModelDependencies{}, common.ErrInvalidArgument
	}

	var published []events.WorkorchEvent
	if session.Runtime != nil {
		published = session.Runtime.PublishedWorkorchEvents()
	}

	return readModelDependencies{
		workspaces:  exports.Repositories.Workspaces,
		runs:        exports.Repositories.Runs,
		steps:       exports.Repositories.Steps,
		tickets:     exports.Repositories.Tickets,
		resolutions: exports.Repositories.Resolutions,
		approvals:   exports.Repositories.Approvals,
		published:   published,
	}, nil
}

func readRunSnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	workspace, err := deps.workspaces.Get(ctx, item.WorkspaceID)
	if err != nil {
		return RunSnapshot{}, err
	}
	steps, err := deps.steps.ListByRun(ctx, item.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	decisions, selectedTicket, selectedResolution, err := decisionsForRun(ctx, deps, item)
	if err != nil {
		return RunSnapshot{}, err
	}
	approvals, selectedApproval, err := approvalsForRun(ctx, deps, item)
	if err != nil {
		return RunSnapshot{}, err
	}

	currentStep := lastStep(steps)
	observedItems := observed.BuildWorkorchSnapshot(workspace, item, steps, selectedTicket, selectedResolution, selectedApproval, deps.published)

	return RunSnapshot{
		Workspace: mapWorkspace(workspace),
		Run:       mapRun(item),
		Step:      presenter.MapStep(currentStep),
		Steps:     presenter.MapSteps(steps),
		Decisions: decisions,
		Approvals: approvals,
		Observed:  presenter.MapObserved(observedItems),
	}, nil
}

func readWorkspaceSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (WorkspaceSnapshot, error) {
	item, err := deps.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}

	runItems, err := deps.runs.List(ctx)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	runs := make([]Run, 0, len(runItems))
	for _, runItem := range runItems {
		if runItem.WorkspaceID != workspaceID {
			continue
		}
		runs = append(runs, mapRun(runItem))
	}

	ticketItems, err := deps.tickets.List(ctx)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	decisions := make([]Decision, 0, len(ticketItems))
	for _, ticket := range ticketItems {
		if ticket.WorkspaceID != workspaceID {
			continue
		}
		resolution, err := resolutionForTicket(ctx, deps.resolutions, ticket)
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		decisions = append(decisions, mapDecision(ticket, resolution))
	}

	approvalItems, err := deps.approvals.List(ctx)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	approvals := make([]Approval, 0, len(approvalItems))
	for _, approval := range approvalItems {
		if approval.WorkspaceID != workspaceID {
			continue
		}
		approvals = append(approvals, mapApproval(approval))
	}

	return WorkspaceSnapshot{
		Workspace: mapWorkspace(item),
		Runs:      runs,
		Decisions: decisions,
		Approvals: approvals,
	}, nil
}

func decisionsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Decision, *decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	items, err := deps.tickets.List(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	ret := make([]Decision, 0, len(items))
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		resolution, err := resolutionForTicket(ctx, deps.resolutions, item)
		if err != nil {
			return nil, nil, nil, err
		}
		ret = append(ret, mapDecision(item, resolution))
		if run.BlockingTicketID != "" && item.ID == run.BlockingTicketID {
			selectedTicket = item
			selectedResolution = resolution
			continue
		}
		if selectedTicket == nil || run.BlockingTicketID == "" {
			selectedTicket = item
			selectedResolution = resolution
		}
	}
	return ret, selectedTicket, selectedResolution, nil
}

func approvalsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Approval, *approvalmodel.Approval, error) {
	items, err := deps.approvals.List(ctx)
	if err != nil {
		return nil, nil, err
	}

	ret := make([]Approval, 0, len(items))
	var selectedApproval *approvalmodel.Approval
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		ret = append(ret, mapApproval(item))
		if run.BlockingApprovalID != "" && item.ID == run.BlockingApprovalID {
			selectedApproval = item
			continue
		}
		if selectedApproval == nil || run.BlockingApprovalID == "" {
			selectedApproval = item
		}
	}
	return ret, selectedApproval, nil
}

func lastStep(steps []*runmodel.Step) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	return steps[len(steps)-1]
}

func mapRunStatus(snapshot RunSnapshot) RunStatus {
	ret := RunStatus{
		ID:                 snapshot.Run.ID,
		WorkspaceID:        snapshot.Run.WorkspaceID,
		State:              snapshot.Run.State,
		StepCount:          len(snapshot.Steps),
		DecisionCount:      len(snapshot.Decisions),
		ApprovalCount:      len(snapshot.Approvals),
		BlockingTicketID:   snapshot.Run.BlockingTicketID,
		BlockingApprovalID: snapshot.Run.BlockingApprovalID,
		FailureReason:      snapshot.Run.FailureReason,
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = snapshot.Step.Kind
		ret.CurrentStepState = snapshot.Step.State
	}
	return ret
}
