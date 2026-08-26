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
		if !ok || cmd.WorkspaceID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.manager.DefineGoal(scopedCtx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(scopedCtx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateGoal:
		cmd, ok := ev.Data().(workspaceevents.UpdateGoalCommand)
		if !ok || cmd.WorkspaceID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.manager.UpdateGoal(scopedCtx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(scopedCtx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteGoal:
		cmd, ok := ev.Data().(workspaceevents.DeleteGoalCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.manager.DeleteGoal(scopedCtx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetGoal:
		cmd, ok := ev.Data().(workspaceevents.GetGoalCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		hidden, err := m.workspaceDeletionReadHidden(scopedCtx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if hidden {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrNotFound
		}
		item, err := m.goals.Get(scopedCtx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandListGoalsByWS:
		cmd, ok := ev.Data().(workspaceevents.ListGoalsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		hidden, err := m.workspaceDeletionReadHidden(scopedCtx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if hidden {
			return workspaceevents.WorkspaceCommandResult{Goals: []*workspacemodel.Goal{}}, nil
		}
		items, err := m.goals.ListByWorkspaceID(scopedCtx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goals: workspacemodel.CloneGoals(items)}, nil
	default:
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
}
