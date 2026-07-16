package orchestrator

import (
	"context"
	"errors"
	"strings"

	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runStateSubsystem struct {
	runs       repository.RunRepository
	workspaces repository.WorkspaceRepository
}

func newRunStateSubsystem(runs repository.RunRepository, workspaces repository.WorkspaceRepository) *runStateSubsystem {
	return &runStateSubsystem{runs: runs, workspaces: workspaces}
}

func (s *runStateSubsystem) canTerminalRunReleaseWorkspace(ctx context.Context, workspace *workspaceModel.Workspace, run *runModel.Run) (bool, error) {
	if workspace == nil || run == nil || workspace.ID != run.WorkspaceID {
		return false, nil
	}
	if workspace.ActiveRunID == "" || workspace.ActiveRunID == run.ID {
		return true, nil
	}
	if s == nil || s.runs == nil {
		return false, common.ErrInvalidArgument
	}
	activeRun, err := s.runs.GetByWorkspaceID(ctx, workspace.ID, workspace.ActiveRunID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	if activeRun.WorkspaceID != workspace.ID {
		return true, nil
	}
	return isTerminalRunState(activeRun.State), nil
}

func (s *runStateSubsystem) releaseWorkspaceForTerminalRun(ctx context.Context, run *runModel.Run, state common.WorkspaceState) (*workspaceModel.Workspace, bool, error) {
	if s == nil || s.workspaces == nil || run == nil {
		return nil, false, common.ErrInvalidArgument
	}
	workspace, err := s.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	canRelease, err := s.canTerminalRunReleaseWorkspace(ctx, workspace, run)
	if err != nil || !canRelease {
		return workspace, false, err
	}
	workspace.ActiveRunID = ""
	workspace.State = common.CanonicalWorkspaceOwnerState(state)
	if err := s.workspaces.Update(ctx, workspace); err != nil {
		return nil, false, err
	}
	return workspace, true, nil
}

func (s *runStateSubsystem) syncRunAndWorkspaceForActiveState(ctx context.Context, run *runModel.Run, state common.WorkspaceState, workspace *workspaceModel.Workspace) (*workspaceModel.Workspace, error) {
	if !workspaceStateCompatibleWithRun(run, state) {
		return nil, common.ErrConflict
	}
	workspace, err := s.workspaceForActiveRunState(ctx, run, workspace)
	if err != nil {
		return nil, err
	}
	if run.State == common.RunStateActive && strings.TrimSpace(run.StartedAt) == "" {
		run.StartedAt = runtimeEvidenceTimestamp()
	}
	workspace.ActiveRunID = run.ID
	workspace.State = common.CanonicalWorkspaceOwnerState(state)
	if err := s.workspaces.Update(ctx, workspace); err != nil {
		return nil, err
	}
	if err := s.runs.Update(ctx, run); err != nil {
		return nil, err
	}
	return workspace, nil
}

func workspaceStateCompatibleWithRun(run *runModel.Run, state common.WorkspaceState) bool {
	if run == nil {
		return false
	}
	switch run.State {
	case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision:
		return common.CanonicalWorkspaceOwnerState(state) == common.WorkspaceStateActive
	case common.RunStateBlockedOnApproval:
		return common.CanonicalWorkspaceOwnerState(state) == common.WorkspaceStateBlockedOnApproval
	default:
		return false
	}
}

func (s *runStateSubsystem) workspaceForCreatedRunStart(ctx context.Context, run *runModel.Run) (*workspaceModel.Workspace, error) {
	if s == nil || s.runs == nil || s.workspaces == nil || run == nil {
		return nil, common.ErrInvalidArgument
	}
	workspace, err := s.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if workspace.ActiveRunID == "" || workspace.ActiveRunID == run.ID {
		return workspace, nil
	}
	activeRun, err := s.runs.GetByWorkspaceID(ctx, workspace.ID, workspace.ActiveRunID)
	if errors.Is(err, common.ErrNotFound) {
		return nil, common.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	if activeRun != nil && activeRun.WorkspaceID == workspace.ID && !isTerminalRunState(activeRun.State) {
		return nil, common.ErrConflict
	}
	return workspace, nil
}

func (s *runStateSubsystem) workspaceForActiveRunState(ctx context.Context, run *runModel.Run, workspace *workspaceModel.Workspace) (*workspaceModel.Workspace, error) {
	if s == nil || s.runs == nil || s.workspaces == nil || run == nil {
		return nil, common.ErrInvalidArgument
	}
	if workspace == nil {
		var err error
		workspace, err = s.workspaces.Get(ctx, run.WorkspaceID)
		if err != nil {
			return nil, err
		}
	} else if workspace.ID != run.WorkspaceID {
		return nil, common.ErrConflict
	}
	if workspace.ActiveRunID == "" || workspace.ActiveRunID == run.ID {
		return workspace, nil
	}
	activeRun, err := s.runs.GetByWorkspaceID(ctx, workspace.ID, workspace.ActiveRunID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}
	if activeRun != nil && activeRun.WorkspaceID == workspace.ID && !isTerminalRunState(activeRun.State) {
		return nil, common.ErrConflict
	}
	return workspace, nil
}
