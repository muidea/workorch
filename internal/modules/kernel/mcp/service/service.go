package mcp

import (
	"context"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateServerCommand struct {
	ServerID              common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	DefaultApprovalPolicy string
	TimeoutMillis         int
}

type UpdateServerCommand = CreateServerCommand

type Service interface {
	CreateServer(context.Context, CreateServerCommand) error
	UpdateServer(context.Context, UpdateServerCommand) error
	GetServer(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
	ListServers(context.Context) ([]*mcpmodel.Server, error)
}

type Dependencies struct {
	CreateServer func(context.Context, CreateServerCommand) error
	UpdateServer func(context.Context, UpdateServerCommand) error
	GetServer    func(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
	ListServers  func(context.Context) ([]*mcpmodel.Server, error)
}

type Module struct {
	createServer func(context.Context, CreateServerCommand) error
	updateServer func(context.Context, UpdateServerCommand) error
	getServer    func(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
	listServers  func(context.Context) ([]*mcpmodel.Server, error)
}

func New(deps Dependencies) *Module {
	return &Module{
		createServer: deps.CreateServer,
		updateServer: deps.UpdateServer,
		getServer:    deps.GetServer,
		listServers:  deps.ListServers,
	}
}

func (m *Module) CreateServer(ctx context.Context, cmd CreateServerCommand) error {
	if m == nil || m.createServer == nil {
		return common.ErrInvalidArgument
	}
	return m.createServer(ctx, cmd)
}

func (m *Module) UpdateServer(ctx context.Context, cmd UpdateServerCommand) error {
	if m == nil || m.updateServer == nil {
		return common.ErrInvalidArgument
	}
	return m.updateServer(ctx, cmd)
}

func (m *Module) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if m == nil || m.getServer == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getServer(ctx, id)
}

func (m *Module) ListServers(ctx context.Context) ([]*mcpmodel.Server, error) {
	if m == nil || m.listServers == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listServers(ctx)
}
