package mcp

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpcontracts "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/contracts"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type CreateServerCommand = mcpcontracts.CreateServerCommand
type UpdateServerCommand = mcpcontracts.UpdateServerCommand

type Dependencies struct {
	Servers           repository.MCPServerRepository
	Capabilities      repository.MCPCapabilityRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service      *Service
	servers      repository.MCPServerRepository
	capabilities repository.MCPCapabilityRepository
	topics       []string
	started      bool
}

func New(eventHub magicevent.Hub, background task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Servers == nil || deps.Capabilities == nil || deps.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, background), service: NewService(deps.Servers, deps.Capabilities), servers: deps.Servers, capabilities: deps.Capabilities}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
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
		m.SubscribeFunc(topic, m.handleMCPCommand)
	}
	m.topics = append(m.topics, mcpevents.TopicMCPCommandEnsureDefaults)
	m.SubscribeFunc(mcpevents.TopicMCPCommandEnsureDefaults, m.handleSystemDefaultsCommand)
	m.started = true
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.service = nil
	m.servers = nil
	m.capabilities = nil
	m.started = false
}

func (m *Biz) handleSystemDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	if _, ok := ev.Data().(mcpevents.EnsureDefaultsCommand); !ok || m == nil || m.servers == nil || m.capabilities == nil {
		setSystemMCPDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	setSystemMCPDefaultsResult(result, ensureBuiltinMCPDefaults(ev.Context(), m.servers, m.capabilities))
}

func setSystemMCPDefaultsResult(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(mcpevents.EnsureDefaultsResult{Ensured: true}, nil)
}

func (m *Biz) handleMCPCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeMCPCommand(ev)
	setMagicResult(result, response, err)
}

func (m *Biz) executeMCPCommand(ev magicevent.Event) (mcpevents.MCPCommandResult, error) {
	if m == nil || m.service == nil {
		return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case mcpevents.TopicMCPCommandCreateServer:
		cmd, ok := ev.Data().(mcpevents.CreateServerCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.CreateServer(ctx, cmd); err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		server, err := m.service.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Server: server}, nil
	case mcpevents.TopicMCPCommandGetServer:
		cmd, ok := ev.Data().(mcpevents.GetServerCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		server, err := m.service.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Server: server}, nil
	case mcpevents.TopicMCPCommandListServers:
		if _, ok := ev.Data().(mcpevents.ListServersCommand); !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		servers, err := m.service.ListServers(ctx)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Servers: servers}, nil
	case mcpevents.TopicMCPCommandUpdateServer:
		cmd, ok := ev.Data().(mcpevents.UpdateServerCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.UpdateServer(ctx, cmd); err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		server, err := m.service.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Server: server}, nil
	case mcpevents.TopicMCPCommandCreateCapability:
		cmd, ok := ev.Data().(mcpevents.CreateCapabilityCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.service.CreateCapability(ctx, mcpCapabilityContractCommand(cmd))
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capability: item}, nil
	case mcpevents.TopicMCPCommandGetCapability:
		cmd, ok := ev.Data().(mcpevents.GetCapabilityCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.service.GetCapability(ctx, cmd.CapabilityID)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capability: item}, nil
	case mcpevents.TopicMCPCommandListCapabilities:
		cmd, ok := ev.Data().(mcpevents.ListCapabilitiesCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.service.ListCapabilities(ctx, cmd.Query)
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capabilities: items}, nil
	case mcpevents.TopicMCPCommandUpdateCapability:
		cmd, ok := ev.Data().(mcpevents.UpdateCapabilityCommand)
		if !ok {
			return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.service.UpdateCapability(ctx, mcpCapabilityContractCommand(cmd))
		if err != nil {
			return mcpevents.MCPCommandResult{}, err
		}
		return mcpevents.MCPCommandResult{Capability: item}, nil
	default:
		return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
	}
}

func mcpCapabilityContractCommand(cmd mcpevents.CreateCapabilityCommand) mcpcontracts.CreateCapabilityCommand {
	return mcpcontracts.CreateCapabilityCommand{
		ID: cmd.ID, ServerID: cmd.ServerID, Kind: cmd.Kind, Name: cmd.Name, DisplayName: cmd.DisplayName,
		Description: cmd.Description, Category: cmd.Category, Tags: append([]string(nil), cmd.Tags...),
		UsageHint: cmd.UsageHint, InputSummary: cmd.InputSummary, OutputSummary: cmd.OutputSummary,
		StageHints: append([]string(nil), cmd.StageHints...), RiskLevel: cmd.RiskLevel,
		EffectScope: cmd.EffectScope, IdempotencyClass: cmd.IdempotencyClass, Schema: cmd.Schema.Clone(),
		URI: cmd.URI, Enabled: cmd.Enabled, ApprovalPolicy: cmd.ApprovalPolicy, Status: cmd.Status,
	}
}

func setMagicResult(result magicevent.Result, data mcpevents.MCPCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
