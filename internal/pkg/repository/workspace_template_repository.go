package repository

import (
	"context"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

type WorkspaceTemplateRepository interface {
	Create(context.Context, *workspacemodel.WorkspaceTemplate) error
	Get(context.Context, string) (*workspacemodel.WorkspaceTemplate, error)
	List(context.Context) ([]*workspacemodel.WorkspaceTemplate, error)
	Update(context.Context, *workspacemodel.WorkspaceTemplate) error
	Delete(context.Context, string) error
}
