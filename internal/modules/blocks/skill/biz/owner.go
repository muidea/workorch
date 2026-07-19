package skill

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	skillcontracts "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/contracts"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type CreateSkillCommand = skillcontracts.CreateSkillCommand
type UpdateSkillCommand = skillcontracts.UpdateSkillCommand

type Dependencies struct {
	Skills            repository.SkillRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service *Service
	topics  []string
	started bool
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Skills == nil {
		return nil, common.ErrInvalidArgument
	}
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine), service: NewService(deps.Skills)}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = []string{
		skillevents.TopicSkillCommandCreate,
		skillevents.TopicSkillCommandGet,
		skillevents.TopicSkillCommandList,
		skillevents.TopicSkillCommandUpdate,
	}
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleSkillCommand)
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

func (m *Biz) handleSkillCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeSkillCommand(ev)
	setMagicResult(result, response, err)
}

func (m *Biz) executeSkillCommand(ev magicevent.Event) (skillevents.SkillCommandResult, error) {
	if m == nil || m.service == nil {
		return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case skillevents.TopicSkillCommandCreate:
		cmd, ok := ev.Data().(skillevents.CreateSkillCommand)
		if !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.CreateSkill(ctx, cmd); err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		item, err := m.service.GetSkill(ctx, cmd.SkillID)
		if err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		return skillevents.SkillCommandResult{Skill: item}, nil
	case skillevents.TopicSkillCommandGet:
		cmd, ok := ev.Data().(skillevents.GetSkillCommand)
		if !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.service.GetSkill(ctx, cmd.SkillID)
		if err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		return skillevents.SkillCommandResult{Skill: item}, nil
	case skillevents.TopicSkillCommandList:
		if _, ok := ev.Data().(skillevents.ListSkillsCommand); !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.service.ListSkills(ctx)
		if err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		return skillevents.SkillCommandResult{Skills: items}, nil
	case skillevents.TopicSkillCommandUpdate:
		cmd, ok := ev.Data().(skillevents.UpdateSkillCommand)
		if !ok {
			return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.UpdateSkill(ctx, cmd); err != nil {
			return skillevents.SkillCommandResult{}, err
		}
		item, err := m.service.GetSkill(ctx, cmd.SkillID)
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
