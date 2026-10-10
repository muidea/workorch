package definitionports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
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
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.DefineGoalCommand{
		GoalID:      goalID,
		WorkspaceID: workspaceID,
		Text:        text,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDefineGoal, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDefineGoal, "goal", string(goalID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.GoalResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandDefineGoal)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspacemodel.CloneGoal(response.Goal), nil
}

func UpdateGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, text string) (*workspacemodel.Goal, error) {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.UpdateGoalCommand{
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		Text:        text,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateGoal, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandUpdateGoal, "goal", string(goalID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.GoalResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdateGoal)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspacemodel.CloneGoal(response.Goal), nil
}

func DeleteGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID) error {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	command := workspaceevents.DeleteGoalCommand{WorkspaceID: workspaceID, GoalID: goalID}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDeleteGoal, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDeleteGoal, "goal", string(goalID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.DeleteGoalResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandDeleteGoal)
	if err != nil {
		return err
	}
	return response.ValidateFor(command)
}
