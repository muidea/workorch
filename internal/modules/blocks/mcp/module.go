package mcp

import (
	"context"

	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindToolCall = modulecommon.StepKindToolCall

type Dependencies struct {
	MCPService mcpservice.Service
}

type Module struct {
	runtimeAdapter *mcpbiz.RuntimeAdapter
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.MCPService == nil {
		return common.ErrInvalidArgument
	}
	m.runtimeAdapter = mcpbiz.NewRuntimeAdapter(deps.MCPService)
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.runtimeAdapter = nil
	return nil
}

func (m *Module) RuntimeAdapter() *mcpbiz.RuntimeAdapter {
	return m.runtimeAdapter
}
