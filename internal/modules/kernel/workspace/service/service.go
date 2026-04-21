package workspace

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
	Title       string
}

type Service interface {
	CreateWorkspace(context.Context, CreateWorkspaceCommand) error
}
