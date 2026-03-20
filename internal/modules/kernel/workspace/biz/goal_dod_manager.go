package workspace

import (
	"context"

	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type GoalDoDManager struct {
	workspaces repository.WorkspaceRepository
}

func NewGoalDoDManager(workspaces repository.WorkspaceRepository) *GoalDoDManager {
	return &GoalDoDManager{workspaces: workspaces}
}

func (m *GoalDoDManager) CreateWorkspace(ctx context.Context, cmd contracts.CreateWorkspaceCommand) error {
	if cmd.WorkspaceID == "" || cmd.Title == "" {
		return common.ErrInvalidArgument
	}

	workspace := &workspaceModel.Workspace{
		ID:    cmd.WorkspaceID,
		Title: cmd.Title,
		State: common.WorkspaceStateActive,
	}
	return m.workspaces.Create(ctx, workspace)
}
