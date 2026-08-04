package mcp

import (
	"context"

	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindToolCall = modulecommon.StepKindToolCall
const StepKindResourceRead = modulecommon.StepKindResourceRead
const StepKindPromptGet = modulecommon.StepKindPromptGet

// Module is the framework lifecycle bridge. MCP execution and all owner Hub
// interaction belong to biz.Biz.
type Module struct {
	bizPtr *mcpbiz.Biz
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps mcpbiz.Dependencies) error {
	return m.configure(deps)
}

func (m *Module) configure(deps mcpbiz.Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	configured, err := mcpbiz.NewWithDependencies(deps)
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
		m.bizPtr.Teardown(ctx)
		m.bizPtr = nil
	}
	return nil
}
