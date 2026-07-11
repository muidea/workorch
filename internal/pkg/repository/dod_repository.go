package repository

import (
	"context"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type DoDRepository interface {
	Create(context.Context, *workspace.DoD) error
	Get(context.Context, common.DoDID) (*workspace.DoD, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*workspace.DoD, error)
	Update(context.Context, *workspace.DoD) error
	Delete(context.Context, common.DoDID) error
}
