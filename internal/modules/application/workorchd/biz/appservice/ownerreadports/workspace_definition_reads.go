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
	command := workspaceevents.GetGoalCommand{
		WorkspaceID: workspaceID,
		GoalID:      goalID,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), commandCtx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGetGoal, "goal", string(goalID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.GoalResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandGetGoal)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspacemodel.CloneGoal(response.Goal), nil
}

func ListGoalsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	command := workspaceevents.ListGoalsByWorkspaceCommand{WorkspaceID: workspaceID}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandListGoalsByWS, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), commandCtx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandListGoalsByWS, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.GoalListResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandListGoalsByWS)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspacemodel.CloneGoals(response.Goals), nil
}
