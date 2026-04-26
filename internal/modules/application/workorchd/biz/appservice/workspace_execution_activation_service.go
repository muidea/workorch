package appservice

import (
	"context"
	"errors"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ActivateWorkspaceExecutionCommand struct {
	GoalID string `json:"goalId,omitempty"`
	DoDID  string `json:"doDId,omitempty"`
}

type WorkspaceExecutionActivation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}

func ActivateWorkspaceExecutionWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (ret WorkspaceExecutionActivation, err error) {
	exports, err := session.Exports()
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if exports.Services.RunService == nil ||
		exports.Repositories.Workspaces == nil ||
		exports.Repositories.Goals == nil ||
		exports.Repositories.DoDs == nil ||
		exports.Repositories.Runs == nil {
		return WorkspaceExecutionActivation{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)

	workspace, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if workspace.ActiveRunID != "" {
		run, err := exports.Repositories.Runs.Get(ctx, workspace.ActiveRunID)
		if err != nil {
			return WorkspaceExecutionActivation{}, err
		}
		snapshot, err := readWorkspaceSnapshotFromExports(ctx, session, common.WorkspaceID(workspaceID))
		if err != nil {
			return WorkspaceExecutionActivation{}, err
		}
		return WorkspaceExecutionActivation{
			Workspace: snapshot.Workspace,
			Run:       mapRun(run),
			Snapshot:  snapshot,
		}, nil
	}

	goalID, dodID, err := selectWorkspaceExecutionDefinitions(ctx, session, common.WorkspaceID(workspaceID), cmd)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if err := validateRunDefinitions(ctx, session, workspaceID, string(goalID), string(dodID)); err != nil {
		return WorkspaceExecutionActivation{}, err
	}

	run, err := createAndStartWorkspaceExecutionRun(ctx, session, common.WorkspaceID(workspaceID), goalID, dodID)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	snapshot, err := readWorkspaceSnapshotFromExports(ctx, session, common.WorkspaceID(workspaceID))
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return WorkspaceExecutionActivation{
		Workspace: snapshot.Workspace,
		Run:       mapRun(run),
		Snapshot:  snapshot,
	}, nil
}

func ActivateWorkspaceExecutionWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
}

func ActivateWorkspaceExecutionFromEnv(ctx context.Context, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
}

func selectWorkspaceExecutionDefinitions(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, cmd ActivateWorkspaceExecutionCommand) (common.GoalID, common.DoDID, error) {
	exports, err := session.Exports()
	if err != nil {
		return "", "", err
	}

	goalID, err := selectWorkspaceExecutionGoal(ctx, exports.Repositories.Goals.ListByWorkspaceID, workspaceID, common.GoalID(cmd.GoalID))
	if err != nil {
		return "", "", err
	}
	dodID, err := selectWorkspaceExecutionDoD(ctx, exports.Repositories.DoDs.ListByWorkspaceID, workspaceID, common.DoDID(cmd.DoDID))
	if err != nil {
		return "", "", err
	}
	return goalID, dodID, nil
}

func selectWorkspaceExecutionGoal(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.Goal, error),
	workspaceID common.WorkspaceID,
	requestedID common.GoalID,
) (common.GoalID, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if requestedID != "" {
		for _, item := range items {
			if item.ID == requestedID {
				return requestedID, nil
			}
		}
		return "", common.ErrConflict
	}
	if len(items) == 0 {
		return "", common.ErrInvalidArgument
	}
	if len(items) > 1 {
		return "", common.ErrConflict
	}
	return items[0].ID, nil
}

func selectWorkspaceExecutionDoD(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error),
	workspaceID common.WorkspaceID,
	requestedID common.DoDID,
) (common.DoDID, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	readyItems := make([]*workspacemodel.DoD, 0, len(items))
	for _, item := range items {
		if item.State == common.DoDStateReady || item.State == common.DoDStateSatisfied {
			readyItems = append(readyItems, item)
		}
	}
	if requestedID != "" {
		for _, item := range readyItems {
			if item.ID == requestedID {
				return requestedID, nil
			}
		}
		return "", common.ErrConflict
	}
	if len(readyItems) == 0 {
		return "", common.ErrInvalidArgument
	}
	if len(readyItems) > 1 {
		return "", common.ErrConflict
	}
	return readyItems[0].ID, nil
}

func createAndStartWorkspaceExecutionRun(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) (*runmodel.Run, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		runID := common.RunID(generateBusinessID("run"))
		if err := exports.Services.RunService.CreateRun(ctx, runmodule.CreateRunCommand{
			RunID:           runID,
			WorkspaceID:     workspaceID,
			GoalID:          goalID,
			DoDID:           dodID,
			InitialStepKind: llmcommon.StepKindWorkspaceExecution,
			ExecutionContext: map[string]any{
				"activation": "workspace_execution",
				"goalId":     string(goalID),
				"doDId":      string(dodID),
			},
		}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, err
		}
		if err := exports.Services.RunService.EnsureStarted(ctx, runmodule.StartRunCommand{RunID: runID}); err != nil {
			return nil, err
		}
		return exports.Repositories.Runs.Get(ctx, runID)
	}
	return nil, common.ErrConflict
}

func readWorkspaceSnapshotFromExports(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID) (WorkspaceSnapshot, error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return readWorkspaceSnapshot(ctx, deps, workspaceID)
}
