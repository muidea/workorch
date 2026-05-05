package file

import (
	"context"

	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindList = modulecommon.StepKindList
const StepKindRead = modulecommon.StepKindRead
const StepKindEdit = modulecommon.StepKindEdit

type Module struct {
	runtimeAdapter *filebiz.RuntimeAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup() error {
	m.runtimeAdapter = filebiz.NewRuntimeAdapter()
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *filebiz.RuntimeAdapter {
	return m.runtimeAdapter
}
