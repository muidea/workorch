package workspace

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateWorkspaceCommand struct {
	WorkspaceID      common.WorkspaceID
	Title            string
	Description      string
	WorkingDirectory string
}

type EnsureWorkspaceCommand struct {
	WorkspaceID      common.WorkspaceID
	Title            string
	Description      string
	WorkingDirectory string
}

type Service interface {
	CreateWorkspace(context.Context, CreateWorkspaceCommand) error
	EnsureWorkspace(context.Context, EnsureWorkspaceCommand) error
}

type CreateWorkspaceHandler func(context.Context, CreateWorkspaceCommand) error
type EnsureWorkspaceHandler func(context.Context, EnsureWorkspaceCommand) error

type Module struct {
	createWorkspace CreateWorkspaceHandler
	ensureWorkspace EnsureWorkspaceHandler
}

type Dependencies struct {
	CreateWorkspace CreateWorkspaceHandler
	EnsureWorkspace EnsureWorkspaceHandler
}

func New(deps Dependencies) *Module {
	return &Module{
		createWorkspace: deps.CreateWorkspace,
		ensureWorkspace: deps.EnsureWorkspace,
	}
}

func (m *Module) CreateWorkspace(ctx context.Context, cmd CreateWorkspaceCommand) error {
	if m == nil || m.createWorkspace == nil {
		return common.ErrInvalidArgument
	}
	return m.createWorkspace(ctx, cmd)
}

func (m *Module) EnsureWorkspace(ctx context.Context, cmd EnsureWorkspaceCommand) error {
	if m == nil || m.ensureWorkspace == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureWorkspace(ctx, cmd)
}
