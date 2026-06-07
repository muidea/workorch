package workspace

import (
	"context"

	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
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
	EventHub   events.Hub
}

type Module struct {
	bizPtr     *workspacebiz.GoalDoDManager
	servicePtr *workspaceservice.Module
	workspaces repository.WorkspaceRepository
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	templates  repository.WorkspaceTemplateRepository
	eventHub   events.Hub
	unsub      []func()
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Workspaces == nil || deps.Goals == nil || deps.DoDs == nil || deps.Templates == nil {
		return common.ErrInvalidArgument
	}
	m.workspaces = deps.Workspaces
	m.goals = deps.Goals
	m.dods = deps.DoDs
	m.templates = deps.Templates
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
		m.eventHub.Subscribe(events.TopicWorkspaceTemplateCommandCreate, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceTemplateCommandGet, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceTemplateCommandList, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceTemplateCommandUpdate, m.handleWorkspaceQueryCommand),
		m.eventHub.Subscribe(events.TopicWorkspaceTemplateCommandDelete, m.handleWorkspaceQueryCommand),
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
	m.templates = nil
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
	case events.TopicWorkspaceCommandEnsure:
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
	case events.TopicWorkspaceCommandDelete:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		id := deleteWorkspaceIDFromPayload(envelope.Payload, envelope.WorkspaceID)
		if err := m.servicePtr.DeleteWorkspace(ctx, id); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case events.TopicWorkspaceCommandPurge:
		if m.servicePtr == nil {
			return common.ErrInvalidArgument
		}
		id := deleteWorkspaceIDFromPayload(envelope.Payload, envelope.WorkspaceID)
		if err := m.servicePtr.DeleteWorkspace(ctx, id); err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, nil)
	case events.TopicWorkspaceCommandDefineGoal:
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
	case events.TopicWorkspaceCommandUpdateGoal:
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
	case events.TopicWorkspaceCommandDeleteGoal:
		if m.goals == nil {
			return common.ErrInvalidArgument
		}
		if err := m.goals.Delete(ctx, goalIDFromPayload(envelope.Payload)); err != nil {
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
	case events.TopicWorkspaceCommandUpdateDoD:
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
	case events.TopicWorkspaceCommandDeleteDoD:
		if m.dods == nil {
			return common.ErrInvalidArgument
		}
		if err := m.dods.Delete(ctx, dodIDFromPayload(envelope.Payload)); err != nil {
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
	case events.TopicWorkspaceTemplateCommandCreate:
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
	case events.TopicWorkspaceTemplateCommandGet:
		if m.templates == nil {
			return common.ErrInvalidArgument
		}
		item, err := m.templates.Get(ctx, stringPayload(envelope.Payload, events.PayloadKeyTemplateID))
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaceTemplate: item})
	case events.TopicWorkspaceTemplateCommandList:
		if m.templates == nil {
			return common.ErrInvalidArgument
		}
		items, err := m.templates.List(ctx)
		if err != nil {
			return err
		}
		return workspaceCommandResponse(envelope, map[string]any{events.PayloadKeyWorkspaceTemplates: items})
	case events.TopicWorkspaceTemplateCommandUpdate:
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
	case events.TopicWorkspaceTemplateCommandDelete:
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
