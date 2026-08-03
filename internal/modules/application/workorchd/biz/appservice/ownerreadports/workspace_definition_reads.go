package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadGoalByOwnerCommand(ctx context.Context, hub magicevent.Hub, goalID common.GoalID) (*workspacemodel.Goal, error) {
	if hub == nil || goalID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), commandCtx, workspaceevents.GetGoalCommand{
		WorkspaceID: workspaceID,
		GoalID:      goalID,
	})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGetGoal, "goal", string(goalID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandGetGoal)
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
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandListGoalsByWS, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), commandCtx, workspaceevents.ListGoalsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandListGoalsByWS, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandListGoalsByWS)
	if err != nil {
		return nil, err
	}
	return response.Goals, nil
}
