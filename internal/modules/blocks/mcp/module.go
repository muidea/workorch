package mcp

import (
	"context"

	magictask "github.com/muidea/magicCommon/task"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const ModuleID = modulecommon.ModuleID
const StepKindToolCall = modulecommon.StepKindToolCall
const StepKindResourceRead = modulecommon.StepKindResourceRead
const StepKindPromptGet = modulecommon.StepKindPromptGet

type Dependencies struct {
	EventHub   events.Hub
	Background magictask.BackgroundRoutine
}

type Module struct {
	runtimeAdapter *mcpbiz.RuntimeAdapter
	executor       *runtimeevent.Executor
	eventHub       events.Hub
	background     magictask.BackgroundRoutine
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.background = deps.Background
	m.runtimeAdapter = mcpbiz.NewRuntimeAdapter(mcpEventServerGetter{hub: deps.EventHub})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil || m.background == nil {
		return common.ErrInvalidArgument
	}
	if m.executor != nil {
		return nil
	}
	m.executor = runtimeevent.NewExecutor(ModuleID, m.eventHub, m.background, m.runtimeAdapter, StepKindToolCall, StepKindResourceRead, StepKindPromptGet)
	return m.executor.Run()
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.executor != nil {
		m.executor.Teardown()
	}
	m.executor = nil
	if m.runtimeAdapter != nil {
		m.runtimeAdapter.Close()
	}
	m.runtimeAdapter = nil
	m.eventHub = nil
	m.background = nil
	return nil
}

type mcpEventServerGetter struct {
	hub events.Hub
}

func (g mcpEventServerGetter) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	err := g.hub.Publish(ctx, events.Envelope{
		Topic:        mcpevents.TopicMCPCommandGetServer,
		SourceModule: ModuleID,
		SubjectType:  "mcp_server",
		SubjectID:    string(id),
		Payload: map[string]any{
			events.PayloadKeyServerID: string(id),
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		server, ok := response.Payload[events.PayloadKeyServer].(*mcpmodel.Server)
		if !ok || server == nil {
			return nil, common.ErrNotFound
		}
		return server, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, common.ErrNotFound
}
