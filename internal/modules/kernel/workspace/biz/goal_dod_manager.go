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
	eventHub   events.Hub
}

func NewGoalDoDManager(workspaces repository.WorkspaceRepository, eventHub events.Hub) *GoalDoDManager {
	return &GoalDoDManager{workspaces: workspaces, eventHub: eventHub}
}

func (m *GoalDoDManager) CreateWorkspace(ctx context.Context, cmd workspaceservice.CreateWorkspaceCommand) error {
	if cmd.WorkspaceID == "" || cmd.Title == "" {
		return common.ErrInvalidArgument
	}

	workspace := &workspaceModel.Workspace{
		ID:    cmd.WorkspaceID,
		Title: cmd.Title,
		State: common.WorkspaceStateActive,
	}
	if err := m.workspaces.Create(ctx, workspace); err != nil {
		return err
	}
	return m.publishWorkspaceEvent(ctx, events.TopicWorkspaceCreated, workspace)
}

func (m *GoalDoDManager) EnsureWorkspace(ctx context.Context, cmd workspaceservice.EnsureWorkspaceCommand) error {
	if cmd.WorkspaceID == "" || cmd.Title == "" {
		return common.ErrInvalidArgument
	}

	workspace, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return m.CreateWorkspace(ctx, workspaceservice.CreateWorkspaceCommand{
				WorkspaceID: cmd.WorkspaceID,
				Title:       cmd.Title,
			})
		}
		return err
	}

	changed := false
	if workspace.Title != cmd.Title {
		workspace.Title = cmd.Title
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
		Payload:      events.WorkspacePayload(workspace.Title, workspace.State, workspace.ActiveRunID),
	})
}
