package workspace

import (
	"context"

	workspacebiz "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Module struct{ bizPtr *workspacebiz.Biz }

func New() *Module                                               { return &Module{} }
func (m *Module) ID() string                                     { return ModuleID }
func (m *Module) Configure(deps workspacebiz.Dependencies) error { return m.configure(deps) }

func (m *Module) configure(deps workspacebiz.Dependencies) error {
	if m == nil || deps.EventHub == nil || deps.Workspaces == nil || deps.DeletionMaterialCleaner == nil || deps.Goals == nil || deps.DoDs == nil || deps.Templates == nil {
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
