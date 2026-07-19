package workspace

import (
	"context"

	workspacebiz "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type CreateWorkspaceCommand = workspacecontracts.CreateWorkspaceCommand
type EnsureWorkspaceCommand = workspacecontracts.EnsureWorkspaceCommand
type DefineGoalCommand = workspacecontracts.DefineGoalCommand
type UpdateGoalCommand = workspacecontracts.UpdateGoalCommand
type DefineDoDCommand = workspacecontracts.DefineDoDCommand
type UpdateDoDCommand = workspacecontracts.UpdateDoDCommand
type Dependencies = workspacebiz.Dependencies

type Module struct{ bizPtr *workspacebiz.Biz }

func New() *Module                                  { return &Module{} }
func (m *Module) ID() string                        { return ModuleID }
func (m *Module) Configure(deps Dependencies) error { return m.configure(deps) }

func (m *Module) configure(deps Dependencies) error {
	if m == nil || deps.EventHub == nil || deps.Workspaces == nil || deps.Goals == nil || deps.DoDs == nil || deps.Templates == nil {
		return common.ErrInvalidArgument
	}
	bizPtr, err := workspacebiz.New(deps.EventHub, deps.BackgroundRoutine, deps)
	if err != nil {
		return err
	}
	m.bizPtr = bizPtr
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start()
}

func (m *Module) Stop(ctx context.Context) error {
	if m != nil && m.bizPtr != nil {
		m.bizPtr.Teardown(ctx)
		m.bizPtr = nil
	}
	return nil
}
