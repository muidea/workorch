package legacy

import (
	"context"

	legacybiz "github.com/muidea/workorch/internal/modules/blocks/legacy/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	legacyservice "github.com/muidea/workorch/internal/modules/blocks/legacy/service"
)

const ModuleID = modulecommon.ModuleID

type Module struct {
	runtimeAdapter *legacybiz.MinimalRuntimeAdapter
	toolAdapter    *legacyservice.MinimalToolAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup() error {
	m.runtimeAdapter = legacybiz.NewMinimalRuntimeAdapter()
	m.toolAdapter = legacyservice.NewMinimalToolAdapter()
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	m.toolAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *legacybiz.MinimalRuntimeAdapter {
	return m.runtimeAdapter
}

func (m *Module) ToolAdapter() *legacyservice.MinimalToolAdapter {
	return m.toolAdapter
}
