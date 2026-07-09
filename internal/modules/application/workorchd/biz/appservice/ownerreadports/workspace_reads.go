package ownerreadports

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadWorkspaceByOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[workspaceevents.GetWorkspaceCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandGet, workspaceevents.CommandDestination, "workspace", string(workspaceID), workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	if response.Workspace == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Workspace, nil
}

func ListWorkspacesByOwnerCommand(ctx context.Context, hub events.Hub) ([]*workspacemodel.Workspace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[workspaceevents.ListWorkspacesCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandList, workspaceevents.CommandDestination, "workspace", "all", workspaceevents.ListWorkspacesCommand{})
	if err != nil {
		return nil, err
	}
	return response.Workspaces, nil
}
