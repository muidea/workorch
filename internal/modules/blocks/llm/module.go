package llm

import (
	"context"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

// Module is the framework lifecycle bridge. Provider operations, EventHub
// subscriptions, and request-audit notifications belong to biz.Biz.
type Module struct {
	bizPtr *llmbiz.Biz
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps ...llmbiz.Dependencies) error {
	return m.configure(deps...)
}

func (m *Module) configure(deps ...llmbiz.Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if m.bizPtr != nil {
		return common.ErrConflict
	}
	if len(deps) == 0 || deps[0].EventHub == nil {
		m.bizPtr = nil
		return common.ErrInvalidArgument
	}
	configured, err := llmbiz.New(deps[0].EventHub, deps[0].BackgroundRoutine, deps...)
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
	if m != nil && m.bizPtr != nil {
		if err := m.bizPtr.Teardown(ctx); err != nil {
			return err
		}
		m.bizPtr = nil
	}
	return nil
}
