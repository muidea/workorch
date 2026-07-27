package repository

import (
	"context"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceRepository interface {
	Create(context.Context, *workspace.Workspace) error
	Get(context.Context, common.WorkspaceID) (*workspace.Workspace, error)
	List(context.Context) ([]*workspace.Workspace, error)
	Update(context.Context, *workspace.Workspace) error
	CompareAndSwap(context.Context, *workspace.Workspace, uint64) error
	Delete(context.Context, common.WorkspaceID) error
}
