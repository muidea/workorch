package mcp

import (
	"context"

	mcpbiz "github.com/muidea/workorch/internal/modules/kernel/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/common"
	mcpcontracts "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/contracts"
	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = mcpservice.Service
type CreateServerCommand = mcpcontracts.CreateServerCommand
type UpdateServerCommand = mcpcontracts.UpdateServerCommand

type Dependencies struct {
	Servers      repository.MCPServerRepository
	Capabilities repository.MCPCapabilityRepository
	EventHub     events.Hub
}

type Module struct {
	bizPtr     *mcpbiz.Service
	servicePtr *mcpservice.Module
	eventHub   events.Hub
	unsub      []func()
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
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandCreateServer, m.handleMCPCommand),
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandGetServer, m.handleMCPCommand),
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandListServers, m.handleMCPCommand),
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandUpdateServer, m.handleMCPCommand),
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandCreateCapability, m.handleMCPCommand),
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandGetCapability, m.handleMCPCommand),
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandListCapabilities, m.handleMCPCommand),
		m.eventHub.Subscribe(mcpevents.TopicMCPCommandUpdateCapability, m.handleMCPCommand),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
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

func (m *Module) handleMCPCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case mcpevents.TopicMCPCommandCreateServer:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(mcpcontracts.CreateServerCommand)
		if !ok {
			var err error
			cmd, err = serverCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := m.servicePtr.CreateServer(ctx, cmd); err != nil {
			return err
		}
		server, err := m.servicePtr.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyServer: server})
	case mcpevents.TopicMCPCommandGetServer:
		server, err := m.servicePtr.GetServer(ctx, common.MCPServerID(stringPayload(envelope.Payload, events.PayloadKeyServerID)))
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyServer: server})
	case mcpevents.TopicMCPCommandListServers:
		servers, err := m.servicePtr.ListServers(ctx)
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyServers: servers})
	case mcpevents.TopicMCPCommandUpdateServer:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(mcpcontracts.UpdateServerCommand)
		if !ok {
			var err error
			cmd, err = serverCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := m.servicePtr.UpdateServer(ctx, cmd); err != nil {
			return err
		}
		server, err := m.servicePtr.GetServer(ctx, cmd.ServerID)
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyServer: server})
	case mcpevents.TopicMCPCommandCreateCapability:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(mcpcontracts.CreateCapabilityCommand)
		if !ok {
			var err error
			cmd, err = capabilityCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		item, err := m.servicePtr.CreateCapability(ctx, cmd)
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyCapability: item})
	case mcpevents.TopicMCPCommandGetCapability:
		item, err := m.servicePtr.GetCapability(ctx, common.MCPCapabilityID(stringPayload(envelope.Payload, events.PayloadKeyCapabilityID)))
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyCapability: item})
	case mcpevents.TopicMCPCommandListCapabilities:
		query := capabilityQueryFromPayload(envelope.Payload)
		items, err := m.servicePtr.ListCapabilities(ctx, query)
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyCapabilities: items})
	case mcpevents.TopicMCPCommandUpdateCapability:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(mcpcontracts.UpdateCapabilityCommand)
		if !ok {
			var err error
			cmd, err = capabilityCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		item, err := m.servicePtr.UpdateCapability(ctx, cmd)
		if err != nil {
			return err
		}
		return mcpCommandResponse(envelope, map[string]any{events.PayloadKeyCapability: item})
	default:
		return common.ErrInvalidArgument
	}
}

func mcpCommandResponse(request events.Envelope, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	return events.Response(events.Envelope{
		Topic:        request.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  request.SubjectType,
		SubjectID:    request.SubjectID,
		WorkspaceID:  request.WorkspaceID,
		RunID:        request.RunID,
		StepID:       request.StepID,
		Payload:      payload,
	})
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func stringSlicePayload(payload map[string]any, key string) []string {
	switch value := payload[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		ret := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func stringMapPayload(payload map[string]any, key string) map[string]string {
	switch value := payload[key].(type) {
	case map[string]string:
		ret := make(map[string]string, len(value))
		for key, item := range value {
			ret[key] = item
		}
		return ret
	case map[string]any:
		ret := make(map[string]string, len(value))
		for key, item := range value {
			if text, ok := item.(string); ok {
				ret[key] = text
			}
		}
		return ret
	default:
		return nil
	}
}

func mapPayload(payload map[string]any, key string) map[string]any {
	value, _ := payload[key].(map[string]any)
	if value == nil {
		return nil
	}
	ret := make(map[string]any, len(value))
	for key, item := range value {
		ret[key] = item
	}
	return ret
}

func boolPtrPayload(payload map[string]any, key string) *bool {
	value, ok := payload[key].(bool)
	if !ok {
		return nil
	}
	return &value
}

func serverCommandFromPayload(payload map[string]any) (mcpcontracts.CreateServerCommand, error) {
	item, ok := payload[events.PayloadKeyServer].(*mcpmodel.Server)
	if ok && item != nil {
		return mcpcontracts.CreateServerCommand{
			ServerID:              item.ID,
			Name:                  item.Name,
			Transport:             item.Transport,
			Endpoint:              item.Endpoint,
			Command:               item.Command,
			Args:                  append([]string(nil), item.Args...),
			Env:                   cloneStringMap(item.Env),
			Enabled:               item.Enabled,
			AllowedTools:          append([]string(nil), item.AllowedTools...),
			AllowedResources:      append([]string(nil), item.AllowedResources...),
			AllowedPrompts:        append([]string(nil), item.AllowedPrompts...),
			DefaultApprovalPolicy: item.DefaultApprovalPolicy,
			SessionPolicy:         item.SessionPolicy,
			TimeoutMillis:         item.TimeoutMillis,
		}, nil
	}
	id := common.MCPServerID(stringPayload(payload, events.PayloadKeyServerID))
	if id == "" {
		return mcpcontracts.CreateServerCommand{}, common.ErrInvalidArgument
	}
	return mcpcontracts.CreateServerCommand{
		ServerID:              id,
		Name:                  stringPayload(payload, events.PayloadKeyName),
		Transport:             stringPayload(payload, events.PayloadKeyKind),
		Endpoint:              stringPayload(payload, events.PayloadKeyURI),
		Command:               stringPayload(payload, events.PayloadKeyCommand),
		Args:                  stringSlicePayload(payload, events.PayloadKeyItems),
		Env:                   stringMapPayload(payload, events.PayloadKeyPayload),
		Enabled:               boolValuePayload(payload, events.PayloadKeyEnabled),
		AllowedTools:          stringSlicePayload(payload, events.PayloadKeyToolKinds),
		AllowedResources:      stringSlicePayload(payload, events.PayloadKeyCapabilityRefs),
		AllowedPrompts:        stringSlicePayload(payload, events.PayloadKeySkillRefs),
		DefaultApprovalPolicy: stringPayload(payload, events.PayloadKeyPolicy),
		SessionPolicy:         stringPayload(payload, events.PayloadKeyStatus),
	}, nil
}

func capabilityCommandFromPayload(payload map[string]any) (mcpcontracts.CreateCapabilityCommand, error) {
	item, ok := payload[events.PayloadKeyCapability].(*mcpmodel.Capability)
	if !ok || item == nil {
		return mcpcontracts.CreateCapabilityCommand{}, common.ErrInvalidArgument
	}
	return mcpcontracts.CreateCapabilityCommand{
		ID:             item.ID,
		ServerID:       item.ServerID,
		Kind:           item.Kind,
		Name:           item.Name,
		DisplayName:    item.DisplayName,
		Description:    item.Description,
		Category:       item.Category,
		Tags:           append([]string(nil), item.Tags...),
		UsageHint:      item.UsageHint,
		InputSummary:   item.InputSummary,
		OutputSummary:  item.OutputSummary,
		StageHints:     append([]string(nil), item.StageHints...),
		RiskLevel:      item.RiskLevel,
		Schema:         cloneMap(item.Schema),
		URI:            item.URI,
		Enabled:        item.Enabled,
		ApprovalPolicy: item.ApprovalPolicy,
		Status:         item.Status,
	}, nil
}

func capabilityQueryFromPayload(payload map[string]any) mcpcontracts.CapabilityQuery {
	if typed, ok := payload[events.PayloadKeyCapabilityQuery].(mcpcontracts.CapabilityQuery); ok {
		return typed
	}
	nested, _ := payload[events.PayloadKeyCapabilityQuery].(map[string]any)
	if nested == nil {
		nested = payload
	}
	return mcpcontracts.CapabilityQuery{
		ServerID:  common.MCPServerID(stringPayload(nested, events.PayloadKeyServerID)),
		Kind:      stringPayload(nested, events.PayloadKeyKind),
		Category:  stringPayload(nested, events.PayloadKeyCategory),
		Stage:     stringPayload(nested, events.PayloadKeyStage),
		RiskLevel: stringPayload(nested, events.PayloadKeyRisk),
		Enabled:   boolPtrPayload(nested, events.PayloadKeyEnabled),
		Query:     stringPayload(nested, events.PayloadKeyQuery),
	}
}

func boolValuePayload(payload map[string]any, key string) bool {
	value, _ := payload[key].(bool)
	return value
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
