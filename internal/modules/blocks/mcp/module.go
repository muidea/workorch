package mcp

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindToolCall = modulecommon.StepKindToolCall
const StepKindResourceRead = modulecommon.StepKindResourceRead
const StepKindPromptGet = modulecommon.StepKindPromptGet

type Dependencies struct {
	EventHub magicevent.Hub
}

type Module struct {
	runtimeAdapter     *mcpbiz.RuntimeAdapter
	capabilityObserver magicevent.SimpleObserver
	eventHub           magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.runtimeAdapter = mcpbiz.NewRuntimeAdapter(mcpEventServerGetter{hub: deps.EventHub})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.capabilityObserver != nil {
		return nil
	}
	m.capabilityObserver = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteRuntimeOperation, m.handleRuntimeOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteRuntimeOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	}
	m.capabilityObserver = nil
	if m.runtimeAdapter != nil {
		m.runtimeAdapter.Close()
	}
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleRuntimeOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteRuntimeOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ExecuteRuntimeOperation(event.Context(), command, m.runtimeAdapter)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Module) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindToolCall, StepKindResourceRead, StepKindPromptGet)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}

type mcpEventServerGetter struct {
	hub magicevent.Hub
}

func (g mcpEventServerGetter) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandGetServer,
		ModuleID,
		mcpevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		mcpevents.GetServerCommand{ServerID: id},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, err := result.Get()
	if err != nil {
		return nil, err
	}
	response, ok := data.(mcpevents.MCPCommandResult)
	if !ok || response.Server == nil {
		return nil, common.ErrNotFound
	}
	return response.Server, nil
}
