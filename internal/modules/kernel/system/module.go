package system

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/system/pkg/common"
	systemevents "github.com/muidea/workorch/internal/modules/kernel/system/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	EventHub magicevent.Hub
}

type Module struct {
	eventHub magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if err := m.ensureSoulDefaults(ctx); err != nil {
		return err
	}
	if err := m.ensureWorkspaceTemplateDefaults(ctx); err != nil {
		return err
	}
	return m.ensureMCPDefaults(ctx)
}

func (m *Module) Stop(_ context.Context) error {
	m.eventHub = nil
	return nil
}

func (m *Module) ensureSoulDefaults(ctx context.Context) error {
	ev := magicevent.NewEventWithContext(systemevents.TopicSystemCommandEnsureSoulDefaults, ModuleID, systemevents.SoulDefaultsDestination, magicevent.NewHeader(), ctx, systemevents.EnsureSoulDefaultsCommand{})
	result := m.eventHub.Send(ev)
	if result == nil {
		return fmt.Errorf("system soul defaults command returned no result")
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return err.Cause
		}
		return err
	}
	data, err := result.Get()
	if err != nil {
		if err.Cause != nil {
			return err.Cause
		}
		return err
	}
	response, ok := data.(systemevents.EnsureSoulDefaultsResult)
	if !ok || !response.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}

func (m *Module) ensureWorkspaceTemplateDefaults(ctx context.Context) error {
	ev := magicevent.NewEventWithContext(systemevents.TopicSystemCommandEnsureWorkspaceTemplateDefaults, ModuleID, systemevents.WorkspaceTemplateDefaultsDestination, magicevent.NewHeader(), ctx, systemevents.EnsureWorkspaceTemplateDefaultsCommand{})
	result := m.eventHub.Send(ev)
	if result == nil {
		return fmt.Errorf("system workspace template defaults command returned no result")
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return err.Cause
		}
		return err
	}
	data, err := result.Get()
	if err != nil {
		if err.Cause != nil {
			return err.Cause
		}
		return err
	}
	response, ok := data.(systemevents.EnsureWorkspaceTemplateDefaultsResult)
	if !ok || !response.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}

func (m *Module) ensureMCPDefaults(ctx context.Context) error {
	ev := magicevent.NewEventWithContext(systemevents.TopicSystemCommandEnsureMCPDefaults, ModuleID, systemevents.MCPDefaultsDestination, magicevent.NewHeader(), ctx, systemevents.EnsureMCPDefaultsCommand{})
	result := m.eventHub.Send(ev)
	if result == nil {
		return fmt.Errorf("system MCP defaults command returned no result")
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return err.Cause
		}
		return err
	}
	data, err := result.Get()
	if err != nil {
		if err.Cause != nil {
			return err.Cause
		}
		return err
	}
	response, ok := data.(systemevents.EnsureMCPDefaultsResult)
	if !ok || !response.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}
