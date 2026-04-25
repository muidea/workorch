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

type DefineGoalCommand struct {
	GoalID      common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}

type DefineDoDCommand struct {
	DoDID       common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
}

type Service interface {
	CreateWorkspace(context.Context, CreateWorkspaceCommand) error
	EnsureWorkspace(context.Context, EnsureWorkspaceCommand) error
	DefineGoal(context.Context, DefineGoalCommand) error
	DefineDoD(context.Context, DefineDoDCommand) error
}

type CreateWorkspaceHandler func(context.Context, CreateWorkspaceCommand) error
type EnsureWorkspaceHandler func(context.Context, EnsureWorkspaceCommand) error
type DefineGoalHandler func(context.Context, DefineGoalCommand) error
type DefineDoDHandler func(context.Context, DefineDoDCommand) error

type Module struct {
	createWorkspace CreateWorkspaceHandler
	ensureWorkspace EnsureWorkspaceHandler
	defineGoal      DefineGoalHandler
	defineDoD       DefineDoDHandler
}

type Dependencies struct {
	CreateWorkspace CreateWorkspaceHandler
	EnsureWorkspace EnsureWorkspaceHandler
	DefineGoal      DefineGoalHandler
	DefineDoD       DefineDoDHandler
}

func New(deps Dependencies) *Module {
	return &Module{
		createWorkspace: deps.CreateWorkspace,
		ensureWorkspace: deps.EnsureWorkspace,
		defineGoal:      deps.DefineGoal,
		defineDoD:       deps.DefineDoD,
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

func (m *Module) DefineGoal(ctx context.Context, cmd DefineGoalCommand) error {
	if m == nil || m.defineGoal == nil {
		return common.ErrInvalidArgument
	}
	return m.defineGoal(ctx, cmd)
}

func (m *Module) DefineDoD(ctx context.Context, cmd DefineDoDCommand) error {
	if m == nil || m.defineDoD == nil {
		return common.ErrInvalidArgument
	}
	return m.defineDoD(ctx, cmd)
}
