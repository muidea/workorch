package definitionports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadGoal(ctx context.Context, hub events.Hub, goalID common.GoalID) (*workspacemodel.Goal, error) {
	return ownerreadports.ReadGoalByOwnerCommand(ctx, hub, goalID)
}

func ListGoalsByWorkspace(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	return ownerreadports.ListGoalsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func DefineGoal(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, text string) (*workspacemodel.Goal, error) {
	response, err := ownercommandports.PublishWithResponse(ctx, hub, workspaceevents.TopicWorkspaceCommandDefineGoal, "goal", string(goalID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.DefineGoalCommand{
			GoalID:      goalID,
			WorkspaceID: workspaceID,
			Text:        text,
		},
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyGoal].(*workspacemodel.Goal)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func UpdateGoal(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, text string) (*workspacemodel.Goal, error) {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	response, err := ownercommandports.PublishWithResponse(ctx, hub, workspaceevents.TopicWorkspaceCommandUpdateGoal, "goal", string(goalID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.UpdateGoalCommand{
			GoalID: goalID,
			Text:   text,
		},
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyGoal].(*workspacemodel.Goal)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func DeleteGoal(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, goalID common.GoalID) error {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	return ownercommandports.Publish(ctx, hub, workspaceevents.TopicWorkspaceCommandDeleteGoal, "goal", string(goalID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand:     workspacecontracts.DeleteGoalCommand{GoalID: goalID},
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
}
