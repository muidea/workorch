package workspace

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeGoalCommand(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceCommandDefineGoal:
		cmd, ok := ev.Data().(workspaceevents.DefineGoalCommand)
		if !ok || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.DefineGoal(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateGoal:
		cmd, ok := ev.Data().(workspaceevents.UpdateGoalCommand)
		if !ok || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.UpdateGoal(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteGoal:
		cmd, ok := ev.Data().(workspaceevents.DeleteGoalCommand)
		if !ok || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.goals.Delete(ctx, cmd.GoalID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetGoal:
		cmd, ok := ev.Data().(workspaceevents.GetGoalCommand)
		if !ok || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandListGoalsByWS:
		cmd, ok := ev.Data().(workspaceevents.ListGoalsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goals: workspacemodel.CloneGoals(items)}, nil
	default:
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
}
