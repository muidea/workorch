package skill

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Skills            repository.SkillRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service *service
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
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine), service: newService(deps.Skills)}, nil
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
	// All Skill topics are synchronous commands. In particular, the create
	// command must never mutate state when accidentally delivered through Post.
	if result == nil {
		return
	}
	if m == nil || m.service == nil {
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	ctx := ev.Context()
	switch ev.ID() {
	case skillevents.TopicSkillCommandCreate:
		cmd, ok := ev.Data().(skillevents.CreateSkillCommand)
		if !ok {
			setMagicResult(result, skillevents.CreateSkillResult{}, common.ErrInvalidArgument)
			return
		}
		item, err := m.service.createSkill(ctx, cmd)
		if err != nil {
			setMagicResult(result, skillevents.CreateSkillResult{}, err)
			return
		}
		setMagicResult(result, (skillevents.CreateSkillResult{Skill: item}).Clone(), nil)
	case skillevents.TopicSkillCommandGet:
		cmd, ok := ev.Data().(skillevents.GetSkillCommand)
		if !ok {
			setMagicResult(result, skillevents.GetSkillResult{}, common.ErrInvalidArgument)
			return
		}
		item, err := m.service.getSkill(ctx, cmd.SkillID)
		if err != nil {
			setMagicResult(result, skillevents.GetSkillResult{}, err)
			return
		}
		setMagicResult(result, (skillevents.GetSkillResult{Skill: item}).Clone(), nil)
	case skillevents.TopicSkillCommandList:
		cmd, ok := ev.Data().(skillevents.ListSkillsCommand)
		if !ok {
			setMagicResult(result, skillevents.ListSkillsResult{}, common.ErrInvalidArgument)
			return
		}
		items, nextCursor, err := m.service.listSkills(ctx, cmd.CursorAfterID, cmd.Limit)
		if err != nil {
			setMagicResult(result, skillevents.ListSkillsResult{}, err)
			return
		}
		setMagicResult(result, (skillevents.ListSkillsResult{Skills: items, NextCursor: nextCursor}).Clone(), nil)
	default:
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func setMagicResult[Response any](result magicevent.Result, data Response, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
