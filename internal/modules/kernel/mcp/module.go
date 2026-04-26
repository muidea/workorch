package mcp

import (
	"context"

	mcpbiz "github.com/muidea/workorch/internal/modules/kernel/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/common"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = mcpservice.Service
type CreateServerCommand = mcpservice.CreateServerCommand
type UpdateServerCommand = mcpservice.UpdateServerCommand

type Dependencies struct {
	Servers repository.MCPServerRepository
}

type Module struct {
	bizPtr     *mcpbiz.Service
	servicePtr *mcpservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Servers == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = mcpbiz.NewService(deps.Servers)
	m.servicePtr = mcpservice.New(mcpservice.Dependencies{
		CreateServer: m.bizPtr.CreateServer,
		UpdateServer: m.bizPtr.UpdateServer,
		GetServer:    m.bizPtr.GetServer,
		ListServers:  m.bizPtr.ListServers,
	})
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}
