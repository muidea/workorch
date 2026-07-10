package definitionports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadGoal(ctx context.Context, hub magicevent.Hub, goalID common.GoalID) (*workspacemodel.Goal, error) {
	return ownerreadports.ReadGoalByOwnerCommand(ctx, hub, goalID)
}

func ListGoalsByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	return ownerreadports.ListGoalsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func DefineGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, text string) (*workspacemodel.Goal, error) {
	response, err := sendDefinitionMagicCommand[workspaceevents.DefineGoalCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandDefineGoal, workspaceevents.CommandDestination, "goal", string(goalID), workspaceID, workspaceevents.DefineGoalCommand{
		GoalID:      goalID,
		WorkspaceID: workspaceID,
		Text:        text,
	})
	if err != nil {
		return nil, err
	}
	if response.Goal == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Goal, nil
}

func UpdateGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, text string) (*workspacemodel.Goal, error) {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	response, err := sendDefinitionMagicCommand[workspaceevents.UpdateGoalCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateGoal, workspaceevents.CommandDestination, "goal", string(goalID), workspaceID, workspaceevents.UpdateGoalCommand{
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		Text:        text,
	})
	if err != nil {
		return nil, err
	}
	if response.Goal == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Goal, nil
}

func DeleteGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID) error {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	_, err := sendDefinitionMagicCommand[workspaceevents.DeleteGoalCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandDeleteGoal, workspaceevents.CommandDestination, "goal", string(goalID), workspaceID, workspaceevents.DeleteGoalCommand{WorkspaceID: workspaceID, GoalID: goalID})
	return err
}
