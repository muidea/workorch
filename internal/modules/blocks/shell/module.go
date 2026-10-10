package shell

import (
	"context"

	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Module struct {
	bizPtr *shellbiz.Biz
}

func New() *Module           { return &Module{} }
func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps ...shellbiz.Dependencies) error {
	return m.configure(deps...)
}

func (m *Module) configure(deps ...shellbiz.Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if len(deps) == 0 || deps[0].EventHub == nil {
		m.bizPtr = nil
		return nil
	}
	configured, err := shellbiz.New(deps[0].EventHub, deps[0].BackgroundRoutine, deps...)
	if err != nil {
		return err
	}
	m.bizPtr = configured
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start()
}

func (m *Module) Stop(ctx context.Context) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if m != nil && m.bizPtr != nil {
		if err := m.bizPtr.Teardown(ctx); err != nil {
			return err
		}
		m.bizPtr = nil
	}
	return nil
}
