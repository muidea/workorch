package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadWorkspaceByOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGet, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.WorkspaceResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandGet)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspaceeventmapping.Workspace(response.Workspace), nil
}

func ListWorkspacesByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*workspacemodel.Workspace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	command := workspaceevents.ListWorkspacesCommand{}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandList, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandList, "workspace", "all", "", "", "")
	response, err := ownerevent.WorkspaceOwner[workspaceevents.WorkspaceListResult](hub.Send(event), workspaceevents.TopicWorkspaceCommandList)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(command); err != nil {
		return nil, err
	}
	return workspaceeventmapping.Workspaces(response.Workspaces), nil
}
