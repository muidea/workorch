package workspace

import (
	"context"

	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type CreateWorkspaceCommand = workspaceservice.CreateWorkspaceCommand
type EnsureWorkspaceCommand = workspaceservice.EnsureWorkspaceCommand
type DefineGoalCommand = workspaceservice.DefineGoalCommand
type UpdateGoalCommand = workspaceservice.UpdateGoalCommand
type DefineDoDCommand = workspaceservice.DefineDoDCommand
type UpdateDoDCommand = workspaceservice.UpdateDoDCommand
type Service = workspaceservice.Service

type Dependencies struct {
	Workspaces repository.WorkspaceRepository
	Goals      repository.GoalRepository
	DoDs       repository.DoDRepository
	EventHub   events.Hub
}

type Module struct {
	bizPtr     *workspacebiz.GoalDoDManager
	servicePtr *workspaceservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Workspaces == nil || deps.Goals == nil || deps.DoDs == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = workspacebiz.NewGoalDoDManager(deps.Workspaces, deps.Goals, deps.DoDs, deps.EventHub)
	m.servicePtr = workspaceservice.New(workspaceservice.Dependencies{
		CreateWorkspace: m.bizPtr.CreateWorkspace,
		EnsureWorkspace: m.bizPtr.EnsureWorkspace,
		DefineGoal:      m.bizPtr.DefineGoal,
		UpdateGoal:      m.bizPtr.UpdateGoal,
		DefineDoD:       m.bizPtr.DefineDoD,
		UpdateDoD:       m.bizPtr.UpdateDoD,
	})
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}
