package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadGoalByOwnerCommand(ctx context.Context, hub magicevent.Hub, goalID common.GoalID) (*workspacemodel.Goal, error) {
	if hub == nil || goalID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[workspaceevents.GetGoalCommand, workspaceevents.WorkspaceCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, workspaceevents.TopicWorkspaceCommandGetGoal, workspaceevents.CommandDestination, "goal", string(goalID), workspaceevents.GetGoalCommand{
		WorkspaceID: workspaceID,
		GoalID:      goalID,
	})
	if err != nil {
		return nil, err
	}
	if response.Goal == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Goal, nil
}

func ListGoalsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[workspaceevents.ListGoalsByWorkspaceCommand, workspaceevents.WorkspaceCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, workspaceevents.TopicWorkspaceCommandListGoalsByWS, workspaceevents.CommandDestination, "workspace", string(workspaceID), workspaceevents.ListGoalsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Goals, nil
}
