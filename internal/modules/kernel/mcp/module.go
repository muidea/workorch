package mcp

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	mcpbiz "github.com/muidea/workorch/internal/modules/kernel/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/common"
	mcpcontracts "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/contracts"
	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = mcpservice.Service
type CreateServerCommand = mcpcontracts.CreateServerCommand
type UpdateServerCommand = mcpcontracts.UpdateServerCommand

type Dependencies struct {
	Servers      repository.MCPServerRepository
	Capabilities repository.MCPCapabilityRepository
	EventHub     magicevent.Hub
}

type Module struct {
	bizPtr     *mcpbiz.Service
	servicePtr *mcpservice.Module
	eventHub   magicevent.Hub
	observer   magicevent.SimpleObserver
	topics     []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Servers == nil || deps.Capabilities == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.bizPtr = mcpbiz.NewService(deps.Servers, deps.Capabilities)
	m.servicePtr = mcpservice.New(mcpservice.Dependencies{
		CreateServer:     m.bizPtr.CreateServer,
		UpdateServer:     m.bizPtr.UpdateServer,
		GetServer:        m.bizPtr.GetServer,
		ListServers:      m.bizPtr.ListServers,
		CreateCapability: m.bizPtr.CreateCapability,
		UpdateCapability: m.bizPtr.UpdateCapability,
		GetCapability:    m.bizPtr.GetCapability,
		ListCapabilities: m.bizPtr.ListCapabilities,
	})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		mcpevents.TopicMCPCommandCreateServer,
		mcpevents.TopicMCPCommandGetServer,
		mcpevents.TopicMCPCommandListServers,
		mcpevents.TopicMCPCommandUpdateServer,
		mcpevents.TopicMCPCommandCreateCapability,
		mcpevents.TopicMCPCommandGetCapability,
		mcpevents.TopicMCPCommandListCapabilities,
		mcpevents.TopicMCPCommandUpdateCapability,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleMCPCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	if m == nil || m.servicePtr == nil {
		return nil
	}
	return m.servicePtr
}

func (m *Module) handleMCPCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeMCPCommand(ev.Context(), ev.ID(), ev.Data())
	setMagicResult(result, response, err)
}

func (m *Module) executeMCPCommand(ctx context.Context, topic string, data any) (mcpevents.MCPCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case mcpevents.TopicMCPCommandCreateServer:
		cmd, ok := data.(mcpevents.CreateServerCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateServer(ctx, cmd); err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		server, err := m.servicePtr.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Server: server}, nil
	case mcpevents.TopicMCPCommandGetServer:
		cmd, ok := data.(mcpevents.GetServerCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		server, err := m.servicePtr.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Server: server}, nil
	case mcpevents.TopicMCPCommandListServers:
		if _, ok := data.(mcpevents.ListServersCommand); !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		servers, err := m.servicePtr.ListServers(ctx)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Servers: servers}, nil
	case mcpevents.TopicMCPCommandUpdateServer:
		cmd, ok := data.(mcpevents.UpdateServerCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.UpdateServer(ctx, cmd); err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		server, err := m.servicePtr.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Server: server}, nil
	case mcpevents.TopicMCPCommandCreateCapability:
		cmd, ok := data.(mcpevents.CreateCapabilityCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.servicePtr.CreateCapability(ctx, cmd)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capability: item}, nil
	case mcpevents.TopicMCPCommandGetCapability:
		cmd, ok := data.(mcpevents.GetCapabilityCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.servicePtr.GetCapability(ctx, cmd.CapabilityID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capability: item}, nil
	case mcpevents.TopicMCPCommandListCapabilities:
		cmd, ok := data.(mcpevents.ListCapabilitiesCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.servicePtr.ListCapabilities(ctx, cmd.Query)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capabilities: items}, nil
	case mcpevents.TopicMCPCommandUpdateCapability:
		cmd, ok := data.(mcpevents.UpdateCapabilityCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.servicePtr.UpdateCapability(ctx, cmd)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capability: item}, nil
	default:
		return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
	}
}

func setMagicResult(result magicevent.Result, data any, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
