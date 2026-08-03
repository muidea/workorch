package definitionports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDefineGoal, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DefineGoalCommand{
		GoalID:      goalID,
		WorkspaceID: workspaceID,
		Text:        text,
	})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDefineGoal, "goal", string(goalID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandDefineGoal)
	if err != nil {
		return nil, err
	}
	if response.Goal == nil {
		return nil, common.ErrInvalidArgument
	}
	return workspaceeventmapping.Goal(response.Goal), nil
}

func UpdateGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, text string) (*workspacemodel.Goal, error) {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandUpdateGoal, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.UpdateGoalCommand{
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		Text:        text,
	})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandUpdateGoal, "goal", string(goalID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandUpdateGoal)
	if err != nil {
		return nil, err
	}
	if response.Goal == nil {
		return nil, common.ErrInvalidArgument
	}
	return workspaceeventmapping.Goal(response.Goal), nil
}

func DeleteGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID) error {
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDeleteGoal, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DeleteGoalCommand{WorkspaceID: workspaceID, GoalID: goalID})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandDeleteGoal, "goal", string(goalID), workspaceID, "", "")
	_, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandDeleteGoal)
	return err
}
