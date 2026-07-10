package skill

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	skillbiz "github.com/muidea/workorch/internal/modules/kernel/skill/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/common"
	skillcontracts "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/contracts"
	skillevents "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/events"
	skillservice "github.com/muidea/workorch/internal/modules/kernel/skill/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = skillservice.Service
type CreateSkillCommand = skillcontracts.CreateSkillCommand
type UpdateSkillCommand = skillcontracts.UpdateSkillCommand

type Dependencies struct {
	Skills   repository.SkillRepository
	EventHub magicevent.Hub
}

type Module struct {
	bizPtr     *skillbiz.Service
	servicePtr *skillservice.Module
	eventHub   magicevent.Hub
	observer   magicevent.SimpleObserver
	topics     []string
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
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		skillevents.TopicSkillCommandCreate,
		skillevents.TopicSkillCommandGet,
		skillevents.TopicSkillCommandList,
		skillevents.TopicSkillCommandUpdate,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleSkillCommand)
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

func (m *Module) handleSkillCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeSkillCommand(ev.Context(), ev.ID(), ev.Data())
	setMagicResult(result, response, err)
}

func (m *Module) executeSkillCommand(ctx context.Context, topic string, data any) (skillevents.SkillCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case skillevents.TopicSkillCommandCreate:
		cmd, ok := data.(skillevents.CreateSkillCommand)
		if !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateSkill(ctx, cmd); err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		item, err := m.servicePtr.GetSkill(ctx, cmd.SkillID)
		if err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		return skillevents.SkillCommandResult{Skill: item}, nil
	case skillevents.TopicSkillCommandGet:
		cmd, ok := data.(skillevents.GetSkillCommand)
		if !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.servicePtr.GetSkill(ctx, cmd.SkillID)
		if err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		return skillevents.SkillCommandResult{Skill: item}, nil
	case skillevents.TopicSkillCommandList:
		if _, ok := data.(skillevents.ListSkillsCommand); !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.servicePtr.ListSkills(ctx)
		if err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		return skillevents.SkillCommandResult{Skills: items}, nil
	case skillevents.TopicSkillCommandUpdate:
		cmd, ok := data.(skillevents.UpdateSkillCommand)
		if !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.UpdateSkill(ctx, cmd); err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		item, err := m.servicePtr.GetSkill(ctx, cmd.SkillID)
		if err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		return skillevents.SkillCommandResult{Skill: item}, nil
	default:
		return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
	}
}

func setMagicResult(result magicevent.Result, data skillevents.SkillCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
