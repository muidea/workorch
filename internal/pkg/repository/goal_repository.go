package repository

import (
	"context"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type GoalRepository interface {
	Create(context.Context, *workspace.Goal) error
	Get(context.Context, common.GoalID) (*workspace.Goal, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*workspace.Goal, error)
	Update(context.Context, *workspace.Goal) error
	Delete(context.Context, common.GoalID) error
}
