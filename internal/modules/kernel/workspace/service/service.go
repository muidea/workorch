package workspace

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
}

type EnsureWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
}

type DefineGoalCommand struct {
	GoalID      common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}

type UpdateGoalCommand struct {
	GoalID common.GoalID
	Text   string
}

type DefineDoDCommand struct {
	DoDID       common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
}

type UpdateDoDCommand struct {
	DoDID common.DoDID
	Items []string
}

type Service interface {
	CreateWorkspace(context.Context, CreateWorkspaceCommand) error
	EnsureWorkspace(context.Context, EnsureWorkspaceCommand) error
	DeleteWorkspace(context.Context, common.WorkspaceID) error
	DefineGoal(context.Context, DefineGoalCommand) error
	UpdateGoal(context.Context, UpdateGoalCommand) error
	DefineDoD(context.Context, DefineDoDCommand) error
	UpdateDoD(context.Context, UpdateDoDCommand) error
}

type CreateWorkspaceHandler func(context.Context, CreateWorkspaceCommand) error
type EnsureWorkspaceHandler func(context.Context, EnsureWorkspaceCommand) error
type DeleteWorkspaceHandler func(context.Context, common.WorkspaceID) error
type DefineGoalHandler func(context.Context, DefineGoalCommand) error
type UpdateGoalHandler func(context.Context, UpdateGoalCommand) error
type DefineDoDHandler func(context.Context, DefineDoDCommand) error
type UpdateDoDHandler func(context.Context, UpdateDoDCommand) error

type Module struct {
	createWorkspace CreateWorkspaceHandler
	ensureWorkspace EnsureWorkspaceHandler
	deleteWorkspace DeleteWorkspaceHandler
	defineGoal      DefineGoalHandler
	updateGoal      UpdateGoalHandler
	defineDoD       DefineDoDHandler
	updateDoD       UpdateDoDHandler
}

type Dependencies struct {
	CreateWorkspace CreateWorkspaceHandler
	EnsureWorkspace EnsureWorkspaceHandler
	DeleteWorkspace DeleteWorkspaceHandler
	DefineGoal      DefineGoalHandler
	UpdateGoal      UpdateGoalHandler
	DefineDoD       DefineDoDHandler
	UpdateDoD       UpdateDoDHandler
}

func New(deps Dependencies) *Module {
	return &Module{
		createWorkspace: deps.CreateWorkspace,
		ensureWorkspace: deps.EnsureWorkspace,
		deleteWorkspace: deps.DeleteWorkspace,
		defineGoal:      deps.DefineGoal,
		updateGoal:      deps.UpdateGoal,
		defineDoD:       deps.DefineDoD,
		updateDoD:       deps.UpdateDoD,
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

func (m *Module) DeleteWorkspace(ctx context.Context, id common.WorkspaceID) error {
	if m == nil || m.deleteWorkspace == nil {
		return common.ErrInvalidArgument
	}
	return m.deleteWorkspace(ctx, id)
}

func (m *Module) DefineGoal(ctx context.Context, cmd DefineGoalCommand) error {
	if m == nil || m.defineGoal == nil {
		return common.ErrInvalidArgument
	}
	return m.defineGoal(ctx, cmd)
}

func (m *Module) UpdateGoal(ctx context.Context, cmd UpdateGoalCommand) error {
	if m == nil || m.updateGoal == nil {
		return common.ErrInvalidArgument
	}
	return m.updateGoal(ctx, cmd)
}

func (m *Module) DefineDoD(ctx context.Context, cmd DefineDoDCommand) error {
	if m == nil || m.defineDoD == nil {
		return common.ErrInvalidArgument
	}
	return m.defineDoD(ctx, cmd)
}

func (m *Module) UpdateDoD(ctx context.Context, cmd UpdateDoDCommand) error {
	if m == nil || m.updateDoD == nil {
		return common.ErrInvalidArgument
	}
	return m.updateDoD(ctx, cmd)
}
