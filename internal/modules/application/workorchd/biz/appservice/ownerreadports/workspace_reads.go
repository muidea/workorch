package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadWorkspaceByOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandGet, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Workspace == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Workspace, nil
}

func ListWorkspacesByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*workspacemodel.Workspace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandList, sourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListWorkspacesCommand{})
	ownerevent.BindLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceCommandList, "workspace", "all", "", "", "")
	response, err := ownerevent.Workspace(hub.Send(event), workspaceevents.TopicWorkspaceCommandList)
	if err != nil {
		return nil, err
	}
	return response.Workspaces, nil
}
