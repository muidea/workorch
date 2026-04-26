package llm

import (
	"context"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

type Module struct {
	runtimeAdapter *llmbiz.RuntimeAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup() error {
	adapter, err := llmbiz.NewRuntimeAdapterFromEnv()
	if err != nil {
		return err
	}
	m.runtimeAdapter = adapter
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *llmbiz.RuntimeAdapter {
	return m.runtimeAdapter
}
