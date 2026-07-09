package workspace

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type CreateWorkspaceCommand = workspacecontracts.CreateWorkspaceCommand
type EnsureWorkspaceCommand = workspacecontracts.EnsureWorkspaceCommand
type UpdateWorkspaceCommand = workspacecontracts.UpdateWorkspaceCommand
type DefineGoalCommand = workspacecontracts.DefineGoalCommand
type UpdateGoalCommand = workspacecontracts.UpdateGoalCommand
type DefineDoDCommand = workspacecontracts.DefineDoDCommand
type UpdateDoDCommand = workspacecontracts.UpdateDoDCommand
type Service = workspaceservice.Service

type Dependencies struct {
	Workspaces repository.WorkspaceRepository
	Goals      repository.GoalRepository
	DoDs       repository.DoDRepository
	Templates  repository.WorkspaceTemplateRepository
	EventHub   magicevent.Hub
}

type Module struct {
	bizPtr     *workspacebiz.GoalDoDManager
	servicePtr *workspaceservice.Module
	workspaces repository.WorkspaceRepository
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	templates  repository.WorkspaceTemplateRepository
	eventHub   magicevent.Hub
	observer   magicevent.SimpleObserver
	topics     []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Workspaces == nil || deps.Goals == nil || deps.DoDs == nil || deps.Templates == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.workspaces = deps.Workspaces
	m.goals = deps.Goals
	m.dods = deps.DoDs
	m.templates = deps.Templates
	m.eventHub = deps.EventHub
	m.bizPtr = workspacebiz.NewGoalDoDManager(deps.Workspaces, deps.Goals, deps.DoDs, events.NewMagicHubWithHubOwner(deps.EventHub, ModuleID))
	m.servicePtr = workspaceservice.New(workspaceservice.Dependencies{
		CreateWorkspace: m.bizPtr.CreateWorkspace,
		EnsureWorkspace: m.bizPtr.EnsureWorkspace,
		DeleteWorkspace: m.bizPtr.DeleteWorkspace,
		DefineGoal:      m.bizPtr.DefineGoal,
		UpdateGoal:      m.bizPtr.UpdateGoal,
		DefineDoD:       m.bizPtr.DefineDoD,
		UpdateDoD:       m.bizPtr.UpdateDoD,
	})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		workspaceevents.TopicWorkspaceCommandCreate,
		workspaceevents.TopicWorkspaceCommandEnsure,
		workspaceevents.TopicWorkspaceCommandGet,
		workspaceevents.TopicWorkspaceCommandList,
		workspaceevents.TopicWorkspaceCommandUpdate,
		workspaceevents.TopicWorkspaceCommandDelete,
		workspaceevents.TopicWorkspaceCommandPurge,
		workspaceevents.TopicWorkspaceCommandDefineGoal,
		workspaceevents.TopicWorkspaceCommandUpdateGoal,
		workspaceevents.TopicWorkspaceCommandDeleteGoal,
		workspaceevents.TopicWorkspaceCommandGetGoal,
		workspaceevents.TopicWorkspaceCommandListGoalsByWS,
		workspaceevents.TopicWorkspaceCommandDefineDoD,
		workspaceevents.TopicWorkspaceCommandUpdateDoD,
		workspaceevents.TopicWorkspaceCommandDeleteDoD,
		workspaceevents.TopicWorkspaceCommandGetDoD,
		workspaceevents.TopicWorkspaceCommandListDoDsByWS,
		workspaceevents.TopicWorkspaceTemplateCommandCreate,
		workspaceevents.TopicWorkspaceTemplateCommandGet,
		workspaceevents.TopicWorkspaceTemplateCommandList,
		workspaceevents.TopicWorkspaceTemplateCommandUpdate,
		workspaceevents.TopicWorkspaceTemplateCommandDelete,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
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
	m.workspaces = nil
	m.goals = nil
	m.dods = nil
	m.templates = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setWorkspaceMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (workspaceevents.WorkspaceCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	workspaceID, err := workspaceScopeFromCommand(topic, data)
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	if workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch topic {
	case workspaceevents.TopicWorkspaceCommandCreate:
		cmd, ok := data.(workspaceevents.CreateWorkspaceCommand)
		if !ok {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateWorkspace(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandEnsure:
		cmd, ok := data.(workspaceevents.EnsureWorkspaceCommand)
		if !ok {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.EnsureWorkspace(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGet:
		cmd, ok := data.(workspaceevents.GetWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.workspaces == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Workspace: item}, nil
	case workspaceevents.TopicWorkspaceCommandList:
		if _, ok := data.(workspaceevents.ListWorkspacesCommand); !ok || m.workspaces == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.workspaces.List(ctx)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Workspaces: items}, nil
	case workspaceevents.TopicWorkspaceCommandUpdate:
		cmd, ok := data.(workspaceevents.UpdateWorkspaceCommand)
		if !ok || m.workspaces == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.workspaces.Update(ctx, workspaceFromUpdateCommand(cmd)); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandDelete, workspaceevents.TopicWorkspaceCommandPurge:
		cmd, ok := data.(workspaceevents.DeleteWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DeleteWorkspace(ctx, cmd.WorkspaceID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandDefineGoal:
		cmd, ok := data.(workspaceevents.DefineGoalCommand)
		if !ok || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DefineGoal(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: item}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateGoal:
		cmd, ok := data.(workspaceevents.UpdateGoalCommand)
		if !ok || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.UpdateGoal(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: item}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteGoal:
		cmd, ok := data.(workspaceevents.DeleteGoalCommand)
		if !ok || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.goals.Delete(ctx, cmd.GoalID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetGoal:
		cmd, ok := data.(workspaceevents.GetGoalCommand)
		if !ok || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: item}, nil
	case workspaceevents.TopicWorkspaceCommandListGoalsByWS:
		cmd, ok := data.(workspaceevents.ListGoalsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goals: items}, nil
	case workspaceevents.TopicWorkspaceCommandDefineDoD:
		cmd, ok := data.(workspaceevents.DefineDoDCommand)
		if !ok || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DefineDoD(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: item}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateDoD:
		cmd, ok := data.(workspaceevents.UpdateDoDCommand)
		if !ok || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.UpdateDoD(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: item}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteDoD:
		cmd, ok := data.(workspaceevents.DeleteDoDCommand)
		if !ok || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.dods.Delete(ctx, cmd.DoDID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetDoD:
		cmd, ok := data.(workspaceevents.GetDoDCommand)
		if !ok || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: item}, nil
	case workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		cmd, ok := data.(workspaceevents.ListDoDsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.dods.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoDs: items}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandCreate:
		cmd, ok := data.(workspaceevents.CreateWorkspaceTemplateCommand)
		if !ok || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromCommand(cmd)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.templates.Create(ctx, item); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Template: item}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandGet:
		cmd, ok := data.(workspaceevents.GetWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.templates.Get(ctx, cmd.TemplateID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Template: item}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandList:
		if _, ok := data.(workspaceevents.ListWorkspaceTemplatesCommand); !ok || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.templates.List(ctx)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Templates: items}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandUpdate:
		cmd, ok := data.(workspaceevents.UpdateWorkspaceTemplateCommand)
		if !ok || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromCommand(cmd)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.templates.Update(ctx, item); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Template: item}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandDelete:
		cmd, ok := data.(workspaceevents.DeleteWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.templates.Delete(ctx, cmd.TemplateID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	default:
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
}

func workspaceScopeFromCommand(topic string, data any) (common.WorkspaceID, error) {
	switch cmd := data.(type) {
	case workspaceevents.CreateWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.EnsureWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.GetWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.UpdateWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DeleteWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DefineGoalCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.GetGoalCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.ListGoalsByWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DefineDoDCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.GetDoDCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.ListDoDsByWorkspaceCommand:
		return cmd.WorkspaceID, nil
	default:
		if topic == "" {
			return "", common.ErrInvalidArgument
		}
		return "", nil
	}
}

func setWorkspaceMagicResult(result magicevent.Result, data any, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Module) handleWorkspaceQueryCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	workspaceID, err := workspaceCommandWorkspaceScope(envelope)
	if err != nil {
		return err
	}
	if workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch envelope.Topic {
	case workspaceevents.TopicWorkspaceCommandCreate:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.CreateWorkspaceCommand); ok {
			if err := m.servicePtr.CreateWorkspace(ctx, cmd); err != nil {
				return err
			}
			return workspaceCommandResponse(envelope, nil)
		}
		cmd, err := createWorkspaceCommandFromPayload(envelope.Payload)
		if err != nil {
			item, modelErr := workspaceFromPayload(envelope.Payload)
			if modelErr != nil {
				return err
			}
			cmd = workspacecontracts.CreateWorkspaceCommand{
				WorkspaceID:          item.ID,
				Title:                item.Title,
				Description:          item.Description,
				DeliverableDirectory: item.DeliverableDirectory,
				InternalDirectory:    item.InternalDirectory,
			}
		}
		if err := m.servicePtr.CreateWorkspace(ctx, cmd); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case workspaceevents.TopicWorkspaceCommandEnsure:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.EnsureWorkspaceCommand)
		if !ok {
			cmd = workspacecontracts.EnsureWorkspaceCommand{
				WorkspaceID:          common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)),
				Title:                stringPayload(envelope.Payload, events.PayloadKeyTitle),
				Description:          stringPayload(envelope.Payload, events.PayloadKeyDescription),
				DeliverableDirectory: stringPayload(envelope.Payload, events.PayloadKeyDeliverableDirectory),
				InternalDirectory:    stringPayload(envelope.Payload, events.PayloadKeyInternalDirectory),
			}
			if cmd.WorkspaceID == "" {
				cmd.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
			}
		}
		if err := m.servicePtr.EnsureWorkspace(ctx, cmd); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case workspaceevents.TopicWorkspaceCommandGet:
		if m.workspaces == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.workspaces.Get(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspace: item})
	case workspaceevents.TopicWorkspaceCommandList:
		if m.workspaces == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.workspaces.List(ctx)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaces: items})
	case workspaceevents.TopicWorkspaceCommandUpdate:
		if m.workspaces == nil {
			return common.ErrInvalidArgument
		}
		var item *workspacemodel.Workspace
		if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.UpdateWorkspaceCommand); ok {
			item = workspaceFromUpdateCommand(cmd)
		} else {
			var err error
			item, err = workspaceFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := m.workspaces.Update(ctx, item); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case workspaceevents.TopicWorkspaceCommandDelete:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		id := deleteWorkspaceIDFromPayload(envelope.Payload, envelope.WorkspaceID)
		if err := m.servicePtr.DeleteWorkspace(ctx, id); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case workspaceevents.TopicWorkspaceCommandPurge:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		id := deleteWorkspaceIDFromPayload(envelope.Payload, envelope.WorkspaceID)
		if err := m.servicePtr.DeleteWorkspace(ctx, id); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case workspaceevents.TopicWorkspaceCommandDefineGoal:
		if m.servicePtr == nil || m.goals == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.DefineGoalCommand)
		if !ok {
			cmd = workspacecontracts.DefineGoalCommand{
				GoalID:      common.GoalID(stringPayload(envelope.Payload, events.PayloadKeyGoalID)),
				WorkspaceID: common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)),
				Text:        stringPayload(envelope.Payload, events.PayloadKeyText),
			}
			if cmd.WorkspaceID == "" {
				cmd.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
			}
		}
		if err := m.servicePtr.DefineGoal(ctx, cmd); err != nil {
			return err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyGoal: item})
	case workspaceevents.TopicWorkspaceCommandUpdateGoal:
		if m.servicePtr == nil || m.goals == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.UpdateGoalCommand)
		if !ok {
			cmd = workspacecontracts.UpdateGoalCommand{
				GoalID: common.GoalID(stringPayload(envelope.Payload, events.PayloadKeyGoalID)),
				Text:   stringPayload(envelope.Payload, events.PayloadKeyText),
			}
		}
		if err := m.servicePtr.UpdateGoal(ctx, cmd); err != nil {
			return err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyGoal: item})
	case workspaceevents.TopicWorkspaceCommandDeleteGoal:
		if m.goals == nil {
			return common.ErrInvalidArgument
		}
		if err := m.goals.Delete(ctx, goalIDFromPayload(envelope.Payload)); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case workspaceevents.TopicWorkspaceCommandGetGoal:
		if m.goals == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.goals.Get(ctx, common.GoalID(stringPayload(envelope.Payload, events.PayloadKeyGoalID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyGoal: item})
	case workspaceevents.TopicWorkspaceCommandListGoalsByWS:
		if m.goals == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.goals.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyGoals: items})
	case workspaceevents.TopicWorkspaceCommandDefineDoD:
		if m.servicePtr == nil || m.dods == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.DefineDoDCommand)
		if !ok {
			cmd = workspacecontracts.DefineDoDCommand{
				DoDID:       common.DoDID(stringPayload(envelope.Payload, events.PayloadKeyDoDID)),
				WorkspaceID: common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)),
				Items:       stringSlicePayload(envelope.Payload, events.PayloadKeyItems),
			}
			if cmd.WorkspaceID == "" {
				cmd.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
			}
		}
		if err := m.servicePtr.DefineDoD(ctx, cmd); err != nil {
			return err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyDoD: item})
	case workspaceevents.TopicWorkspaceCommandUpdateDoD:
		if m.servicePtr == nil || m.dods == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspacecontracts.UpdateDoDCommand)
		if !ok {
			cmd = workspacecontracts.UpdateDoDCommand{
				DoDID: common.DoDID(stringPayload(envelope.Payload, events.PayloadKeyDoDID)),
				Items: stringSlicePayload(envelope.Payload, events.PayloadKeyItems),
			}
		}
		if err := m.servicePtr.UpdateDoD(ctx, cmd); err != nil {
			return err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyDoD: item})
	case workspaceevents.TopicWorkspaceCommandDeleteDoD:
		if m.dods == nil {
			return common.ErrInvalidArgument
		}
		if err := m.dods.Delete(ctx, dodIDFromPayload(envelope.Payload)); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case workspaceevents.TopicWorkspaceCommandGetDoD:
		if m.dods == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.dods.Get(ctx, common.DoDID(stringPayload(envelope.Payload, events.PayloadKeyDoDID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyDoD: item})
	case workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		if m.dods == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.dods.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyDoDs: items})
	case workspaceevents.TopicWorkspaceTemplateCommandCreate:
		if m.templates == nil {
			return common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.templates.Create(ctx, item); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaceTemplate: item})
	case workspaceevents.TopicWorkspaceTemplateCommandGet:
		if m.templates == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.templates.Get(ctx, stringPayload(envelope.Payload, events.PayloadKeyTemplateID))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaceTemplate: item})
	case workspaceevents.TopicWorkspaceTemplateCommandList:
		if m.templates == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.templates.List(ctx)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaceTemplates: items})
	case workspaceevents.TopicWorkspaceTemplateCommandUpdate:
		if m.templates == nil {
			return common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.templates.Update(ctx, item); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaceTemplate: item})
	case workspaceevents.TopicWorkspaceTemplateCommandDelete:
		if m.templates == nil {
			return common.ErrInvalidArgument
		}
		if err := m.templates.Delete(ctx, workspaceTemplateIDFromPayload(envelope.Payload)); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func workspaceTemplateFromPayload(payload map[string]any) (*repository.WorkspaceTemplate, error) {
	if cmd, ok := payload[events.PayloadKeyCommand].(workspacecontracts.CreateWorkspaceTemplateCommand); ok {
		return workspaceTemplateFromCommand(cmd)
	}
	if cmd, ok := payload[events.PayloadKeyCommand].(workspacecontracts.UpdateWorkspaceTemplateCommand); ok {
		return workspaceTemplateFromCommand(cmd)
	}
	item, ok := payload[events.PayloadKeyWorkspaceTemplate].(*repository.WorkspaceTemplate)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func workspaceTemplateFromCommand(cmd workspacecontracts.CreateWorkspaceTemplateCommand) (*repository.WorkspaceTemplate, error) {
	if cmd.TemplateID == "" {
		return nil, common.ErrInvalidArgument
	}
	return &repository.WorkspaceTemplate{
		ID:               cmd.TemplateID,
		Name:             cmd.Name,
		Description:      cmd.Description,
		GoalText:         cmd.GoalText,
		DoDItems:         append([]string(nil), cmd.DoDItems...),
		DefaultSoulID:    cmd.DefaultSoulID,
		SoulBindings:     cloneStringMap(cmd.SoulBindings),
		ContextHints:     cloneMap(cmd.ContextHints),
		EvidencePolicies: cloneMapSlice(cmd.EvidencePolicies),
	}, nil
}

func workspaceTemplateIDFromPayload(payload map[string]any) string {
	if cmd, ok := payload[events.PayloadKeyCommand].(workspacecontracts.DeleteWorkspaceTemplateCommand); ok && cmd.TemplateID != "" {
		return cmd.TemplateID
	}
	return stringPayload(payload, events.PayloadKeyTemplateID)
}

func workspaceFromPayload(payload map[string]any) (*workspacemodel.Workspace, error) {
	item, ok := payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func workspaceFromUpdateCommand(cmd workspacecontracts.UpdateWorkspaceCommand) *workspacemodel.Workspace {
	return &workspacemodel.Workspace{
		ID:                   cmd.WorkspaceID,
		Title:                cmd.Title,
		Description:          cmd.Description,
		DeliverableDirectory: cmd.DeliverableDirectory,
		InternalDirectory:    cmd.InternalDirectory,
		State:                cmd.State,
		ActiveRunID:          cmd.ActiveRunID,
		TemplateID:           cmd.TemplateID,
		ProfileID:            cmd.ProfileID,
		DefaultSoulID:        cmd.DefaultSoulID,
		SoulBindings:         cloneSoulBindingMap(cmd.SoulBindings),
		ContextHints:         cloneMap(cmd.ContextHints),
		EvidencePolicies:     cloneMapSlice(cmd.EvidencePolicies),
	}
}

func createWorkspaceCommandFromPayload(payload map[string]any) (workspacecontracts.CreateWorkspaceCommand, error) {
	cmd := workspacecontracts.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID)),
		Title:                stringPayload(payload, events.PayloadKeyTitle),
		Description:          stringPayload(payload, events.PayloadKeyDescription),
		DeliverableDirectory: stringPayload(payload, events.PayloadKeyDeliverableDirectory),
		InternalDirectory:    stringPayload(payload, events.PayloadKeyInternalDirectory),
	}
	if cmd.WorkspaceID == "" || cmd.Title == "" || cmd.DeliverableDirectory == "" {
		return workspacecontracts.CreateWorkspaceCommand{}, common.ErrInvalidArgument
	}
	return cmd, nil
}

func deleteWorkspaceIDFromPayload(payload map[string]any, envelopeWorkspaceID string) common.WorkspaceID {
	if cmd, ok := payload[events.PayloadKeyCommand].(workspacecontracts.DeleteWorkspaceCommand); ok && cmd.WorkspaceID != "" {
		return cmd.WorkspaceID
	}
	id := common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID))
	if id != "" {
		return id
	}
	return common.WorkspaceID(envelopeWorkspaceID)
}

func goalIDFromPayload(payload map[string]any) common.GoalID {
	if cmd, ok := payload[events.PayloadKeyCommand].(workspacecontracts.DeleteGoalCommand); ok && cmd.GoalID != "" {
		return cmd.GoalID
	}
	return common.GoalID(stringPayload(payload, events.PayloadKeyGoalID))
}

func dodIDFromPayload(payload map[string]any) common.DoDID {
	if cmd, ok := payload[events.PayloadKeyCommand].(workspacecontracts.DeleteDoDCommand); ok && cmd.DoDID != "" {
		return cmd.DoDID
	}
	return common.DoDID(stringPayload(payload, events.PayloadKeyDoDID))
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

func cloneSoulBindingMap(input map[string]common.SoulID) map[string]common.SoulID {
	if input == nil {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
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

func cloneMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneMap(item))
	}
	return ret
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func workspaceCommandWorkspaceScope(envelope events.Envelope) (common.WorkspaceID, error) {
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	commandWorkspaceID := workspaceIDFromCommandPayload(envelope.Payload[events.PayloadKeyCommand])
	if workspaceID != "" && commandWorkspaceID != "" && workspaceID != commandWorkspaceID {
		return "", common.ErrConflict
	}
	if workspaceID == "" {
		workspaceID = commandWorkspaceID
	}
	envelopeWorkspaceID := common.WorkspaceID(envelope.WorkspaceID)
	if workspaceID != "" && envelopeWorkspaceID != "" && workspaceID != envelopeWorkspaceID {
		return "", common.ErrConflict
	}
	if workspaceID == "" {
		workspaceID = envelopeWorkspaceID
	}
	return workspaceID, nil
}

func workspaceIDFromCommandPayload(command any) common.WorkspaceID {
	switch cmd := command.(type) {
	case workspacecontracts.CreateWorkspaceCommand:
		return cmd.WorkspaceID
	case workspacecontracts.EnsureWorkspaceCommand:
		return cmd.WorkspaceID
	case workspacecontracts.UpdateWorkspaceCommand:
		return cmd.WorkspaceID
	case workspacecontracts.DeleteWorkspaceCommand:
		return cmd.WorkspaceID
	case workspacecontracts.DefineGoalCommand:
		return cmd.WorkspaceID
	case workspacecontracts.DefineDoDCommand:
		return cmd.WorkspaceID
	default:
		return ""
	}
}

func stringSlicePayload(payload map[string]any, key string) []string {
	switch value := payload[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		ret := make([]string, 0, len(value))
		for _, item := range value {
			text, _ := item.(string)
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func workspaceCommandResponse(request events.Envelope, payload map[string]any) error {
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
