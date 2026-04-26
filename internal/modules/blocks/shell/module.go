package shell

import (
	"context"

	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Module struct {
	runtimeAdapter *shellbiz.RuntimeAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup() error {
	m.runtimeAdapter = shellbiz.NewRuntimeAdapter()
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *shellbiz.RuntimeAdapter {
	return m.runtimeAdapter
}
