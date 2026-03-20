package workspace

import (
	"context"

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
	if cmd.Title == "" {
		return common.ErrInvalidArgument
	}
	_ = ctx
	return nil
}
