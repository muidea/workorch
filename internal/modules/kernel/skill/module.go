package skill

import (
	"context"

	skillbiz "github.com/muidea/workorch/internal/modules/kernel/skill/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/common"
	skillcontracts "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/contracts"
	skillevents "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	skillservice "github.com/muidea/workorch/internal/modules/kernel/skill/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = skillservice.Service
type CreateSkillCommand = skillcontracts.CreateSkillCommand
type UpdateSkillCommand = skillcontracts.UpdateSkillCommand

type Dependencies struct {
	Skills   repository.SkillRepository
	EventHub events.Hub
}

type Module struct {
	bizPtr     *skillbiz.Service
	servicePtr *skillservice.Module
	eventHub   events.Hub
	unsub      []func()
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Skills == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.bizPtr = skillbiz.NewService(deps.Skills)
	m.servicePtr = skillservice.New(skillservice.Dependencies{
		CreateSkill: m.bizPtr.CreateSkill,
		UpdateSkill: m.bizPtr.UpdateSkill,
		GetSkill:    m.bizPtr.GetSkill,
		ListSkills:  m.bizPtr.ListSkills,
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
		m.eventHub.Subscribe(skillevents.TopicSkillCommandCreate, m.handleSkillCommand),
		m.eventHub.Subscribe(skillevents.TopicSkillCommandGet, m.handleSkillCommand),
		m.eventHub.Subscribe(skillevents.TopicSkillCommandList, m.handleSkillCommand),
		m.eventHub.Subscribe(skillevents.TopicSkillCommandUpdate, m.handleSkillCommand),
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

func (m *Module) handleSkillCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case skillevents.TopicSkillCommandCreate:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(skillcontracts.CreateSkillCommand)
		if !ok {
			var err error
			cmd, err = skillCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := m.servicePtr.CreateSkill(ctx, cmd); err != nil {
			return err
		}
		item, err := m.servicePtr.GetSkill(ctx, cmd.SkillID)
		if err != nil {
			return err
		}
		return skillCommandResponse(envelope, map[string]any{events.PayloadKeySkill: item})
	case skillevents.TopicSkillCommandGet:
		item, err := m.servicePtr.GetSkill(ctx, common.SkillID(stringPayload(envelope.Payload, events.PayloadKeySkillID)))
		if err != nil {
			return err
		}
		return skillCommandResponse(envelope, map[string]any{events.PayloadKeySkill: item})
	case skillevents.TopicSkillCommandList:
		items, err := m.servicePtr.ListSkills(ctx)
		if err != nil {
			return err
		}
		return skillCommandResponse(envelope, map[string]any{events.PayloadKeySkills: items})
	case skillevents.TopicSkillCommandUpdate:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(skillcontracts.UpdateSkillCommand)
		if !ok {
			var err error
			cmd, err = skillCommandFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := m.servicePtr.UpdateSkill(ctx, cmd); err != nil {
			return err
		}
		item, err := m.servicePtr.GetSkill(ctx, cmd.SkillID)
		if err != nil {
			return err
		}
		return skillCommandResponse(envelope, map[string]any{events.PayloadKeySkill: item})
	default:
		return common.ErrInvalidArgument
	}
}

func skillCommandResponse(request events.Envelope, payload map[string]any) error {
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

func skillCommandFromPayload(payload map[string]any) (skillcontracts.CreateSkillCommand, error) {
	item, ok := payload[events.PayloadKeySkill].(*skillmodel.Skill)
	if !ok || item == nil {
		return skillcontracts.CreateSkillCommand{}, common.ErrInvalidArgument
	}
	return skillcontracts.CreateSkillCommand{
		SkillID:                item.ID,
		Name:                   item.Name,
		Version:                item.Version,
		Description:            item.Description,
		TriggerRules:           append([]string(nil), item.TriggerRules...),
		Instruction:            item.Instruction,
		ReferenceRefs:          append([]string(nil), item.ReferenceRefs...),
		RequiredToolKinds:      append([]string(nil), item.RequiredToolKinds...),
		RequiredMCPServerRefs:  append([]string(nil), item.RequiredMCPServerRefs...),
		RequiredCapabilityRefs: append([]string(nil), item.RequiredCapabilityRefs...),
		Constraints:            append([]string(nil), item.Constraints...),
	}, nil
}
