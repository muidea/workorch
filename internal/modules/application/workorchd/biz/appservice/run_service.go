package appservice

import (
	"context"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Run struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	GoalID             string         `json:"goalId,omitempty"`
	DoDID              string         `json:"doDId,omitempty"`
	InitialStepKind    string         `json:"initialStepKind,omitempty"`
	ExecutionContext   map[string]any `json:"executionContext,omitempty"`
	State              string         `json:"state"`
	Priority           int            `json:"priority,omitempty"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	FailureReason      string         `json:"failureReason,omitempty"`
}

func CreateRunWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string, goalID string, dodID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	if err := validateRunDefinitions(ctx, session, workspaceID, goalID, dodID); err != nil {
		return Run{}, err
	}
	var executionContext map[string]any
	if goalID != "" || dodID != "" {
		executionContext, err = buildWorkspaceExecutionContext(
			ctx,
			session,
			common.WorkspaceID(workspaceID),
			common.GoalID(goalID),
			common.DoDID(dodID),
			ActivateWorkspaceExecutionCommand{},
		)
		if err != nil {
			return Run{}, err
		}
	}

	id := common.RunID(runID)
	if err := service.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:            id,
		WorkspaceID:      common.WorkspaceID(workspaceID),
		GoalID:           common.GoalID(goalID),
		DoDID:            common.DoDID(dodID),
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: executionContext,
	}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func CreateRunWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
}

func CreateRunFromEnv(ctx context.Context, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
}

func StartRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.RunID(runID)
	if err := service.EnsureStarted(ctx, runmodule.StartRunCommand{RunID: id}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func StartRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return StartRunWithSession(ctx, session, runID)
}

func StartRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return StartRunWithSession(ctx, session, runID)
}

func ResumeRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.RunID(runID)
	if err := service.EnsureResumed(ctx, runmodule.ResumeRunCommand{RunID: id}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func ResumeRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return ResumeRunWithSession(ctx, session, runID)
}

func ResumeRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return ResumeRunWithSession(ctx, session, runID)
}

func ReadRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	_, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := repo.Get(ctx, common.RunID(runID))
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func ReadRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return ReadRunWithSession(ctx, session, runID)
}

func ReadRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return ReadRunWithSession(ctx, session, runID)
}

func ListRunsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []Run, err error) {
	_, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Run, 0, len(items))
	for _, item := range items {
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		ret = append(ret, mapRun(item))
	}
	return ret, nil
}

func ListRunsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListRunsWithSession(ctx, session, workspaceID)
}

func ListRunsFromEnv(ctx context.Context, workspaceID string) ([]Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListRunsWithSession(ctx, session, workspaceID)
}

func runDependenciesFromSession(session *RuntimeSession) (runmodule.Service, repository.RunRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Services.RunService == nil || exports.Repositories.Runs == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.RunService, exports.Repositories.Runs, nil
}

func validateRunDefinitions(ctx context.Context, session *RuntimeSession, workspaceID string, goalID string, dodID string) error {
	if goalID == "" && dodID == "" {
		return nil
	}
	if goalID == "" || dodID == "" {
		return common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return err
	}
	if exports.Repositories.Workspaces == nil || exports.Repositories.Goals == nil || exports.Repositories.DoDs == nil {
		return common.ErrInvalidArgument
	}
	if _, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID)); err != nil {
		return err
	}
	goal, err := exports.Repositories.Goals.Get(ctx, common.GoalID(goalID))
	if err != nil {
		return err
	}
	if goal.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	dod, err := exports.Repositories.DoDs.Get(ctx, common.DoDID(dodID))
	if err != nil {
		return err
	}
	if dod.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	if dod.State != common.DoDStateReady && dod.State != common.DoDStateSatisfied {
		return common.ErrConflict
	}
	return nil
}

func cloneContextMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func mapRun(item *runmodel.Run) Run {
	if item == nil {
		return Run{}
	}
	return Run{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		GoalID:             string(item.GoalID),
		DoDID:              string(item.DoDID),
		InitialStepKind:    item.InitialStepKind,
		ExecutionContext:   cloneContextMap(item.ExecutionContext),
		State:              string(item.State),
		Priority:           item.Priority,
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
}
