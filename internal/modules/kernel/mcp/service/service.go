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
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

type UpdateServerCommand = CreateServerCommand

type CapabilityQuery struct {
	ServerID  common.MCPServerID
	Kind      string
	Category  string
	Stage     string
	RiskLevel string
	Enabled   *bool
	Query     string
}

type CreateCapabilityCommand struct {
	ID             common.MCPCapabilityID
	ServerID       common.MCPServerID
	Kind           string
	Name           string
	DisplayName    string
	Description    string
	Category       string
	Tags           []string
	UsageHint      string
	InputSummary   string
	OutputSummary  string
	StageHints     []string
	RiskLevel      string
	Schema         map[string]any
	URI            string
	Enabled        bool
	ApprovalPolicy string
	Status         string
}

type UpdateCapabilityCommand = CreateCapabilityCommand

type Service interface {
	CreateServer(context.Context, CreateServerCommand) error
	UpdateServer(context.Context, UpdateServerCommand) error
	GetServer(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
	ListServers(context.Context) ([]*mcpmodel.Server, error)
	CreateCapability(context.Context, CreateCapabilityCommand) (*mcpmodel.Capability, error)
	UpdateCapability(context.Context, UpdateCapabilityCommand) (*mcpmodel.Capability, error)
	GetCapability(context.Context, common.MCPCapabilityID) (*mcpmodel.Capability, error)
	ListCapabilities(context.Context, CapabilityQuery) ([]*mcpmodel.Capability, error)
}

type Dependencies struct {
	CreateServer     func(context.Context, CreateServerCommand) error
	UpdateServer     func(context.Context, UpdateServerCommand) error
	GetServer        func(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
	ListServers      func(context.Context) ([]*mcpmodel.Server, error)
	CreateCapability func(context.Context, CreateCapabilityCommand) (*mcpmodel.Capability, error)
	UpdateCapability func(context.Context, UpdateCapabilityCommand) (*mcpmodel.Capability, error)
	GetCapability    func(context.Context, common.MCPCapabilityID) (*mcpmodel.Capability, error)
	ListCapabilities func(context.Context, CapabilityQuery) ([]*mcpmodel.Capability, error)
}

type Module struct {
	createServer     func(context.Context, CreateServerCommand) error
	updateServer     func(context.Context, UpdateServerCommand) error
	getServer        func(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
	listServers      func(context.Context) ([]*mcpmodel.Server, error)
	createCapability func(context.Context, CreateCapabilityCommand) (*mcpmodel.Capability, error)
	updateCapability func(context.Context, UpdateCapabilityCommand) (*mcpmodel.Capability, error)
	getCapability    func(context.Context, common.MCPCapabilityID) (*mcpmodel.Capability, error)
	listCapabilities func(context.Context, CapabilityQuery) ([]*mcpmodel.Capability, error)
}

func New(deps Dependencies) *Module {
	return &Module{
		createServer:     deps.CreateServer,
		updateServer:     deps.UpdateServer,
		getServer:        deps.GetServer,
		listServers:      deps.ListServers,
		createCapability: deps.CreateCapability,
		updateCapability: deps.UpdateCapability,
		getCapability:    deps.GetCapability,
		listCapabilities: deps.ListCapabilities,
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

func (m *Module) CreateCapability(ctx context.Context, cmd CreateCapabilityCommand) (*mcpmodel.Capability, error) {
	if m == nil || m.createCapability == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.createCapability(ctx, cmd)
}

func (m *Module) UpdateCapability(ctx context.Context, cmd UpdateCapabilityCommand) (*mcpmodel.Capability, error) {
	if m == nil || m.updateCapability == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.updateCapability(ctx, cmd)
}

func (m *Module) GetCapability(ctx context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	if m == nil || m.getCapability == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getCapability(ctx, id)
}

func (m *Module) ListCapabilities(ctx context.Context, query CapabilityQuery) ([]*mcpmodel.Capability, error) {
	if m == nil || m.listCapabilities == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listCapabilities(ctx, query)
}
