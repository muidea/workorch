package soul

import (
	"context"

	soulbiz "github.com/muidea/workorch/internal/modules/kernel/soul/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/common"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	soulservice "github.com/muidea/workorch/internal/modules/kernel/soul/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = soulservice.Service
type CreateSoulCommand = soulservice.CreateSoulCommand
type UpdateSoulCommand = soulservice.UpdateSoulCommand
type ValidateCapabilitySelectionCommand = soulservice.ValidateCapabilitySelectionCommand

type Dependencies struct {
	Souls    repository.SoulRepository
	EventHub events.Hub
}

type Module struct {
	bizPtr     *soulbiz.Service
	servicePtr *soulservice.Module
	eventHub   events.Hub
	unsub      []func()
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Souls == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.bizPtr = soulbiz.NewService(deps.Souls)
	m.servicePtr = soulservice.New(soulservice.Dependencies{
		CreateSoul:                  m.bizPtr.CreateSoul,
		UpdateSoul:                  m.bizPtr.UpdateSoul,
		DeleteSoul:                  m.bizPtr.DeleteSoul,
		GetSoul:                     m.bizPtr.GetSoul,
		ListSouls:                   m.bizPtr.ListSouls,
		ValidateCapabilitySelection: m.bizPtr.ValidateCapabilitySelection,
	})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if m.eventHub == nil {
		return nil
	}
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(events.TopicSoulCommandCreate, m.handleSoulCommand),
		m.eventHub.Subscribe(events.TopicSoulCommandGet, m.handleSoulCommand),
		m.eventHub.Subscribe(events.TopicSoulCommandList, m.handleSoulCommand),
		m.eventHub.Subscribe(events.TopicSoulCommandUpdate, m.handleSoulCommand),
		m.eventHub.Subscribe(events.TopicSoulCommandDelete, m.handleSoulCommand),
		m.eventHub.Subscribe(events.TopicSoulCommandValidateCapability, m.handleSoulCommand),
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
	return m.servicePtr
}

func (m *Module) handleSoulCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicSoulCommandCreate:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(soulservice.CreateSoulCommand)
		if !ok {
			var err error
			cmd, err = soulCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := m.servicePtr.CreateSoul(ctx, cmd); err != nil {
			return err
		}
		item, err := m.servicePtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return err
		}
		return soulCommandResponse(envelope, map[string]any{events.PayloadKeySoul: item})
	case events.TopicSoulCommandGet:
		item, err := m.servicePtr.GetSoul(ctx, common.SoulID(stringPayload(envelope.Payload, events.PayloadKeySoulID)))
		if err != nil {
			return err
		}
		return soulCommandResponse(envelope, map[string]any{events.PayloadKeySoul: item})
	case events.TopicSoulCommandList:
		items, err := m.servicePtr.ListSouls(ctx)
		if err != nil {
			return err
		}
		return soulCommandResponse(envelope, map[string]any{events.PayloadKeySouls: items})
	case events.TopicSoulCommandUpdate:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(soulservice.UpdateSoulCommand)
		if !ok {
			var err error
			cmd, err = soulCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := m.servicePtr.UpdateSoul(ctx, cmd); err != nil {
			return err
		}
		item, err := m.servicePtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return err
		}
		return soulCommandResponse(envelope, map[string]any{events.PayloadKeySoul: item})
	case events.TopicSoulCommandDelete:
		if err := m.servicePtr.DeleteSoul(ctx, common.SoulID(stringPayload(envelope.Payload, events.PayloadKeySoulID))); err != nil {
			return err
		}
		return soulCommandResponse(envelope, nil)
	case events.TopicSoulCommandValidateCapability:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(soulservice.ValidateCapabilitySelectionCommand)
		if !ok {
			cmd = soulCapabilitySelectionCommandFromPayload(envelope.Payload)
			if cmd.SoulID == "" {
				return common.ErrInvalidArgument
			}
		}
		if err := m.servicePtr.ValidateCapabilitySelection(ctx, cmd); err != nil {
			return err
		}
		return soulCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func soulCommandResponse(request events.Envelope, payload map[string]any) error {
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

func soulCommandFromPayload(payload map[string]any) (soulservice.CreateSoulCommand, error) {
	item, ok := payload[events.PayloadKeySoul].(*soulmodel.Soul)
	if !ok || item == nil {
		return soulservice.CreateSoulCommand{}, common.ErrInvalidArgument
	}
	return soulservice.CreateSoulCommand{
		SoulID:                      item.ID,
		Name:                        item.Name,
		Description:                 item.Description,
		BusinessDomains:             append([]string(nil), item.BusinessDomains...),
		SoulMD:                      item.SoulMD,
		AllowedSkillRefs:            append([]string(nil), item.AllowedSkillRefs...),
		AllowedMCPServerRefs:        append([]string(nil), item.AllowedMCPServerRefs...),
		AllowedCapabilityRefs:       append([]string(nil), item.AllowedCapabilityRefs...),
		AllowedCapabilityCategories: append([]string(nil), item.AllowedCapabilityCategories...),
		AllowedToolKinds:            append([]string(nil), item.AllowedToolKinds...),
	}, nil
}

func soulCapabilitySelectionCommandFromPayload(payload map[string]any) soulservice.ValidateCapabilitySelectionCommand {
	return soulservice.ValidateCapabilitySelectionCommand{
		SoulID:               common.SoulID(stringPayload(payload, events.PayloadKeySoulID)),
		SkillRefs:            stringSlicePayload(payload, events.PayloadKeySkillRefs),
		MCPServerRefs:        stringSlicePayload(payload, events.PayloadKeyMCPServerRefs),
		CapabilityRefs:       stringSlicePayload(payload, events.PayloadKeyCapabilityRefs),
		CapabilityCategories: stringSlicePayload(payload, events.PayloadKeyCapabilityCategories),
		ToolKinds:            stringSlicePayload(payload, events.PayloadKeyToolKinds),
	}
}
