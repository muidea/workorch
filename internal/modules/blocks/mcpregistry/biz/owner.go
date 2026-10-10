package mcp

import (
	"context"
	"sync"

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

const (
	applicationContextRegistrySource = "application/workorchd/context-registry"
	applicationOwnerReadSource       = "application/workorchd/appservice"
	applicationReadModelSource       = "application/workorchd/readmodel"
	mcpExecutionSource               = "blocks/mcp"
)

type Dependencies struct {
	Registry          repository.MCPRegistryRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service    *service
	topics     []string
	started    bool
	lifecycle  basebiz.Lifecycle
	shutdownMu sync.Mutex
	releasing  bool
	released   bool
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
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	defer leave()
	if m.started {
		return nil
	}
	if len(m.topics) != 0 {
		return common.ErrConflict
	}
	topics := []string{
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
	for _, topic := range topics {
		if err := m.subscribeOwner(topic, m.handleCommand); err != nil {
			return err
		}
	}
	m.started = true
	return nil
}

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.BeginShutdown(ctx)
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	if m.releasing {
		m.shutdownMu.Unlock()
		return common.ErrConflict
	}
	m.releasing = true
	m.shutdownMu.Unlock()
	defer func() { m.shutdownMu.Lock(); m.releasing = false; m.shutdownMu.Unlock() }()
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	for len(m.topics) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.topics) - 1
		if err := m.UnsubscribeFuncChecked(m.topics[last]); err != nil {
			return err
		}
		m.topics = m.topics[:last]
	}
	m.service = nil
	m.started = false
	m.shutdownMu.Lock()
	m.released = true
	m.shutdownMu.Unlock()
	return nil
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
	if !mcpRegistryCommandSourceAllowed(ev.ID(), ev.Source()) {
		setMCPRegistryCommandInvalidResult(ev, result)
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

func mcpRegistryCommandSourceAllowed(topic, source string) bool {
	switch topic {
	case mcpevents.TopicMCPCommandCreateServer,
		mcpevents.TopicMCPCommandUpdateServer,
		mcpevents.TopicMCPCommandCreateCapability,
		mcpevents.TopicMCPCommandUpdateCapability:
		return source == applicationContextRegistrySource
	case mcpevents.TopicMCPCommandListServers:
		return source == applicationContextRegistrySource
	case mcpevents.TopicMCPCommandGetServer:
		return source == applicationContextRegistrySource || source == applicationOwnerReadSource || source == mcpExecutionSource
	case mcpevents.TopicMCPCommandListCapabilities:
		return source == applicationContextRegistrySource || source == applicationReadModelSource
	case mcpevents.TopicMCPCommandGetCapability:
		return source == applicationContextRegistrySource || source == applicationOwnerReadSource
	case mcpevents.TopicMCPCommandReadRegistrySnapshot:
		return source == mcpExecutionSource
	case mcpevents.TopicMCPCommandReadSelectionSnapshot:
		return source == applicationOwnerReadSource
	case mcpevents.TopicMCPCommandEnsureDefaults:
		return source == systemCommandSource
	default:
		return false
	}
}

func setMCPRegistryCommandInvalidResult(ev magicevent.Event, result magicevent.Result) {
	topic := ""
	if ev != nil {
		topic = ev.ID()
	}
	switch topic {
	case mcpevents.TopicMCPCommandCreateServer:
		setMagicResult(result, mcpevents.CreateServerResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandGetServer:
		setMagicResult(result, mcpevents.GetServerResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandListServers:
		setMagicResult(result, mcpevents.ListServersResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandUpdateServer:
		setMagicResult(result, mcpevents.UpdateServerResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandCreateCapability:
		setMagicResult(result, mcpevents.CreateCapabilityResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandGetCapability:
		setMagicResult(result, mcpevents.GetCapabilityResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandListCapabilities:
		setMagicResult(result, mcpevents.ListCapabilitiesResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandUpdateCapability:
		setMagicResult(result, mcpevents.UpdateCapabilityResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandReadRegistrySnapshot:
		setMagicResult(result, mcpevents.RegistrySnapshotResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandReadSelectionSnapshot:
		setMagicResult(result, mcpevents.SelectionSnapshotResult{}, common.ErrInvalidArgument)
	case mcpevents.TopicMCPCommandEnsureDefaults:
		setMagicResult(result, mcpevents.EnsureDefaultsResult{}, common.ErrInvalidArgument)
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
