package workspace

import (
	"context"

	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type CreateWorkspaceCommand = workspaceservice.CreateWorkspaceCommand
type EnsureWorkspaceCommand = workspaceservice.EnsureWorkspaceCommand
type DefineGoalCommand = workspaceservice.DefineGoalCommand
type UpdateGoalCommand = workspaceservice.UpdateGoalCommand
type DefineDoDCommand = workspaceservice.DefineDoDCommand
type UpdateDoDCommand = workspaceservice.UpdateDoDCommand
type Service = workspaceservice.Service

type Dependencies struct {
	Workspaces repository.WorkspaceRepository
	Goals      repository.GoalRepository
	DoDs       repository.DoDRepository
	EventHub   events.Hub
}

type Module struct {
	bizPtr     *workspacebiz.GoalDoDManager
	servicePtr *workspaceservice.Module
	workspaces repository.WorkspaceRepository
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	eventHub   events.Hub
	unsub      []func()
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Workspaces == nil || deps.Goals == nil || deps.DoDs == nil {
		return common.ErrInvalidArgument
	}
	m.workspaces = deps.Workspaces
	m.goals = deps.Goals
	m.dods = deps.DoDs
	m.eventHub = deps.EventHub
	m.bizPtr = workspacebiz.NewGoalDoDManager(deps.Workspaces, deps.Goals, deps.DoDs, deps.EventHub)
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
	if m == nil || m.eventHub == nil {
		return nil
	}
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(events.TopicWorkspaceCommandCreate, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandEnsure, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandGet, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandList, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandUpdate, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandDelete, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandPurge, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandDefineGoal, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandUpdateGoal, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandDeleteGoal, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandGetGoal, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandListGoalsByWS, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandDefineDoD, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandUpdateDoD, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandDeleteDoD, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandGetDoD, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceCommandListDoDsByWS, m.handleWorkspaceQueryCommand),
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
	m.workspaces = nil
	m.goals = nil
	m.dods = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleWorkspaceQueryCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicWorkspaceCommandCreate:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspaceservice.CreateWorkspaceCommand); ok {
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
			cmd = workspaceservice.CreateWorkspaceCommand{
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
	case events.TopicWorkspaceCommandEnsure:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspaceservice.EnsureWorkspaceCommand)
		if !ok {
			cmd = workspaceservice.EnsureWorkspaceCommand{
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
	case events.TopicWorkspaceCommandGet:
		if m.workspaces == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.workspaces.Get(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspace: item})
	case events.TopicWorkspaceCommandList:
		if m.workspaces == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.workspaces.List(ctx)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaces: items})
	case events.TopicWorkspaceCommandUpdate:
		if m.workspaces == nil {
			return common.ErrInvalidArgument
		}
		item, err := workspaceFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.workspaces.Update(ctx, item); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case events.TopicWorkspaceCommandDelete:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		id := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
		if id == "" {
			id = common.WorkspaceID(envelope.WorkspaceID)
		}
		if err := m.servicePtr.DeleteWorkspace(ctx, id); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case events.TopicWorkspaceCommandPurge:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		id := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
		if id == "" {
			id = common.WorkspaceID(envelope.WorkspaceID)
		}
		if err := m.servicePtr.DeleteWorkspace(ctx, id); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case events.TopicWorkspaceCommandDefineGoal:
		if m.servicePtr == nil || m.goals == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspaceservice.DefineGoalCommand)
		if !ok {
			cmd = workspaceservice.DefineGoalCommand{
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
	case events.TopicWorkspaceCommandUpdateGoal:
		if m.servicePtr == nil || m.goals == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspaceservice.UpdateGoalCommand)
		if !ok {
			cmd = workspaceservice.UpdateGoalCommand{
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
	case events.TopicWorkspaceCommandDeleteGoal:
		if m.goals == nil {
			return common.ErrInvalidArgument
		}
		if err := m.goals.Delete(ctx, common.GoalID(stringPayload(envelope.Payload, events.PayloadKeyGoalID))); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case events.TopicWorkspaceCommandGetGoal:
		if m.goals == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.goals.Get(ctx, common.GoalID(stringPayload(envelope.Payload, events.PayloadKeyGoalID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyGoal: item})
	case events.TopicWorkspaceCommandListGoalsByWS:
		if m.goals == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.goals.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyGoals: items})
	case events.TopicWorkspaceCommandDefineDoD:
		if m.servicePtr == nil || m.dods == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspaceservice.DefineDoDCommand)
		if !ok {
			cmd = workspaceservice.DefineDoDCommand{
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
	case events.TopicWorkspaceCommandUpdateDoD:
		if m.servicePtr == nil || m.dods == nil {
			return common.ErrInvalidArgument
		}
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(workspaceservice.UpdateDoDCommand)
		if !ok {
			cmd = workspaceservice.UpdateDoDCommand{
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
	case events.TopicWorkspaceCommandDeleteDoD:
		if m.dods == nil {
			return common.ErrInvalidArgument
		}
		if err := m.dods.Delete(ctx, common.DoDID(stringPayload(envelope.Payload, events.PayloadKeyDoDID))); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case events.TopicWorkspaceCommandGetDoD:
		if m.dods == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.dods.Get(ctx, common.DoDID(stringPayload(envelope.Payload, events.PayloadKeyDoDID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyDoD: item})
	case events.TopicWorkspaceCommandListDoDsByWS:
		if m.dods == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.dods.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyDoDs: items})
	default:
		return common.ErrInvalidArgument
	}
}

func workspaceFromPayload(payload map[string]any) (*workspacemodel.Workspace, error) {
	item, ok := payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func createWorkspaceCommandFromPayload(payload map[string]any) (workspaceservice.CreateWorkspaceCommand, error) {
	cmd := workspaceservice.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID)),
		Title:                stringPayload(payload, events.PayloadKeyTitle),
		Description:          stringPayload(payload, events.PayloadKeyDescription),
		DeliverableDirectory: stringPayload(payload, events.PayloadKeyDeliverableDirectory),
		InternalDirectory:    stringPayload(payload, events.PayloadKeyInternalDirectory),
	}
	if cmd.WorkspaceID == "" || cmd.Title == "" || cmd.DeliverableDirectory == "" {
		return workspaceservice.CreateWorkspaceCommand{}, common.ErrInvalidArgument
	}
	return cmd, nil
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
