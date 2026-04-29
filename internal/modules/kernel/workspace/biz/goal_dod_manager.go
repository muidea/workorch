package workspace

import (
	"context"
	"errors"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type GoalDoDManager struct {
	workspaces repository.WorkspaceRepository
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	eventHub   events.Hub
}

func NewGoalDoDManager(workspaces repository.WorkspaceRepository, goals repository.GoalRepository, dods repository.DoDRepository, eventHub events.Hub) *GoalDoDManager {
	return &GoalDoDManager{workspaces: workspaces, goals: goals, dods: dods, eventHub: eventHub}
}

func (m *GoalDoDManager) CreateWorkspace(ctx context.Context, cmd workspaceservice.CreateWorkspaceCommand) error {
	if cmd.WorkspaceID == "" || cmd.Title == "" || cmd.WorkingDirectory == "" {
		return common.ErrInvalidArgument
	}

	workspace := &workspaceModel.Workspace{
		ID:               cmd.WorkspaceID,
		Title:            cmd.Title,
		Description:      cmd.Description,
		WorkingDirectory: cmd.WorkingDirectory,
		State:            common.WorkspaceStateActive,
	}
	if err := m.workspaces.Create(ctx, workspace); err != nil {
		return err
	}
	return m.publishWorkspaceEvent(ctx, events.TopicWorkspaceCreated, workspace)
}

func (m *GoalDoDManager) EnsureWorkspace(ctx context.Context, cmd workspaceservice.EnsureWorkspaceCommand) error {
	if cmd.WorkspaceID == "" || cmd.Title == "" || cmd.WorkingDirectory == "" {
		return common.ErrInvalidArgument
	}

	workspace, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return m.CreateWorkspace(ctx, workspaceservice.CreateWorkspaceCommand{
				WorkspaceID:      cmd.WorkspaceID,
				Title:            cmd.Title,
				Description:      cmd.Description,
				WorkingDirectory: cmd.WorkingDirectory,
			})
		}
		return err
	}

	changed := false
	if workspace.Title != cmd.Title {
		workspace.Title = cmd.Title
		changed = true
	}
	if workspace.Description != cmd.Description {
		workspace.Description = cmd.Description
		changed = true
	}
	if workspace.WorkingDirectory != cmd.WorkingDirectory {
		workspace.WorkingDirectory = cmd.WorkingDirectory
		changed = true
	}
	if workspace.State != common.WorkspaceStateActive {
		workspace.State = common.WorkspaceStateActive
		changed = true
	}
	if workspace.ActiveRunID != "" {
		workspace.ActiveRunID = ""
		changed = true
	}
	if !changed {
		return nil
	}
	if err := m.workspaces.Update(ctx, workspace); err != nil {
		return err
	}
	return m.publishWorkspaceEvent(ctx, events.TopicWorkspaceStateChanged, workspace)
}

func (m *GoalDoDManager) DeleteWorkspace(ctx context.Context, id common.WorkspaceID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	workspace, err := m.workspaces.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := m.workspaces.Delete(ctx, id); err != nil {
		return err
	}
	return m.publishWorkspaceEvent(ctx, events.TopicWorkspaceDeleted, workspace)
}

func (m *GoalDoDManager) DefineGoal(ctx context.Context, cmd workspaceservice.DefineGoalCommand) error {
	if cmd.GoalID == "" || cmd.WorkspaceID == "" || cmd.Text == "" {
		return common.ErrInvalidArgument
	}
	if _, err := m.workspaces.Get(ctx, cmd.WorkspaceID); err != nil {
		return err
	}
	goal := &workspaceModel.Goal{
		ID:          cmd.GoalID,
		WorkspaceID: cmd.WorkspaceID,
		Text:        cmd.Text,
	}
	if err := m.goals.Create(ctx, goal); err != nil {
		return err
	}
	return m.publishGoalEvent(ctx, goal)
}

func (m *GoalDoDManager) UpdateGoal(ctx context.Context, cmd workspaceservice.UpdateGoalCommand) error {
	if cmd.GoalID == "" || cmd.Text == "" {
		return common.ErrInvalidArgument
	}
	goal, err := m.goals.Get(ctx, cmd.GoalID)
	if err != nil {
		return err
	}
	if _, err := m.workspaces.Get(ctx, goal.WorkspaceID); err != nil {
		return err
	}
	goal.Text = cmd.Text
	if err := m.goals.Update(ctx, goal); err != nil {
		return err
	}
	return m.publishGoalEvent(ctx, goal)
}

func (m *GoalDoDManager) DefineDoD(ctx context.Context, cmd workspaceservice.DefineDoDCommand) error {
	if cmd.DoDID == "" || cmd.WorkspaceID == "" || len(cmd.Items) == 0 {
		return common.ErrInvalidArgument
	}
	for _, item := range cmd.Items {
		if item == "" {
			return common.ErrInvalidArgument
		}
	}
	if _, err := m.workspaces.Get(ctx, cmd.WorkspaceID); err != nil {
		return err
	}
	dod := &workspaceModel.DoD{
		ID:          cmd.DoDID,
		WorkspaceID: cmd.WorkspaceID,
		Items:       append([]string(nil), cmd.Items...),
		State:       common.DoDStateReady,
	}
	if err := m.dods.Create(ctx, dod); err != nil {
		return err
	}
	return m.publishDoDEvent(ctx, dod)
}

func (m *GoalDoDManager) UpdateDoD(ctx context.Context, cmd workspaceservice.UpdateDoDCommand) error {
	if cmd.DoDID == "" || len(cmd.Items) == 0 {
		return common.ErrInvalidArgument
	}
	for _, item := range cmd.Items {
		if item == "" {
			return common.ErrInvalidArgument
		}
	}
	dod, err := m.dods.Get(ctx, cmd.DoDID)
	if err != nil {
		return err
	}
	if _, err := m.workspaces.Get(ctx, dod.WorkspaceID); err != nil {
		return err
	}
	dod.Items = append([]string(nil), cmd.Items...)
	dod.State = common.DoDStateReady
	if err := m.dods.Update(ctx, dod); err != nil {
		return err
	}
	return m.publishDoDEvent(ctx, dod)
}

func (m *GoalDoDManager) publishWorkspaceEvent(ctx context.Context, topic string, workspace *workspaceModel.Workspace) error {
	if m.eventHub == nil {
		return nil
	}
	return m.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "workspace",
		SubjectID:    string(workspace.ID),
		WorkspaceID:  string(workspace.ID),
		Payload:      events.WorkspacePayload(workspace.Title, workspace.Description, workspace.WorkingDirectory, workspace.State, workspace.ActiveRunID),
	})
}

func (m *GoalDoDManager) publishGoalEvent(ctx context.Context, goal *workspaceModel.Goal) error {
	if m.eventHub == nil {
		return nil
	}
	return m.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicGoalDefined,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "goal",
		SubjectID:    string(goal.ID),
		WorkspaceID:  string(goal.WorkspaceID),
		Payload:      events.GoalPayload(goal.Text),
	})
}

func (m *GoalDoDManager) publishDoDEvent(ctx context.Context, dod *workspaceModel.DoD) error {
	if m.eventHub == nil {
		return nil
	}
	return m.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicDoDDefined,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "dod",
		SubjectID:    string(dod.ID),
		WorkspaceID:  string(dod.WorkspaceID),
		Payload:      events.DoDPayload(dod.Items, dod.State),
	})
}
