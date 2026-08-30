package mcp

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

const systemCommandSource = "kernel/system"

type Dependencies struct {
	Registry          repository.MCPRegistryRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service *service
	topics  []string
	started bool
}

func New(eventHub magicevent.Hub, background task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Registry == nil || deps.EventHub == nil || eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, background), service: newService(deps.Registry)}, nil
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
		mcpevents.TopicMCPCommandReadRegistrySnapshot,
		mcpevents.TopicMCPCommandReadSelectionSnapshot,
		mcpevents.TopicMCPCommandEnsureDefaults,
	}
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
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
	m.started = false
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	// Every Registry topic is a synchronous command/query. Post must never
	// mutate strong owner state without a result channel.
	if result == nil {
		return
	}
	if m == nil || m.service == nil {
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	if ev == nil {
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	if ev.ID() == mcpevents.TopicMCPCommandEnsureDefaults && ev.Source() != systemCommandSource {
		setMagicResult(result, mcpevents.EnsureDefaultsResult{}, common.ErrInvalidArgument)
		return
	}
	ctx := ev.Context()
	switch ev.ID() {
	case mcpevents.TopicMCPCommandCreateServer:
		cmd, ok := ev.Data().(mcpevents.CreateServerCommand)
		if !ok {
			setMagicResult(result, mcpevents.CreateServerResult{}, common.ErrInvalidArgument)
			return
		}
		server, revision, err := m.service.createServer(ctx, cmd)
		view, viewErr := mcpevents.MCPServerFromModel(server)
		setMagicResult(result, mcpevents.CreateServerResult{Server: view, RegistryRevision: revision}, firstError(err, viewErr))
	case mcpevents.TopicMCPCommandGetServer:
		cmd, ok := ev.Data().(mcpevents.GetServerCommand)
		if !ok {
			setMagicResult(result, mcpevents.GetServerResult{}, common.ErrInvalidArgument)
			return
		}
		server, revision, err := m.service.getServer(ctx, cmd.ServerID)
		view, viewErr := mcpevents.MCPServerFromModel(server)
		setMagicResult(result, mcpevents.GetServerResult{Server: view, RegistryRevision: revision}, firstError(err, viewErr))
	case mcpevents.TopicMCPCommandListServers:
		cmd, ok := ev.Data().(mcpevents.ListServersCommand)
		if !ok {
			setMagicResult(result, mcpevents.ListServersResult{}, common.ErrInvalidArgument)
			return
		}
		servers, cursor, revision, err := m.service.listServers(ctx, cmd.CursorAfterID, cmd.Limit)
		views, viewErr := mcpevents.MCPServersFromModels(servers)
		setMagicResult(result, mcpevents.ListServersResult{Servers: views, NextCursor: cursor, RegistryRevision: revision}, firstError(err, viewErr))
	case mcpevents.TopicMCPCommandUpdateServer:
		cmd, ok := ev.Data().(mcpevents.UpdateServerCommand)
		if !ok {
			setMagicResult(result, mcpevents.UpdateServerResult{}, common.ErrInvalidArgument)
			return
		}
		server, revision, err := m.service.updateServer(ctx, cmd)
		view, viewErr := mcpevents.MCPServerFromModel(server)
		setMagicResult(result, mcpevents.UpdateServerResult{Server: view, RegistryRevision: revision}, firstError(err, viewErr))
	case mcpevents.TopicMCPCommandCreateCapability:
		cmd, ok := ev.Data().(mcpevents.CreateCapabilityCommand)
		if !ok {
			setMagicResult(result, mcpevents.CreateCapabilityResult{}, common.ErrInvalidArgument)
			return
		}
		item, revision, err := m.service.createCapability(ctx, cmd)
		setMagicResult(result, mcpevents.CreateCapabilityResult{Capability: mcpmodel.CloneCapability(item), RegistryRevision: revision}, err)
	case mcpevents.TopicMCPCommandGetCapability:
		cmd, ok := ev.Data().(mcpevents.GetCapabilityCommand)
		if !ok {
			setMagicResult(result, mcpevents.GetCapabilityResult{}, common.ErrInvalidArgument)
			return
		}
		item, revision, err := m.service.getCapability(ctx, cmd.CapabilityID)
		setMagicResult(result, mcpevents.GetCapabilityResult{Capability: mcpmodel.CloneCapability(item), RegistryRevision: revision}, err)
	case mcpevents.TopicMCPCommandListCapabilities:
		cmd, ok := ev.Data().(mcpevents.ListCapabilitiesCommand)
		if !ok {
			setMagicResult(result, mcpevents.ListCapabilitiesResult{}, common.ErrInvalidArgument)
			return
		}
		items, cursor, revision, err := m.service.listCapabilities(ctx, cmd.Query, cmd.CursorAfterID, cmd.Limit)
		setMagicResult(result, mcpevents.ListCapabilitiesResult{Capabilities: mcpmodel.CloneCapabilities(items), NextCursor: cursor, RegistryRevision: revision}, err)
	case mcpevents.TopicMCPCommandUpdateCapability:
		cmd, ok := ev.Data().(mcpevents.UpdateCapabilityCommand)
		if !ok {
			setMagicResult(result, mcpevents.UpdateCapabilityResult{}, common.ErrInvalidArgument)
			return
		}
		item, revision, err := m.service.updateCapability(ctx, cmd)
		setMagicResult(result, mcpevents.UpdateCapabilityResult{Capability: mcpmodel.CloneCapability(item), RegistryRevision: revision}, err)
	case mcpevents.TopicMCPCommandReadRegistrySnapshot:
		if _, ok := ev.Data().(mcpevents.ReadRegistrySnapshotCommand); !ok {
			setMagicResult(result, mcpevents.RegistrySnapshotResult{}, common.ErrInvalidArgument)
			return
		}
		snapshot, revision, err := m.service.registrySnapshot(ctx)
		setRegistrySnapshotResult(result, snapshot, revision, err)
	case mcpevents.TopicMCPCommandReadSelectionSnapshot:
		cmd, ok := ev.Data().(mcpevents.ReadSelectionSnapshotCommand)
		if !ok {
			setMagicResult(result, mcpevents.SelectionSnapshotResult{}, common.ErrInvalidArgument)
			return
		}
		snapshot, revision, err := m.service.selectionSnapshot(ctx, cmd.ServerIDs, cmd.CapabilityIDs)
		setSelectionSnapshotResult(result, snapshot, revision, err)
	case mcpevents.TopicMCPCommandEnsureDefaults:
		if _, ok := ev.Data().(mcpevents.EnsureDefaultsCommand); !ok {
			setMagicResult(result, mcpevents.EnsureDefaultsResult{}, common.ErrInvalidArgument)
			return
		}
		revision, err := m.service.ensureBuiltinDefaults(ctx)
		setMagicResult(result, mcpevents.EnsureDefaultsResult{Ensured: err == nil, RegistryRevision: revision}, err)
	default:
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func setRegistrySnapshotResult(result magicevent.Result, snapshot *mcpmodel.RegistrySnapshot, revision string, err error) {
	if err != nil {
		setMagicResult(result, mcpevents.RegistrySnapshotResult{}, err)
		return
	}
	servers, viewErr := mcpevents.MCPServersFromModels(snapshot.Servers)
	setMagicResult(result, mcpevents.RegistrySnapshotResult{Servers: servers, Capabilities: mcpmodel.CloneCapabilities(snapshot.Capabilities), RegistryRevision: revision}, viewErr)
}

func setSelectionSnapshotResult(result magicevent.Result, snapshot *mcpmodel.RegistrySnapshot, revision string, err error) {
	if err != nil {
		setMagicResult(result, mcpevents.SelectionSnapshotResult{}, err)
		return
	}
	servers, viewErr := mcpevents.MCPServersFromModels(snapshot.Servers)
	setMagicResult(result, mcpevents.SelectionSnapshotResult{Servers: servers, Capabilities: mcpmodel.CloneCapabilities(snapshot.Capabilities), RegistryRevision: revision}, viewErr)
}

func setMagicResult[Result any](result magicevent.Result, data Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func firstError(left, right error) error {
	if left != nil {
		return left
	}
	return right
}
