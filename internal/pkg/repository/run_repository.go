package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RunRepository interface {
	Create(context.Context, *run.Run) error
	Get(context.Context, common.RunID) (*run.Run, error)
	GetByWorkspaceID(context.Context, common.WorkspaceID, common.RunID) (*run.Run, error)
	List(context.Context) ([]*run.Run, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*run.Run, error)
	Update(context.Context, *run.Run) error
	Delete(context.Context, common.RunID) error
}
