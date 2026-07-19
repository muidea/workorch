package system

import (
	"context"

	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/system/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Dependencies = systembiz.Dependencies

type Module struct {
	bizPtr *systembiz.System
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	return m.configure(deps)
}

func (m *Module) Start(ctx context.Context) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start(ctx)
}

func (m *Module) Stop(_ context.Context) error {
	if m != nil {
		m.bizPtr = nil
	}
	return nil
}

func (m *Module) configure(deps Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	configured, err := systembiz.NewWithDependencies(deps)
	if err != nil {
		return err
	}
	m.bizPtr = configured
	return nil
}
