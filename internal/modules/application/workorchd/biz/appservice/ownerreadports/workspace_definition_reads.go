package ownerreadports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadGoalByOwnerCommand(ctx context.Context, hub events.Hub, goalID common.GoalID) (*workspacemodel.Goal, error) {
	if hub == nil || goalID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, workspaceevents.TopicWorkspaceCommandGetGoal, "goal", string(goalID), "", "", map[string]any{
		events.PayloadKeyGoalID: string(goalID),
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

func ListGoalsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, workspaceevents.TopicWorkspaceCommandListGoalsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyGoals].([]*workspacemodel.Goal)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
