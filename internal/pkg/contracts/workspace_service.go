package contracts

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
	Title       string
}

type WorkspaceService interface {
	CreateWorkspace(context.Context, CreateWorkspaceCommand) error
}
